package main

import (
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"

	"regexp"
	"strings"
	"time"

	"code.google.com/p/go.crypto/openpgp"
	"github.com/gorilla/mux"
)

// The maximum amount of RAM to use when parsing
// mime object in requests.
var mimeMemoryBufferSize = int64(64000000)

type AptServer struct {
	MaxReqs    int
	CookieName string
	TTL        time.Duration

	ValidateChanges bool
	ValidateDebs    bool

	PostUploadHook HookRunner
	PreAftpHook    HookRunner
	PostAftpHook   HookRunner

	AftpPath      string
	AftpConfig    string
	ReleaseConfig string
	RepoBase      string
	PoolBase      string
	TmpDir        string
	PoolPattern   *regexp.Regexp

	PubRing  openpgp.EntityList
	PrivRing openpgp.EntityList
	SignerId *openpgp.Entity

	aptLocks        *Governor
	uploadHandler   http.HandlerFunc
	downloadHandler http.HandlerFunc
	sessionManager  UploadSessionManager
}

func (a *AptServer) InitAptServer() {
	a.aptLocks, _ = NewGovernor(a.MaxReqs)

	a.downloadHandler = a.makeDownloadHandler()
	a.uploadHandler = a.makeUploadHandler()
	a.sessionManager = NewUploadSessionManager(*a, a.TTL)
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
