package main

//"crypto/md5"
//"github.com/stapelberg/godebiancontrol"

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"code.google.com/p/go.crypto/openpgp"
	"code.google.com/p/go.crypto/openpgp/clearsign"
	"github.com/gorilla/mux"
)

var mimeMemoryBufferSize = int64(64000000)

type AptServer struct {
	MaxReqs         int
	RepoBase        string
	PoolBase        string
	TmpDir          string
	CookieName      string
	TTL             time.Duration
	ValidateChanges bool
	ValidateDebs    bool
	AftpPath        string
	AftpConfig      string
	ReleaseConfig   string
	PreAftpHook     string
	PostUploadHook  string
	PostAftpHook    string
	SignerId        *openpgp.Entity
	PoolPattern     *regexp.Regexp
	PubRing         openpgp.EntityList
	PrivRing        openpgp.EntityList

	aptLocks        *Governor
	uploadHandler   http.HandlerFunc
	downloadHandler http.HandlerFunc
	sessionManager  UploadSessionManager
}

func (a *AptServer) InitAptServer() {
	a.aptLocks, _ = NewGovernor(a.MaxReqs)

	a.downloadHandler = a.makeDownloadHandler()
	a.uploadHandler = a.makeUploadHandler()
	a.sessionManager = NewUploadSessionManager(*a)
}

func (a *AptServer) Register(r *mux.Router) {
	r.HandleFunc("/repo/{rest:.*}", a.downloadHandler).Methods("GET")
	r.HandleFunc("/package/upload", a.uploadHandler).Methods("POST", "PUT")
	r.HandleFunc("/package/upload/{session}", a.uploadHandler).Methods("GET", "POST", "PUT")
}

func (a *AptServer) makeDownloadHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a.aptLocks.ReadLock()
		defer a.aptLocks.ReadUnLock()

		file := mux.Vars(r)["rest"]
		realFile := a.RepoBase + "/" + file
		http.ServeFile(w, r, realFile)
	}
}

// This is used to store any response we want
// to send back to the caller
type AptServerResponder interface {
	GetStatus() int
	GetJSONMessage() []byte
	error
}

type aptServerResponse struct {
	statusCode int
	message    []byte
}

func (r aptServerResponse) GetStatus() int {
	return r.statusCode
}

func (r aptServerResponse) GetJSONMessage() []byte {
	return r.message
}

func (r aptServerResponse) Error() string {
	return "ERROR: " + string(r.message)
}

func AptServerStringMessage(status int, msg string) AptServerResponder {
	return aptServerResponse{
		status,
		[]byte(msg),
	}
}

func AptServerJSONMessage(status int, msg json.Marshaler) aptServerResponse {
	j, err := json.Marshal(msg)
	if err != nil {
		return aptServerResponse{
			status,
			[]byte("Could not Marshal JSON response, " + err.Error()),
		}
	}

	return aptServerResponse{
		status,
		j,
	}
}

func (a *AptServer) makeUploadHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Did we get a session
		session, found := mux.Vars(r)["session"]
		var resp AptServerResponder

		//maybe in a cookie?
		if !found {
			cookie, err := r.Cookie(a.CookieName)
			if err == nil {
				session = cookie.Value
			}
		}

		us, activeSession := a.sessionManager.GetSession(session)

		// THis all needs rewriting
		switch r.Method {
		case "GET":
			{
				if !activeSession {
					resp = AptServerStringMessage(http.StatusFound, "Unkown sesssion")
				} else {
					resp = AptServerJSONMessage(http.StatusOK, us)
				}
			}
		case "PUT", "POST":
			{
				changes, otherParts, err := a.changesFromRequest(r)

				if err != nil {
					resp = AptServerStringMessage(http.StatusBadRequest, "")
				} else {
					if session == "" {
						us, err = a.sessionManager.NewUploadSession(changes)
						if err != nil {
							resp = AptServerStringMessage(http.StatusBadRequest, err.Error())
						} else {
							cookie := http.Cookie{
								Name:     a.CookieName,
								Value:    us.SessionID(),
								Expires:  time.Now().Add(a.TTL),
								HttpOnly: false,
								Path:     "/package/upload",
							}
							http.SetCookie(w, &cookie)
							activeSession = true
						}
					} else {
						if !activeSession {
							resp = AptServerStringMessage(http.StatusNotFound, "No such session")
						}
					}

					if activeSession {
						resp = a.dispatchPostRequest(us, otherParts)
					}
				}
			}
		}

		if resp.GetStatus() == 0 {
			http.Error(w, "AptServer response statuscode not set", http.StatusInternalServerError)
		} else {
			w.WriteHeader(resp.GetStatus())
			w.Write(resp.GetJSONMessage())
		}
	}
}

func (a *AptServer) changesFromRequest(r *http.Request) (
	changes *DebChanges,
	other []*multipart.FileHeader,
	err error) {

	err = r.ParseMultipartForm(mimeMemoryBufferSize)
	if err != nil {
		return
	}

	form := r.MultipartForm
	files := form.File["debfiles"]
	var changesPart multipart.File
	for _, f := range files {
		if strings.HasSuffix(f.Filename, ".changes") {
			changesPart, _ = f.Open()
		} else {
			other = append(other, f)
		}
	}

	if changesPart == nil {
		err = errors.New("No debian changes file in request")
		return
	}

	changes, err = ParseDebianChanges(changesPart, a.PubRing)
	if err != nil {
		return
	}

	if a.ValidateChanges && !changes.signed {
		err = errors.New("Changes file was not signed")
		return
	}

	if a.ValidateChanges && !changes.validated {
		err = errors.New("Changes file could not be validated")
		return
	}

	return
}

func (a *AptServer) dispatchPostRequest(
	session UploadSessioner,
	otherParts []*multipart.FileHeader) (resp AptServerResponder) {
	var returnCode int

	if len(otherParts) > 0 {
		for _, f := range otherParts {
			reader, _ := f.Open()
			session.AddFile(&ChangesItem{
				Filename: f.Filename,
				data:     reader,
			})
		}
		if session.IsComplete() {
			a.aptLocks.WriteLock()
			defer a.aptLocks.WriteUnLock()

			os.Chdir(session.Dir()) // Chdir may be bad here
			if a.PreAftpHook != "" {
				err := exec.Command(a.PreAftpHook, session.SessionID()).Run()
				if !err.(*exec.ExitError).Success() {
					return AptServerStringMessage(
						http.StatusBadRequest,
						"Pre apt-ftparchive hook failed, "+err.Error())
				}
			}

			//Move the files into the pool
			for _, f := range session.Files() {
				dstdir := a.PoolBase + "/"
				matches := a.PoolPattern.FindSubmatch([]byte(f.Filename))
				if len(matches) > 0 {
					dstdir = dstdir + string(matches[0]) + "/"
				}
				err := os.Rename(f.Filename, dstdir+f.Filename)
				if err != nil {
					return AptServerStringMessage(http.StatusInternalServerError, "File move failed, "+err.Error())
				}
			}

			err := a.runAptFtpArchive()
			if err != nil {
				return AptServerStringMessage(http.StatusInternalServerError, "Apt FTP Archive failed, "+err.Error())
			} else {
				if a.PostAftpHook != "" {
					err = exec.Command(a.PostAftpHook, session.SessionID()).Run()
					log.Println("Error executing post-aftp-hook, " + err.Error())
				}
			}

			returnCode = http.StatusOK
		} else {
			returnCode = http.StatusAccepted
		}
	} else {
		returnCode = http.StatusCreated
	}
	return AptServerJSONMessage(returnCode, session)
}

func (a *AptServer) FindReleaseBase() (string, error) {
	releasePath := ""

	visit := func(path string, f os.FileInfo, errIn error) (err error) {
		switch {
		case f.Name() == "Contents-all":
			releasePath = filepath.Dir(path)
			err = errors.New("Found file")
		case f.Name() == "pool":
			err = filepath.SkipDir
		}
		return err
	}

	filepath.Walk(a.RepoBase, visit)

	if releasePath == "" {
		return releasePath, errors.New("Can't locate release base dir")
	}

	return releasePath, nil
}

func (a *AptServer) runAptFtpArchive() (err error) {
	err = exec.Command(a.AftpPath, "generate", a.AftpConfig).Run()
	if err != nil {
		if !err.(*exec.ExitError).Success() {
			return errors.New("Pre apt-ftparchive failed, " + err.Error())
		}
	}

	if a.ReleaseConfig != "" {
		// Generate the Releases and InReleases file
		releaseBase, _ := a.FindReleaseBase()
		releaseFilename := releaseBase + "/Release"

		releaseWriter, err := os.Create(releaseFilename)
		defer releaseWriter.Close()

		if err != nil {
			return errors.New("Error creating release file, " + err.Error())
		}

		cmd := exec.Command(a.AftpPath, "-c", a.ReleaseConfig, "release", releaseBase)
		releaseReader, _ := cmd.StdoutPipe()
		cmd.Start()
		io.Copy(releaseWriter, releaseReader)

		err = cmd.Wait()
		if err != nil {
			if !err.(*exec.ExitError).Success() {
				return errors.New("apt-ftparchive release generation failed, " + err.Error())
			}
		}

		if a.SignerId != nil {
			rereadRelease, err := os.Open(releaseFilename)
			defer rereadRelease.Close()
			releaseSignatureWriter, err := os.Create(releaseBase + "/Release.gpg")
			if err != nil {
				return errors.New("Error creating release signature file, " + err.Error())
			}
			defer releaseSignatureWriter.Close()

			err = openpgp.ArmoredDetachSign(releaseSignatureWriter, a.SignerId, rereadRelease, nil)
			if err != nil {
				return errors.New("Detached Sign failed, , " + err.Error())
			}

			rereadRelease2, err := os.Open(releaseFilename)
			defer rereadRelease2.Close()
			inReleaseSignatureWriter, err := os.Create(releaseBase + "/InRelease")
			if err != nil {
				return errors.New("Error creating InRelease file, " + err.Error())
			}
			inReleaseWriter, err := clearsign.Encode(inReleaseSignatureWriter, a.SignerId.PrivateKey, nil)
			if err != nil {
				return errors.New("Error InRelease clear-signer, " + err.Error())
			}
			io.Copy(inReleaseWriter, rereadRelease2)
			inReleaseWriter.Close()
		}

		// Release file generated

		if err != nil {
			if !err.(*exec.ExitError).Success() {
				return errors.New("Pre apt-ftparchive failed, " + err.Error())
			}
		}
	}
	return nil
}
