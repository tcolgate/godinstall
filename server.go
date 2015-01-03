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

	"code.google.com/p/go.crypto/openpgp"

	"strings"
	"time"

	"github.com/codegangsta/cli"
	"github.com/gorilla/mux"
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

// Construct the download handler for normal client downloads

func makeDownloadHandler() http.HandlerFunc {
	fsHandler := http.StripPrefix("/repo/", http.FileServer(http.Dir(state.Archive.PublicDir())))
	downloadHandler := func(w http.ResponseWriter, r *http.Request) {
		handleWithReadLock(fsHandler.ServeHTTP, w, r)
		log.Printf("%s %s %s %s", r.Method, r.Proto, r.URL.Path, r.RemoteAddr)
		state.getCount.Add(1)
	}
	return downloadHandler
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

func SendOKOrErrorResponse(w http.ResponseWriter, obj interface{}, err error, errStatus int) {
	var msg *ServerResponse
	if err != nil {
		msg = NewServerResponse(errStatus, err.Error())
	} else {
		msg = NewServerResponse(http.StatusOK, obj)
	}
	SendResponse(w, msg)
}

// This build a function to despatch upload requests
func uploadHandler(w http.ResponseWriter, r *http.Request) {

	vars := mux.Vars(r)

	branchName, ok := vars["name"]
	if !ok {
		branchName = "master"
	}

	session, found := vars["session"]

	var resp *ServerResponse

	//Maybe in a cookie?
	if !found {
		cookie, err := r.Cookie(cfg.CookieName)
		if err == nil {
			session = cookie.Value
		}
	}

	switch r.Method {
	case "GET":
		{
			s, ok := state.SessionManager.GetSession(session)
			if !ok {
				resp = NewServerResponse(http.StatusNotFound, "File not found")
				w.WriteHeader(resp.StatusCode)
				w.Write(resp.Message)
				return
			}

			resp = s.Status()
		}
	case "PUT", "POST":
		{
			changesReader, otherParts, err := ChangesFromHTTPRequest(r)
			if err != nil {
				resp = NewServerResponse(http.StatusBadRequest, err.Error())
				w.WriteHeader(resp.StatusCode)
				w.Write(resp.Message)
				return
			}

			if session == "" {
				// We don't have an active session, lets create one
				rel, err := state.Archive.GetDist(branchName)
				if err != nil {
					w.WriteHeader(http.StatusNotFound)
					w.Write([]byte("Unknown distribution " + branchName + ", " + err.Error()))
					return
				}

				var loneDeb bool
				if changesReader == nil {
					if !rel.Config().AcceptLoneDebs {
						err = errors.New("No debian changes file in request")
						resp = NewServerResponse(http.StatusBadRequest, err.Error())
						w.WriteHeader(resp.StatusCode)
						w.Write(resp.Message)
						return
					}

					if len(otherParts) != 1 {
						err = errors.New("Too many files in upload request without changes file present")
						resp = NewServerResponse(http.StatusBadRequest, err.Error())
						w.WriteHeader(resp.StatusCode)
						w.Write(resp.Message)
						return
					}

					if !strings.HasSuffix(otherParts[0].Filename, ".deb") {
						err = errors.New("Lone files for upload must end in .deb")
						resp = NewServerResponse(http.StatusBadRequest, err.Error())
						w.WriteHeader(resp.StatusCode)
						w.Write(resp.Message)
						return
					}

					loneDeb = true
				}

				session, err = state.SessionManager.NewSession(rel, changesReader, loneDeb)
				if err != nil {
					resp = NewServerResponse(http.StatusBadRequest, err.Error())
					w.WriteHeader(resp.StatusCode)
					w.Write(resp.Message)
					return
				}

				cookie := http.Cookie{
					Name:     cfg.CookieName,
					Value:    session,
					Expires:  time.Now().Add(cfg.TTL),
					HttpOnly: false,
					Path:     "/upload",
				}
				http.SetCookie(w, &cookie)
			}
			if err != nil {
				resp = NewServerResponse(http.StatusBadRequest, err.Error())
				w.WriteHeader(resp.StatusCode)
				w.Write(resp.Message)
				return
			}

			sess, ok := state.SessionManager.GetSession(session)
			if !ok {
				resp = NewServerResponse(http.StatusNotFound, "File Not Found")
				w.WriteHeader(resp.StatusCode)
				w.Write(resp.Message)
				return
			}

			resp = sess.Status()

			for _, part := range otherParts {
				fh, err := part.Open()
				if err != nil {
					resp = NewServerResponse(http.StatusBadRequest, fmt.Sprintf("Error opening mime item, %s", err.Error()))
					w.WriteHeader(resp.StatusCode)
					w.Write(resp.Message)
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

	if resp.StatusCode == 0 {
		http.Error(w, "AptServer response statuscode not set", http.StatusInternalServerError)
	} else {
		w.WriteHeader(resp.StatusCode)
		w.Write(resp.Message)
	}
}

func logHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	curr, err := state.Archive.GetDist(name)
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

		curr, err = state.Archive.GetRelease(curr.ParentID)
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

// This build a function to enumerate the distributions
func distsHandler(w http.ResponseWriter, r *http.Request) {

	switch r.Method {
	case "GET":
		vars := mux.Vars(r)
		name, nameGiven := vars["name"]
		dists := state.Archive.Dists()
		if !nameGiven {
			SendOKResponse(w, dists)
		} else {
			_, ok := dists[name]
			if !ok {
				http.NotFound(w, r)
				return
			}
			rel, err := state.Archive.GetDist(name)
			SendOKOrErrorResponse(w, rel, err, http.StatusInternalServerError)
		}
	case "PUT":
		vars := mux.Vars(r)
		name, _ := vars["name"]
		dists := state.Archive.Dists()
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
			rootid, err := state.Archive.GetReleaseRoot(seedrel)
			if err != nil {
				http.Error(w,
					"Get reelase root failed"+err.Error(),
					http.StatusInternalServerError)
				return
			}

			emptyidx, err := state.Archive.EmptyReleaseIndex()
			if err != nil {
				http.Error(w,
					"Get rempty release index failed"+err.Error(),
					http.StatusInternalServerError)
				return
			}
			newrelid, err := NewRelease(state.Archive, rootid, emptyidx, []ReleaseLogAction{})
			if err != nil {
				http.Error(w,
					"Create new release failed, "+err.Error(),
					http.StatusInternalServerError)
				return
			}
			err = state.Archive.SetDist(name, newrelid)
			if err != nil {
				http.Error(w,
					"Setting new dist tag failed, "+err.Error(),
					http.StatusInternalServerError)
				return
			}
			rel, err = state.Archive.GetDist(name)
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
	case "DELETE":
		vars := mux.Vars(r)
		name, nameGiven := vars["name"]
		dists := state.Archive.Dists()
		if !nameGiven {
			http.Error(w,
				http.StatusText(http.StatusMethodNotAllowed),
				http.StatusMethodNotAllowed)
		} else {
			_, ok := dists[name]
			if !ok {
				http.NotFound(w, r)
				return
			}
			err := state.Archive.DeleteDist(name)
			if err != nil {
				http.Error(w,
					"failed to retrieve distribution details, "+err.Error(),
					http.StatusInternalServerError)
			}
		}
	default:
		http.Error(w,
			http.StatusText(http.StatusMethodNotAllowed),
			http.StatusMethodNotAllowed)
	}
	return
}

// This build a function to manage the config of a distribution
func configHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	rel, err := state.Archive.GetDist(name)
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

// This build a function to manage the config of a distribution
func configSigningKeyHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	rel, err := state.Archive.GetDist(name)
	if err != nil {
		http.Error(w,
			fmt.Sprintf("distribution %v not found, %s", name, err.Error()),
			http.StatusNotFound)
		return
	}

	switch r.Method {
	case "GET":
		id, err := rel.SignerKey()
		if err != nil {
			http.Error(w,
				fmt.Sprintf("\"Error retrieving signing key\"", name, err.Error()),
				http.StatusNotFound)
			return
		}

		if id == nil {
			http.Error(w,
				fmt.Sprintf("\"No signing key set\""),
				http.StatusNotFound)
			return
		}

		key := id.PrimaryKey
		if key == nil {
			http.Error(w,
				fmt.Sprintf("\"No signing key set\""),
				http.StatusNotFound)
			return
		}
		w.Write([]byte("\"" + key.KeyIdString() + "\""))
		return

	case "DELETE":
		http.Error(w,
			http.StatusText(http.StatusMethodNotAllowed),
			http.StatusMethodNotAllowed)
	case "PUT", "POST":
		id, err := state.Archive.CopyToStore(r.Body)
		if err != nil {
			http.Error(w,
				"failed to copy key to store, "+err.Error(),
				http.StatusInternalServerError)
			return
		}
		rdr, err := state.Archive.Open(id)
		kr, err := openpgp.ReadArmoredKeyRing(rdr)
		if err != nil {
			http.Error(w,
				"failed to parse data as  key, "+err.Error(),
				http.StatusInternalServerError)
			return
		}

		log.Println(kr[0])
	default:
		http.Error(w,
			http.StatusText(http.StatusMethodNotAllowed),
			http.StatusMethodNotAllowed)
	}
	return
}

// For managing public keys in a config
func configPublicKeysHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]
	reqid := vars["id"]

	rel, err := state.Archive.GetDist(name)
	if err != nil {
		http.Error(w,
			fmt.Sprintf("distribution %v not found, %s", name, err.Error()),
			http.StatusNotFound)
		return
	}

	switch r.Method {
	case "GET":
		ids, err := rel.PubRing()
		if err != nil {
			http.Error(w,
				fmt.Sprintf("\"Error retrieving signing keys\"", name, err.Error()),
				http.StatusNotFound)
			return
		}
		if reqid == "" {
			var keyids []string
			for _, id := range ids {
				key := id.PrimaryKey
				if key != nil {
					keyids = append(keyids, "\""+key.KeyIdString()+"\"")
				}
			}
			w.Write([]byte("[" + strings.Join(keyids, ",") + "]"))
		} else {
			found := false
			output := ""
			for _, id := range ids {
				key := id.PrimaryKey
				if key.KeyIdString() == reqid {
					output = reqid
				}
			}
			if found {
				w.Write([]byte(output))
			} else {
				http.NotFound(w, r)
			}
		}

		return
	case "DELETE":
		http.Error(w,
			http.StatusText(http.StatusMethodNotAllowed),
			http.StatusMethodNotAllowed)
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

// Updater ensures that updates to the repository are serialized.
// it reads from a channel of messages, responds to clients, and
// instigates the actual regernation of the repository
func updater() {
	for {
		select {
		case msg := <-state.UpdateChannel:
			{
				var err error
				respStatus := http.StatusOK
				var respObj interface{}

				session := msg.session
				completedsession := CompletedUpload{UploadSession: session}

				state.Lock.WriteLock()

				hookResult := cfg.PreGenHook.Run(session.Directory())
				if hookResult.err != nil {
					respStatus = http.StatusBadRequest
					respObj = "Pre gen hook failed " + hookResult.Error()
				} else {
					completedsession.PreGenHookOutput = hookResult
				}

				respStatus, respObj, err = state.Archive.AddUpload(session)
				if err == nil {
					hookResult := cfg.PostGenHook.Run(session.ID())
					completedsession.PostGenHookOutput = hookResult
				}

				state.Lock.WriteUnLock()

				if respStatus == http.StatusOK {
					respObj = completedsession
				}

				msg.resp <- NewServerResponse(respStatus, respObj)
			}
		}
	}
}

// UpdateRequest contains the information needed to
// request an update, only regeneration is supported
// at present
type UpdateRequest struct {
	resp    chan *ServerResponse
	session *UploadSession
}

// CmdServe is the implementation of the godinstall "serve" command
func CmdServe(c *cli.Context) {
	listenAddress := c.String("listen")
	//	sslListenAddress := c.String("listen-ssl")
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

	state.Archive = NewAptBlobArchive(
		&storeDir,
		&tmpDir,
		&publicDir,
		ReleaseConfig{
			VerifyChanges:           verifyChanges,
			VerifyChangesSufficient: verifyChangesSufficient,
			VerifyDebs:              verifyDebs,
			AcceptLoneDebs:          acceptLoneDebs,
			PruneRules:              pruneRulesStr,
			AutoTrim:                autoTrim,
			AutoTrimLength:          trimLen,
			PoolPattern:             poolPattern,
		},
	)

	state.UpdateChannel = make(chan UpdateRequest)
	state.SessionManager = NewUploadSessionManager(
		expire,
		&tmpDir,
		state.Archive,
		NewScriptHook(&uploadHook),
		state.UpdateChannel,
	)

	cfg.CookieName = cookieName
	cfg.PreGenHook = NewScriptHook(&preGenHook)
	cfg.PostGenHook = NewScriptHook(&postGenHook)

	state.Lock = NewGovernor(maxReqs)
	state.getCount = expvar.NewInt("GetRequests")

	go updater()

	r := mux.NewRouter()

	// We'll hook up all the normal debug business
	r.HandleFunc("/debug/pprof/", pprof.Index)
	r.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	r.HandleFunc("/debug/pprof/profile", pprof.Profile)
	r.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	r.Handle("/debug/vars", http.DefaultServeMux)

	r.PathPrefix("/repo/").HandlerFunc(makeDownloadHandler())
	r.PathPrefix("/upload").HandlerFunc(uploadHandler)

	r.HandleFunc("/dists", distsHandler)
	r.HandleFunc("/dists/{name}", distsHandler)
	r.HandleFunc("/dists/{name}/config", configHandler)
	r.HandleFunc("/dists/{name}/config/signingkey", configSigningKeyHandler)
	r.HandleFunc("/dists/{name}/config/publickeys", configPublicKeysHandler)
	r.HandleFunc("/dists/{name}/config/publickeys/{id}", configPublicKeysHandler)
	r.HandleFunc("/dists/{name}/log", logHandler)
	r.HandleFunc("/dists/{name}/upload", uploadHandler)
	r.HandleFunc("/dists/{name}/upload/{session}", uploadHandler)

	http.ListenAndServe(listenAddress, r)
	//	http.ListenAndServeTLS(sslListenAddress, r)
}
