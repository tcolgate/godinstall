package main

import (
	"encoding/json"
	"expvar"
	"log"
	"net/http"
	"strings"

	"time"
)

// aptServerConfig holds some global defaults for the server
var cfg struct {
	CookieName string        // The session cookie name for uploads
	TTL        time.Duration // How long to keep session alive

	PreGenHook  HookRunner // A hook to run before we run the genrator
	PostGenHook HookRunner // A hooke to run after successful regeneration
}

var state struct {
	Archive        Archiver              // The generator for updating the repo
	SessionManager *UploadSessionManager // The session manager
	UpdateChannel  chan UpdateRequest    // A channel to recieve update requests
	Lock           *Governor             // Locks to ensure the repo update is atomic
	getCount       *expvar.Int           // Download count
}

// ServerResponse is a custom error type to
// encode the HTTP status and meesage we will
// send back to a client
type ServerResponse struct {
	StatusCode int
	Message    []byte
}

func (r ServerResponse) Error() string {
	return "ERROR: " + string(r.Message)
}

// NewServerResponse contructs a new repsonse to a client and can take
// a string of JSON'able object
func NewServerResponse(status int, msg interface{}) *ServerResponse {
	var err error
	var j []byte

	resp := ServerResponse{
		StatusCode: status,
	}

	j, err = json.Marshal(msg)
	if err != nil {
		resp.StatusCode = http.StatusInternalServerError
		resp.Message = []byte("failed to marshal response, " + err.Error())
	} else {
		resp.Message = j
	}

	return &resp
}

func SendResponse(w http.ResponseWriter, msg *ServerResponse) {
	if len(msg.Message) == 0 {
		msg.Message = []byte(http.StatusText(msg.StatusCode))
	}
	if msg.StatusCode >= 400 {
		log.Println(msg.Error())
	}
	w.WriteHeader(msg.StatusCode)
	w.Write(msg.Message)
}

func SendOKResponse(w http.ResponseWriter, obj interface{}) {
	msg := NewServerResponse(http.StatusOK, obj)
	SendResponse(w, msg)
}

func SendDefaultResponse(w http.ResponseWriter, status int) {
	SendResponse(w, NewServerResponse(status, http.StatusText(status)))
}

func SendOKOrErrorResponse(w http.ResponseWriter, obj interface{}, err error, errStatus int) {
	var msg *ServerResponse
	if err != nil {
		msg = NewServerResponse(errStatus, err.Error())
	} else {
		msg = NewServerResponse(http.StatusOK, obj)
	}
	SendResponse(w, msg)
}

func handleWithReadLock(f http.HandlerFunc, w http.ResponseWriter, r *http.Request) {
	state.Lock.ReadLock()
	defer state.Lock.ReadUnLock()
	f(w, r)
}

func handleWithWriteLock(f http.HandlerFunc, w http.ResponseWriter, r *http.Request) {
	state.Lock.WriteLock()
	defer state.Lock.WriteUnLock()
	f(w, r)
}

func AuthorisedAdmin(w http.ResponseWriter, r *http.Request) bool {
	h := r.RemoteAddr[:strings.LastIndex(r.RemoteAddr, ":")]
	if !(h == "127.0.0.1" || h == "::1") {
		log.Printf("UNAUTHORIZED: %v %v", r.RemoteAddr, r.RequestURI)
		SendDefaultResponse(w, http.StatusUnauthorized)
		return false
	}
	return true
}
