package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"

	"strings"
	"time"

	"github.com/gorilla/mux"
)

// The maximum amount of RAM to use when parsing
// mime object in requests.
var mimeMemoryBufferSize = int64(64000000)

type AptServer struct {
	MaxReqs    int
	CookieName string
	TTL        time.Duration

	Repo           AptRepo
	AptGenerator   AptGenerator
	SessionManager UploadSessionManager

	PreAftpHook  HookRunner
	PostAftpHook HookRunner

	aptLocks        *Governor
	uploadHandler   http.HandlerFunc
	downloadHandler http.HandlerFunc
}

func (a *AptServer) InitAptServer() {
	a.aptLocks, _ = NewGovernor(a.MaxReqs)
	a.downloadHandler = a.makeDownloadHandler()
	a.uploadHandler = a.makeUploadHandler()
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
		realFile := a.Repo.Base() + "/" + file
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
				resp = a.SessionManager.UploadSessionStatus(session)
			}
		case "PUT", "POST":
			{
				changesReader, otherParts, err := a.changesFromRequest(r)

				if err != nil {
					resp = AptServerMessage(http.StatusBadRequest, err.Error())
				} else {
					if session == "" {
						session, err = a.SessionManager.AddUploadSession(changesReader)
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
						resp = a.SessionManager.UploadSessionAddItems(session, otherParts)
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
	changesReader io.Reader,
	other []*multipart.FileHeader,
	err error) {

	err = r.ParseMultipartForm(mimeMemoryBufferSize)
	if err != nil {
		return
	}

	form := r.MultipartForm
	files := form.File["debfiles"]
	for _, f := range files {
		if strings.HasSuffix(f.Filename, ".changes") {
			changesReader, _ = f.Open()
		} else {
			other = append(other, f)
		}
	}

	if changesReader == nil {
		err = errors.New("No debian changes file in request")
		return
	}

	return
}

// Can't work out where this should live.
func OnCompletedUpload(
	a *AptServer,
	s *uploadSession) AptServerResponder {
	var err error

	// All files uploaded
	a.aptLocks.WriteLock()
	defer a.aptLocks.WriteUnLock()

	os.Chdir(s.dir) // Chdir may be bad here

	err = a.PreAftpHook.Run(s.SessionId)
	if !err.(*exec.ExitError).Success() {
		return AptServerMessage(
			http.StatusBadRequest,
			"Pre apt-ftparchive hook failed, "+err.Error())
	}

	//Move the files into the pool
	for _, f := range s.changes.Files {
		dstdir := a.Repo.PoolFilePath(f.Filename)
		err = os.Rename(f.Filename, dstdir+f.Filename)
		if err != nil {
			return AptServerMessage(http.StatusInternalServerError, "File move failed, "+err.Error())
		}
	}

	err = a.AptGenerator.Regenerate()
	if err != nil {
		return AptServerMessage(http.StatusInternalServerError, "Apt FTP Archive failed, "+err.Error())
	} else {
		err = a.PostAftpHook.Run(s.SessionId)
		if err != nil {
			log.Println("Error executing post-aftp-hook, " + err.Error())
		}
	}

	return AptServerMessage(http.StatusOK, s)
}
