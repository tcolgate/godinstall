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
	sessMap         *SafeMap
}

func (a *AptServer) InitAptServer() {
	a.aptLocks, _ = NewGovernor(a.MaxReqs)

	a.downloadHandler = makeDownloadHandler(a)
	a.uploadHandler = makeUploadHandler(a)
	a.sessMap = NewSafeMap()
}

func (a *AptServer) Register(r *mux.Router) {
	r.HandleFunc("/repo/{rest:.*}", a.downloadHandler).Methods("GET")
	r.HandleFunc("/package/upload", a.uploadHandler).Methods("POST", "PUT")
	r.HandleFunc("/package/upload/{session}", a.uploadHandler).Methods("GET", "POST", "PUT")
}

func makeDownloadHandler(a *AptServer) http.HandlerFunc {
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

type uploadSessionReq struct {
	SessionId string
	W         http.ResponseWriter
	R         *http.Request
	create    bool // This is a request to create a new upload session
}

func makeUploadHandler(a *AptServer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Did we get a session
		session, found := mux.Vars(r)["session"]

		//maybe in a cookie?
		if !found {
			cookie, err := r.Cookie(a.CookieName)
			if err == nil {
				session = cookie.Value
			}
		}

		// THis all needs rewriting
		var resp AptServerResponder
		if session == "" {
			resp = dispatchRequest(a, &uploadSessionReq{"", w, r, true})
		} else {
			resp = dispatchRequest(a, &uploadSessionReq{session, w, r, false})
		}

		if resp.GetStatus() == 0 {
			http.Error(w, "AptServer response statuscode not set", http.StatusInternalServerError)
		} else {
			w.WriteHeader(resp.GetStatus())
			w.Write(resp.GetJSONMessage())
		}
	}
}

func dispatchRequest(a *AptServer, r *uploadSessionReq) AptServerResponder {
	// Lots of this need refactoring into go routines and
	// response chanels

	if r.create {
		err := r.R.ParseMultipartForm(mimeMemoryBufferSize)
		if err != nil {
			http.Error(r.W, err.Error(), http.StatusBadRequest)
			return AptServerStringMessage(http.StatusBadRequest, err.Error())
		}

		form := r.R.MultipartForm
		files := form.File["debfiles"]
		var changesPart multipart.File
		var otherParts []*multipart.FileHeader
		for _, f := range files {
			if strings.HasSuffix(f.Filename, ".changes") {
				changesPart, _ = f.Open()
			} else {
				otherParts = append(otherParts, f)
			}
		}

		if changesPart == nil {
			return AptServerStringMessage(http.StatusBadRequest, "No debian changes file in request")
		}

		changes, err := ParseDebianChanges(changesPart, a.PubRing)
		if err != nil {
			return AptServerStringMessage(http.StatusBadRequest, err.Error())
		}

		if a.ValidateChanges && !changes.signed {
			return AptServerStringMessage(http.StatusBadRequest, "Changes file was not signed")
		}

		if a.ValidateChanges && !changes.validated {
			return AptServerStringMessage(http.StatusBadRequest, "Changes file could not be validated")
		}

		// This should probably move into the upload session constructor
		us := NewUploadSessioner(a)
		s := us.SessionID()
		cookie := http.Cookie{
			Name:     a.CookieName,
			Value:    s,
			Expires:  time.Now().Add(a.TTL),
			HttpOnly: false,
			Path:     "/package/upload",
		}
		http.SetCookie(r.W, &cookie)
		us.AddChanges(changes)

		var returnCode int

		if len(otherParts) > 0 {
			for _, f := range otherParts {
				reader, _ := f.Open()
				err = us.AddFile(&ChangesItem{
					Filename: f.Filename,
					data:     reader,
				})
			}
			if us.IsComplete() {
				returnCode = http.StatusOK
			} else {
				returnCode = http.StatusAccepted
			}
		} else {
			returnCode = http.StatusCreated
		}
		return AptServerJSONMessage(returnCode, us)
	} else {
		var us UploadSessioner
		c := a.sessMap.Get(r.SessionId)
		if c != nil {
			// Move this logic elseqhere
			switch sess := c.(type) {
			case UploadSessioner:
				us = sess
			default:
				return AptServerStringMessage(http.StatusInternalServerError, "Invalid session map entry")
			}
		} else {
			return AptServerStringMessage(http.StatusNotFound, "request for unknown session")
		}

		switch r.R.Method {
		case "GET":
			{
				return AptServerJSONMessage(http.StatusOK, us)
			}
		case "PUT", "POST":
			{
				//Add any files we have been passed
				err := r.R.ParseMultipartForm(mimeMemoryBufferSize)
				if err != nil {
					return AptServerStringMessage(http.StatusBadRequest, "Couldn't parse mime request, "+err.Error())
				}
				form := r.R.MultipartForm
				files := form.File["debfiles"]
				for _, f := range files {
					log.Println("Trying to upload: " + f.Filename)
					reader, err := f.Open()
					if err != nil {
						return AptServerStringMessage(http.StatusBadRequest, "Can't upload "+f.Filename+" - "+err.Error())
					}
					err = us.AddFile(&ChangesItem{
						Filename: f.Filename,
						data:     reader,
					})
					if err != nil {
						return AptServerStringMessage(http.StatusBadRequest, "Can't upload "+f.Filename+" - "+err.Error())
					}
				}

				if us.IsComplete() {
					a.aptLocks.WriteLock()
					defer a.aptLocks.WriteUnLock()

					os.Chdir(us.Dir()) // Chdir may be bad here
					if a.PreAftpHook != "" {
						err = exec.Command(a.PreAftpHook, us.SessionID()).Run()
						if !err.(*exec.ExitError).Success() {
							return AptServerStringMessage(http.StatusBadRequest, "Pre apt-ftparchive hook failed, "+err.Error())
						}
					}

					//Move the files into the pool
					for _, f := range us.Files() {
						dstdir := a.PoolBase + "/"
						matches := a.PoolPattern.FindSubmatch([]byte(f.Filename))
						if len(matches) > 0 {
							dstdir = dstdir + string(matches[0]) + "/"
						}
						err := os.Rename(f.Filename, dstdir+f.Filename)
						if err != nil {
							http.Error(r.W, "File move failed, "+err.Error(), http.StatusBadRequest)
							return AptServerStringMessage(http.StatusInternalServerError, "File move failed, "+err.Error())
						}
					}

					err = a.runAptFtpArchive()

					if err != nil {
						return AptServerStringMessage(http.StatusInternalServerError, "Apt FTP Archive failed, "+err.Error())
					} else {
						if a.PostAftpHook != "" {
							err = exec.Command(a.PostAftpHook, us.SessionID()).Run()
							log.Println("Error executing post-aftp-hook, " + err.Error())
						}

						return AptServerJSONMessage(http.StatusOK, us)
					}
				} else {
					return AptServerJSONMessage(http.StatusAccepted, us)
				}
			}
		default:
			{
				return AptServerStringMessage(http.StatusBadRequest, "Bad request method")
			}
		}
	}
}

func getKeyByEmail(keyring openpgp.EntityList, email string) *openpgp.Entity {
	for _, entity := range keyring {
		for _, ident := range entity.Identities {
			if ident.UserId.Email == email {
				return entity
			}
		}
	}

	return nil
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
