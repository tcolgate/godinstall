package main

import (
	"encoding/json"
	"expvar"
	"log"
	"net/http"
	"strings"

	"golang.org/x/net/context"

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
	Lock           *Governor             // Locks to ensure the repo update is atomic
	getCount       *expvar.Int           // Download count
}

// appError is a custom error type to
// encode the HTTP status and meesage we will
// send back to a client
type appError struct {
	Code    int
	Message []byte
	Error   error
}

type appHandler func(context.Context, http.ResponseWriter, *http.Request) *appError

func (fn appHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if e := fn(ctx, w, r); e != nil { // e is *appError, not os.Error.
		if e.Code == 0 {
			e.Code = http.StatusInternalServerError
		}
		if e.Message == nil {
			e.Message = []byte(http.StatusText(e.Code))
		}
		log.Printf("ERROR: %v", e.Error)
		sendResponse(w, e.Code, e.Message)
	}
}

// NewServerResponse contructs a new repsonse to a client and can take
// a string of JSON'able object
func sendResponse(w http.ResponseWriter, code int, obj interface{}) *appError {
	var err error
	var j []byte

	if obj != nil {
		j, err = json.Marshal(obj)
		if err != nil {
			code = http.StatusInternalServerError
			j = []byte("{\"error\": \"Failed to marshal response, " + err.Error() + "\"}")
			w.WriteHeader(code)
			w.Write(j)
			return nil
		}
	} else {
		j, _ = json.Marshal(http.StatusText(code))
	}

	w.WriteHeader(code)
	w.Write(j)
	return nil
}

func sendOKResponse(w http.ResponseWriter, obj interface{}) *appError {
	return sendResponse(w, http.StatusOK, obj)
}

func handleWithReadLock(f appHandler, ctx context.Context, w http.ResponseWriter, r *http.Request) *appError {
	state.Lock.ReadLock()
	defer state.Lock.ReadUnLock()
	return f(ctx, w, r)
}

func handleWithWriteLock(f appHandler, ctx context.Context, w http.ResponseWriter, r *http.Request) *appError {
	state.Lock.WriteLock()
	defer state.Lock.WriteUnLock()
	return f(ctx, w, r)
}

func AuthorisedAdmin(ctx context.Context, w http.ResponseWriter, r *http.Request) bool {
	h := r.RemoteAddr[:strings.LastIndex(r.RemoteAddr, ":")]
	if !(h == "127.0.0.1" || h == "[::1]") {
		log.Printf("UNAUTHORIZED: %v %v", r.RemoteAddr, r.RequestURI)
		return false
	}
	return true
}
