package main

import (
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"os"

	"strings"
	"time"

	"github.com/gorilla/mux"
)

// The maximum amount of RAM to use when parsing
// mime object in requests.
var mimeMemoryBufferSize = int64(64000000)

// AptServer describes a web server
type AptServer struct {
	MaxReqs    int           // The maximum nuber of concurrent requests we'll handle
	CookieName string        // The session cookie name for uploads
	TTL        time.Duration // How long to keep session alive

	AcceptLoneDebs bool // Whether we should allow individual deb uploads

	Repo           AptRepo              // The repository to populate
	AptGenerator   AptGenerator         // The generator for updating the repo
	SessionManager UploadSessionManager // The session manager
	UpdateChannel  chan UpdateRequest   // A channel to recieve update requests

	PreAftpHook  HookRunner // A hook to run before we run the genrator
	PostAftpHook HookRunner // A hooke to run after successful regeneration

	aptLocks *Governor // Locks to ensure the repo update is atomic

	uploadHandler   http.HandlerFunc // HTTP handler for upload requests
	downloadHandler http.HandlerFunc // HTTP handler for apt client downloads
}

// Setup the server, start related go routines
func (a *AptServer) InitAptServer() {
	a.aptLocks, _ = NewGovernor(a.MaxReqs)
	a.downloadHandler = a.makeDownloadHandler()
	a.uploadHandler = a.makeUploadHandler()

	go a.Updater()
}

// Register this server with a HTTP server
func (a *AptServer) Register(r *mux.Router) {
	r.HandleFunc("/repo/{rest:.*}", a.downloadHandler).Methods("GET")
	r.HandleFunc("/upload", a.uploadHandler).Methods("POST", "PUT")
	r.HandleFunc("/upload/{session}", a.uploadHandler).Methods("GET", "POST", "PUT")
}

// Construct the download handler for normal client downloads
func (a *AptServer) makeDownloadHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a.aptLocks.ReadLock()
		defer a.aptLocks.ReadUnLock()

		file := mux.Vars(r)["rest"]
		realFile := a.Repo.Base() + "/" + file
		http.ServeFile(w, r, realFile)
	}
}

// AptServerResponder is a custom error type to
// encode the HTTP status and meesage we will
// send back to a client
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

// This contructs a new repsonse to a client and can take
// a string of JSON'able object
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

// This build a function to despatch upload requests
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
				resp = a.SessionManager.Status(session)
			}
		case "PUT", "POST":
			{
				changesReader, otherParts, err := a.changesFromRequest(r)

				if err != nil {
					resp = AptServerMessage(http.StatusBadRequest, err.Error())
				} else {
					if session == "" {
						if changesReader == nil {
							// Not overly keen on having this here.
							if !a.AcceptLoneDebs {
								err = errors.New("No debian changes file in request")
							} else {
								if len(otherParts) == 1 {
									if !strings.HasSuffix(otherParts[0].Filename, ".deb") {
										err = errors.New("Lone files for upload must end in .deb")
									}

									resp := a.SessionManager.AddDeb(otherParts[0])
									w.WriteHeader(resp.GetStatus())
									w.Write(resp.GetMessage())
									return
								} else {
									err = errors.New("Too many files in upload request without changes file present")
								}
							}
						} else {
							session, err = a.SessionManager.AddChangesSession(changesReader)
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
					}

					if err != nil {
						resp = AptServerMessage(http.StatusBadRequest, err.Error())
					} else {
						resp = a.SessionManager.AddItems(session, otherParts)
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

// Seperate the changes file from any other files in a http
// request
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

	return
}

// Updater is a go rourtine that repond to request to run
// the apt regeneration

type CompletedUpload struct {
	Session            UploadSessioner
	PreAftpHookOutput  HookOutput
	PostAftpHookOutput HookOutput
}

func (s CompletedUpload) MarshalJSON() (j []byte, err error) {
	resp := struct {
		Session            UploadSessioner
		PreAftpHookOutput  HookOutput
		PostAftpHookOutput HookOutput
	}{
		s.Session,
		s.PreAftpHookOutput,
		s.PostAftpHookOutput,
	}
	j, err = json.Marshal(resp)
	return
}

func (a *AptServer) Updater() {
	for {
		select {
		case msg := <-a.UpdateChannel:
			{
				var err error
				respStatus := http.StatusOK
				var respObj interface{}

				session := msg.session
				completedsession := CompletedUpload{Session: session}

				os.Chdir(session.Directory()) // Chdir may be bad here

				a.aptLocks.WriteLock()

				hookResult := a.PreAftpHook.Run(session.SessionID())
				if hookResult.err != nil {
					respStatus = http.StatusBadRequest
					respObj = "Pre apt-ftparchive " + hookResult.Error()
				} else {
					completedsession.PreAftpHookOutput = hookResult
				}

				//Move the files into the pool
				for _, f := range session.Items() {
					dstdir := a.Repo.PoolFilePath(f.Filename)
					stat, err := os.Lstat(dstdir)

					if err != nil {
						if os.IsNotExist(err) {
							err = os.MkdirAll(dstdir, 0777)
							if err != nil {
								respStatus = http.StatusInternalServerError
								respObj = "File move failed, " + err.Error()
							}
						} else {
							respStatus = http.StatusInternalServerError
							respObj = "File move failed, "
						}
					} else {
						if !stat.IsDir() {
							respStatus = http.StatusInternalServerError
							respObj = "Destinatio path, " + dstdir + " is not a directory"
						}
					}

					err = os.Rename(f.Filename, dstdir+f.Filename)
					if err != nil {
						respStatus = http.StatusInternalServerError
						respObj = "File move failed, " + err.Error()
					}
				}

				err = a.AptGenerator.Regenerate()
				if err != nil {
					respStatus = http.StatusInternalServerError
					respObj = "Apt FTP Archive failed, " + err.Error()
				} else {
					hookResult := a.PostAftpHook.Run(session.SessionID())
					completedsession.PostAftpHookOutput = hookResult
				}

				a.aptLocks.WriteUnLock()

				if respStatus == http.StatusOK {
					respObj = completedsession
				}

				msg.resp <- AptServerMessage(respStatus, respObj)
			}
		}
	}
}

// UpdateRequest contains the information needed to
// request an update, only regeneration is supported
// at present
type UpdateRequest struct {
	resp    chan AptServerResponder
	session UploadSessioner
}
