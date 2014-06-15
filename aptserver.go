package main

//"crypto/md5"
//"github.com/stapelberg/godebiancontrol"

import (
	"encoding/json"
	"errors"
	"io"
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
	GetMessage() []byte
	error
}

type aptServerResponse struct {
	statusCode int
	message    []byte
}

func (r aptServerResponse) GetStatus() int {
	return r.statusCode
}

func (r aptServerResponse) GetMessage() []byte {
	return r.message
}

func (r aptServerResponse) Error() string {
	return "ERROR: " + string(r.message)
}

func AptServerMessage(status int, msg interface{}) AptServerResponder {
	var err error
	var j []byte

	resp := aptServerResponse{
		statusCode: status,
	}

	switch t := msg.(type) {
	case json.Marshaler:
		{
			j, err = json.Marshal(
				struct {
					StatusCode int
					Message    json.Marshaler
				}{
					status,
					t,
				})
			resp.message = j
		}
	case string:
		{
			j, err = json.Marshal(
				struct {
					StatusCode int
					Message    string
				}{
					status,
					t,
				})
			resp.message = j
		}
	default:
		{
			j, err = json.Marshal(
				struct {
					StatusCode int
					Message    string
				}{
					status,
					t.(string),
				})
			resp.message = j
		}
	}

	if err != nil {
		resp.message = []byte("Could not marshal response, " + err.Error())
	}

	return &resp
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

		// THis all needs rewriting
		switch r.Method {
		case "GET":
			{
				resp = a.sessionManager.UploadSessionStatus(session)
			}
		case "PUT", "POST":
			{
				changes, otherParts, err := a.changesFromRequest(r)

				if err != nil {
					resp = AptServerMessage(http.StatusBadRequest, err.Error())
				} else {
					if session == "" {
						session, err = a.sessionManager.AddUploadSession(changes)
						if err != nil {
							resp = AptServerMessage(http.StatusBadRequest, err.Error())
						} else {
							cookie := http.Cookie{
								Name:     a.CookieName,
								Value:    session,
								Expires:  time.Now().Add(a.TTL),
								HttpOnly: false,
								Path:     "/package/upload",
							}
							http.SetCookie(w, &cookie)
						}
					}

					if err != nil {
						resp = AptServerMessage(http.StatusBadRequest, err.Error())
					} else {
						resp = a.sessionManager.UploadSessionAddItems(session, otherParts)
					}
				}
			}
		}

		if resp.GetStatus() == 0 {
			http.Error(w, "AptServer response statuscode not set", http.StatusInternalServerError)
		} else {
			w.WriteHeader(resp.GetStatus())
			w.Write(resp.GetMessage())
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

func (a *AptServer) findReleaseBase() (string, error) {
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
		releaseBase, _ := a.findReleaseBase()
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
