package main

import (
	"encoding/json"
	"errors"
	"expvar"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/pprof"
	"os"
	"regexp"

	"strings"
	"time"

	"github.com/codegangsta/cli"
	"github.com/gorilla/mux"
)

// AptServer describes a web server
type AptServer struct {
	MaxReqs    int           // The maximum nuber of concurrent requests we'll handle
	CookieName string        // The session cookie name for uploads
	TTL        time.Duration // How long to keep session alive

	Archive        Archiver              // The generator for updating the repo
	SessionManager *UploadSessionManager // The session manager
	UpdateChannel  chan UpdateRequest    // A channel to recieve update requests

	PreGenHook  HookRunner // A hook to run before we run the genrator
	PostGenHook HookRunner // A hooke to run after successful regeneration

	DefaultReleaseConfig ReleaseConfig //

	aptLocks *Governor // Locks to ensure the repo update is atomic

	downloadHandler http.HandlerFunc // HTTP handler for apt client downloads
	distsHandler    http.HandlerFunc // HTTP handler for exposing the logs
	logHandler      http.HandlerFunc // HTTP handler for exposing the logs
	uploadHandler   http.HandlerFunc // HTTP handler for upload requests
	configHandler   http.HandlerFunc // HTTP handler for upload requests

	getCount *expvar.Int // Download count
}

// InitAptServer setups, and starts  a server.
func (a *AptServer) InitAptServer() {
	a.aptLocks = NewGovernor(a.MaxReqs)
	a.downloadHandler = a.makeDownloadHandler()
	a.distsHandler = a.makeDistsHandler()
	a.uploadHandler = a.makeUploadHandler()
	a.logHandler = a.makeLogHandler()
	a.configHandler = a.makeConfigHandler()

	a.getCount = expvar.NewInt("GetRequests")

	go a.Updater()
}

// Register this server with a HTTP server
func (a *AptServer) Register(r *mux.Router) {
	r.PathPrefix("/repo/").HandlerFunc(a.downloadHandler)
	r.PathPrefix("/upload").HandlerFunc(a.uploadHandler)

	r.HandleFunc("/dists", a.distsHandler)
	r.HandleFunc("/dists/{name}", a.distsHandler)
	r.HandleFunc("/dists/{name}/config", a.configHandler)
	r.HandleFunc("/dists/{name}/log", a.logHandler)
	r.HandleFunc("/dists/{name}/upload", a.uploadHandler)
	r.HandleFunc("/dists/{name}/upload/{session}", a.uploadHandler)
}

// Construct the download handler for normal client downloads
func (a *AptServer) makeDownloadHandler() http.HandlerFunc {
	fsHandler := http.StripPrefix("/repo/", http.FileServer(http.Dir(a.Archive.PublicDir())))
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
			j, err = json.Marshal(t)
			resp.message = j
		}
	case string:
		{
			j, err = json.Marshal(
				struct {
					Message string
				}{
					t,
				})
			resp.message = j
		}
	default:
		{
			j, err = json.Marshal(
				struct {
					Message string
				}{
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

		vars := mux.Vars(r)

		branchName, ok := vars["name"]
		if !ok {
			branchName = "master"
		}

		session, found := vars["session"]

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
				s, ok := a.SessionManager.GetSession(session)
				if !ok {
					resp = AptServerMessage(http.StatusNotFound, "File not found")
					w.WriteHeader(resp.GetStatus())
					w.Write(resp.GetMessage())
					return
				}

				resp = s.Status()
			}
		case "PUT", "POST":
			{
				changesReader, otherParts, err := ChangesFromHTTPRequest(r)
				if err != nil {
					resp = AptServerMessage(http.StatusBadRequest, err.Error())
					w.WriteHeader(resp.GetStatus())
					w.Write(resp.GetMessage())
					return
				}

				if session == "" {
					// We don't have an active session, lets create one
					rel, err := a.Archive.GetDist(branchName)
					if err != nil {
						w.WriteHeader(http.StatusNotFound)
						w.Write([]byte("Unknown distribution " + branchName + ", " + err.Error()))
						return
					}

					var loneDeb bool
					if changesReader == nil {
						if !rel.Config().AcceptLoneDebs {
							err = errors.New("No debian changes file in request")
							resp = AptServerMessage(http.StatusBadRequest, err.Error())
							w.WriteHeader(resp.GetStatus())
							w.Write(resp.GetMessage())
							return
						}

						if len(otherParts) != 1 {
							err = errors.New("Too many files in upload request without changes file present")
							resp = AptServerMessage(http.StatusBadRequest, err.Error())
							w.WriteHeader(resp.GetStatus())
							w.Write(resp.GetMessage())
							return
						}

						if !strings.HasSuffix(otherParts[0].Filename, ".deb") {
							err = errors.New("Lone files for upload must end in .deb")
							resp = AptServerMessage(http.StatusBadRequest, err.Error())
							w.WriteHeader(resp.GetStatus())
							w.Write(resp.GetMessage())
							return
						}

						loneDeb = true
					}

					session, err = a.SessionManager.NewSession(rel, changesReader, loneDeb)
					if err != nil {
						resp = AptServerMessage(http.StatusBadRequest, err.Error())
						w.WriteHeader(resp.GetStatus())
						w.Write(resp.GetMessage())
						return
					}

					cookie := http.Cookie{
						Name:     a.CookieName,
						Value:    session,
						Expires:  time.Now().Add(a.TTL),
						HttpOnly: false,
						Path:     "/upload",
					}
					http.SetCookie(w, &cookie)
				}
				if err != nil {
					resp = AptServerMessage(http.StatusBadRequest, err.Error())
					w.WriteHeader(resp.GetStatus())
					w.Write(resp.GetMessage())
					return
				}

				sess, ok := a.SessionManager.GetSession(session)
				if !ok {
					resp = AptServerMessage(http.StatusNotFound, "File Not Found")
					w.WriteHeader(resp.GetStatus())
					w.Write(resp.GetMessage())
					return
				}

				resp = sess.Status()

				for _, part := range otherParts {
					fh, err := part.Open()
					if err != nil {
						resp = AptServerMessage(http.StatusBadRequest, fmt.Sprintf("Error opening mime item, %s", err.Error()))
						w.WriteHeader(resp.GetStatus())
						w.Write(resp.GetMessage())
						return
					}

					uf := UploadFile{
						Name:   part.Filename,
						reader: fh,
					}
					resp = sess.AddFile(&uf)
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

func (a *AptServer) makeLogHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		name := vars["name"]

		curr, err := a.Archive.GetDist(name)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("failed to retrieve store reference for distribution " + name + ", " + err.Error()))
			return
		}

		w.Write([]byte("["))
		defer w.Write([]byte("]"))

		displayTrimmed := false
		trimmerActive := false
		trimAfter := int32(0)

		for {
			output, err := json.Marshal(curr)
			if err != nil {
				log.Println("Could not marshal json object, " + err.Error())
				return
			}
			w.Write(output)

			if !displayTrimmed {
				if !trimmerActive && curr.TrimAfter != 0 {
					trimmerActive = true
					trimAfter = curr.TrimAfter
				}
			}

			curr, err = a.Archive.GetRelease(curr.ParentID)
			if err != nil {
				log.Println("Could not get parent, " + err.Error())
				return
			}

			if curr.ParentID != nil {
				if !displayTrimmed {
					if trimmerActive {
						if trimAfter > 0 {
							trimAfter--
						} else {
							// Stop displaying history here
							return
						}
					}
				}
				w.Write([]byte(","))
			} else {
				return
			}
		}
	}
}

// This build a function to enumerate the distributions
func (a *AptServer) makeDistsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		name, nameGiven := vars["name"]
		dists := a.Archive.Dists()

		switch r.Method {
		case "GET":
			if !nameGiven {
				output, err := json.Marshal(dists)
				if err != nil {
					http.Error(w,
						"failed to retrieve list of distributions, "+err.Error(),
						http.StatusInternalServerError)
				}
				w.Write(output)
			} else {
				_, ok := dists[name]
				if !ok {
					http.NotFound(w, r)
					return
				}
				rel, err := a.Archive.GetDist(name)
				output, err := json.Marshal(rel)
				if err != nil {
					http.Error(w,
						"failed to retrieve distribution details, "+err.Error(),
						http.StatusInternalServerError)
				}
				w.Write(output)
			}
		case "PUT":
			var rel *Release
			var err error

			_, exists := dists[name]
			if exists {
				http.Error(w,
					"cannot update release directly",
					http.StatusConflict)
			} else {
				seedrel := Release{
					Suite: name,
				}
				rootid, err := a.Archive.GetReleaseRoot(seedrel)
				if err != nil {
					http.Error(w,
						"Get reelase root failed"+err.Error(),
						http.StatusInternalServerError)
					return
				}

				emptyidx, err := a.Archive.EmptyReleaseIndex()
				if err != nil {
					http.Error(w,
						"Get rempty release index failed"+err.Error(),
						http.StatusInternalServerError)
					return
				}
				newrelid, err := NewRelease(a.Archive, rootid, emptyidx, []ReleaseLogAction{})
				if err != nil {
					http.Error(w,
						"Create new release failed, "+err.Error(),
						http.StatusInternalServerError)
					return
				}
				err = a.Archive.SetDist(name, newrelid)
				if err != nil {
					http.Error(w,
						"Setting new dist tag failed, "+err.Error(),
						http.StatusInternalServerError)
					return
				}
				rel, err = a.Archive.GetDist(name)
				if err != nil {
					http.Error(w,
						"retrieving new dist tag failed, "+err.Error(),
						http.StatusInternalServerError)
					return
				}
			}

			output, err := json.Marshal(rel)
			if err != nil {
				http.Error(w,
					"failed to retrieve distribution details, "+err.Error(),
					http.StatusInternalServerError)
			}
			w.Write(output)
		default:
			http.Error(w,
				http.StatusText(http.StatusMethodNotAllowed),
				http.StatusMethodNotAllowed)
		}
		return
	}
}

// This build a function to manage the config of a distribution
func (a *AptServer) makeConfigHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		name := vars["name"]

		rel, err := a.Archive.GetDist(name)
		if err != nil {
			http.Error(w,
				fmt.Sprintf("distribution %v not found, %s", name, err.Error()),
				http.StatusNotFound)
			return
		}

		switch r.Method {
		case "GET":
			output, err := json.Marshal(rel.Config())
			if err != nil {
				http.Error(w,
					"failed to marshal release config, "+err.Error(),
					http.StatusInternalServerError)
			}

			w.Write(output)
		case "PUT", "POST":
			http.Error(w,
				http.StatusText(http.StatusMethodNotAllowed),
				http.StatusMethodNotAllowed)
		default:
			http.Error(w,
				http.StatusText(http.StatusMethodNotAllowed),
				http.StatusMethodNotAllowed)
		}
		return
	}
}

// CompletedUpload describes a finished session, the details of the session,
// and the output of any hooks
type CompletedUpload struct {
	*UploadSession
	PreGenHookOutput  HookOutput
	PostGenHookOutput HookOutput
}

// MarshalJSON implements the json.Marshaler interface to allow
// presentation of a completed session to the user
func (s CompletedUpload) MarshalJSON() (j []byte, err error) {
	resp := struct {
		UploadSession
		PreGenHookOutput  HookOutput
		PostGenHookOutput HookOutput
	}{
		*s.UploadSession,
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
				completedsession := CompletedUpload{UploadSession: session}

				a.aptLocks.WriteLock()

				hookResult := a.PreGenHook.Run(session.Directory())
				if hookResult.err != nil {
					respStatus = http.StatusBadRequest
					respObj = "Pre gen hook failed " + hookResult.Error()
				} else {
					completedsession.PreGenHookOutput = hookResult
				}

				respStatus, respObj, err = a.Archive.AddUpload(session)
				if err == nil {
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
	session *UploadSession
}

// CmdServe is the implementation of the godinstall "serve" command
func CmdServe(c *cli.Context) {
	// Setup CLI flags
	listenAddress := c.String("listen")
	ttl := c.String("ttl")
	maxReqs := c.Int("max-requests")
	repoBase := c.String("repo-base")
	cookieName := c.String("cookie-name")
	uploadHook := c.String("upload-hook")
	preGenHook := c.String("pre-gen-hook")
	postGenHook := c.String("post-gen-hook")
	poolPattern := c.String("default-pool-pattern")
	verifyChanges := c.Bool("default-verify-changes")
	verifyChangesSufficient := c.Bool("default-verify-changes-sufficient")
	acceptLoneDebs := c.Bool("default-accept-lone-debs")
	verifyDebs := c.Bool("default-verify-debs")
	pruneRulesStr := c.String("default-prune")
	autoTrim := c.Bool("default-auto-trim")
	trimLen := c.Int("default-auto-trim-length")

	flag.Parse()

	if repoBase == "" {
		log.Println("You must pass --repo-base")
		return
	}

	expire, err := time.ParseDuration(ttl)
	if err != nil {
		log.Println(err.Error())
		return
	}

	updateChan := make(chan UpdateRequest)

	storeDir := repoBase + "/store"
	tmpDir := repoBase + "/tmp"
	publicDir := repoBase + "/archive"

	_, patherr := os.Stat(publicDir)
	if os.IsNotExist(patherr) {
		err = os.Mkdir(publicDir, 0777)
		if err != nil {
			log.Println(err.Error())
			return
		}
	}
	_, patherr = os.Stat(storeDir)
	if os.IsNotExist(patherr) {
		err = os.Mkdir(storeDir, 0777)
		if err != nil {
			log.Println(err.Error())
			return
		}
	}

	_, patherr = os.Stat(tmpDir)
	if os.IsNotExist(patherr) {
		err = os.Mkdir(tmpDir, 0777)
		if err != nil {
			log.Println(err.Error())
			return
		}
	}

	_, err = ParsePruneRules(pruneRulesStr)
	if err != nil {
		log.Println(err.Error())
		return
	}

	// We make sure the default pool pattern is a valid rege
	_, err = regexp.CompilePOSIX("^(" + poolPattern + ")")
	if err != nil {
		log.Println(err.Error())
		return
	}

	defRelConfig := ReleaseConfig{
		VerifyChanges:           verifyChanges,
		VerifyChangesSufficient: verifyChangesSufficient,
		VerifyDebs:              verifyDebs,
		AcceptLoneDebs:          acceptLoneDebs,
		PruneRules:              pruneRulesStr,
		AutoTrim:                autoTrim,
		AutoTrimLength:          trimLen,
		PoolPattern:             poolPattern,
	}

	archive := NewAptBlobArchive(
		&storeDir,
		&tmpDir,
		&publicDir,
		defRelConfig,
	)

	uploadSessionManager := NewUploadSessionManager(
		expire,
		&tmpDir,
		archive,
		NewScriptHook(&uploadHook),
		updateChan,
	)

	server := &AptServer{
		MaxReqs:     maxReqs,
		CookieName:  cookieName,
		PreGenHook:  NewScriptHook(&preGenHook),
		PostGenHook: NewScriptHook(&postGenHook),

		Archive:        archive,
		SessionManager: uploadSessionManager,
		UpdateChannel:  updateChan,

		DefaultReleaseConfig: defRelConfig,
	}

	r := mux.NewRouter()

	// We'll hook up all the normal debug business
	r.HandleFunc("/debug/pprof/", pprof.Index)
	r.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	r.HandleFunc("/debug/pprof/profile", pprof.Profile)
	r.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	r.Handle("/debug/vars", http.DefaultServeMux)

	server.InitAptServer()
	server.Register(r)

	http.ListenAndServe(listenAddress, r)
}
