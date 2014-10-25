package main

import (
	"encoding/json"
	"errors"
	"expvar"
	"log"
	"net/http"
	"regexp"

	"code.google.com/p/go.crypto/openpgp"
	"strings"
	"time"
)

// The maximum amount of RAM to use when parsing
// mime object in requests.
var mimeMemoryBufferSize = int64(64000000)

// AptServer describes a web server
type AptServer struct {
	MaxReqs    int                // The maximum nuber of concurrent requests we'll handle
	CookieName string             // The session cookie name for uploads
	TTL        time.Duration      // How long to keep session alive
	PubRing    openpgp.EntityList // public keyring for checking changes files

	AcceptLoneDebs bool // Whether we should allow individual deb uploads

	Repo           AptRepo              // The repository to populate
	AptGenerator   AptGenerator         // The generator for updating the repo
	SessionManager UploadSessionManager // The session manager
	UpdateChannel  chan UpdateRequest   // A channel to recieve update requests
	Store          Storer               // We'll write files directly to this

	PreGenHook  HookRunner // A hook to run before we run the genrator
	PostGenHook HookRunner // A hooke to run after successful regeneration

	aptLocks *Governor // Locks to ensure the repo update is atomic

	uploadHandler   http.HandlerFunc // HTTP handler for upload requests
	downloadHandler http.HandlerFunc // HTTP handler for apt client downloads

	getCount *expvar.Int // Download count

}

// InitAptServer setups, and starts  a server.
func (a *AptServer) InitAptServer() {
	a.aptLocks = NewGovernor(a.MaxReqs)
	a.downloadHandler = a.makeDownloadHandler()
	a.uploadHandler = a.makeUploadHandler()

	a.getCount = expvar.NewInt("GetRequests")

	go a.Updater()
}

// Register this server with a HTTP server
func (a *AptServer) Register(mux *http.ServeMux) {
	mux.HandleFunc("/repo/", a.downloadHandler)
	mux.HandleFunc("/upload", a.uploadHandler)
	mux.HandleFunc("/upload/", a.uploadHandler)
}

// Construct the download handler for normal client downloads
func (a *AptServer) makeDownloadHandler() http.HandlerFunc {
	fsHandler := http.StripPrefix("/repo/", http.FileServer(http.Dir(a.Repo.Base())))
	return func(w http.ResponseWriter, r *http.Request) {
		a.aptLocks.ReadLock()
		defer a.aptLocks.ReadUnLock()

		log.Printf("%s %s %s %s", r.Method, r.Proto, r.URL.Path, r.RemoteAddr)
		a.getCount.Add(1)
		fsHandler.ServeHTTP(w, r)
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

// AptServerMessage contructs a new repsonse to a client and can take
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
	var reqRegex = regexp.MustCompile("^/upload(|/(.+))$")
	return func(w http.ResponseWriter, r *http.Request) {
		// Check the URL matches
		rest := reqRegex.FindStringSubmatch(r.URL.Path)
		if rest == nil {
			http.NotFound(w, r)
			return
		}

		found := false
		session := ""

		// Did we get a session in the URL
		if rest[1] != "" {
			session = rest[2]
			found = true
		}

		var resp AptServerResponder

		//Maybe in a cookie?
		if !found {
			cookie, err := r.Cookie(a.CookieName)
			if err == nil {
				session = cookie.Value
			}
		}

		switch r.Method {
		case "GET":
			{
				resp = a.SessionManager.Status(session)
			}
		case "PUT", "POST":
			{
				changesReader, otherParts, err := ChangesFromHTTPRequest(r)

				if err != nil {
					resp = AptServerMessage(http.StatusBadRequest, err.Error())
				} else {
					if session == "" {
						// We don't have an active session, lets create one
						var changes *ChangesFile
						if changesReader != nil {
							changes, err = ParseDebianChanges(changesReader, a.PubRing)
							if err != nil {
								resp = AptServerMessage(http.StatusBadRequest, err.Error())
							}
						} else {
							if !a.AcceptLoneDebs {
								err = errors.New("No debian changes file in request")
							} else {
								if len(otherParts) != 1 {
									err = errors.New("Too many files in upload request without changes file present")
								} else {
									if !strings.HasSuffix(otherParts[0].Filename, ".deb") {
										err = errors.New("Lone files for upload must end in .deb")
									}
									// No chnages file in the request, we need to create
									// a changes session based on the deb
									changes = &ChangesFile{
										loneDeb: true,
									}
									changes.Files = make(map[string]*ChangesItem)
									changes.Files[otherParts[0].Filename] = &ChangesItem{
										Filename: otherParts[0].Filename,
									}
								}
							}

						}
						session, err = a.SessionManager.Add(changes)
						if err != nil {
							resp = AptServerMessage(http.StatusBadRequest, err.Error())
						} else {
							cookie := http.Cookie{
								Name:     a.CookieName,
								Value:    session,
								Expires:  time.Now().Add(a.TTL),
								HttpOnly: false,
								Path:     "/upload",
							}
							http.SetCookie(w, &cookie)
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

// CompletedUpload describes a finished session, the details of the session,
// and the output of any hooks
type CompletedUpload struct {
	Session           UploadSessioner
	PreGenHookOutput  HookOutput
	PostGenHookOutput HookOutput
}

// MarshalJSON implements the json.Marshaler interface to allow
// presentation of a completed session to the user
func (s CompletedUpload) MarshalJSON() (j []byte, err error) {
	resp := struct {
		Session           UploadSessioner
		PreGenHookOutput  HookOutput
		PostGenHookOutput HookOutput
	}{
		s.Session,
		s.PreGenHookOutput,
		s.PostGenHookOutput,
	}
	j, err = json.Marshal(resp)
	return
}

// Updater ensures that updates to the repository are serialized.
// it reads from a channel of messages, responds to clients, and
// instigates the actual regernation of the repository
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

				a.aptLocks.WriteLock()

				hookResult := a.PreGenHook.Run(session.Directory())
				if hookResult.err != nil {
					respStatus = http.StatusBadRequest
					respObj = "Pre gen hook failed " + hookResult.Error()
				} else {
					completedsession.PreGenHookOutput = hookResult
				}

				respStatus, respObj, err = a.AptGenerator.AddSession(session)

				if err != nil {
					respStatus = http.StatusInternalServerError
					respObj = "Archive regeneration failed, " + err.Error()
				} else {
					hookResult := a.PostGenHook.Run(session.ID())
					completedsession.PostGenHookOutput = hookResult
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
