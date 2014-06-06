package main

//"crypto/md5"
//"github.com/stapelberg/godebiancontrol"

import (
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"

	"code.google.com/p/go.crypto/openpgp"
	"github.com/gorilla/mux"
)

var mimeMemoryBufferSize = int64(64000000)

type AptServer struct {
	MaxGets         int
	MaxPuts         int
	RepoDir         string
	TmpDir          string
	CookieName      string
	TTL             time.Duration
	ValidateChanges bool
	ValidateDebs    bool
	AftpPath        string
	AftpConfig      string
	ReleaseConfig   string
	PreAftpHook     string
	PostAftpHook    string

	getLocks        *Governor
	putLocks        *Governor
	aptLock         *Governor
	uploadHandler   http.HandlerFunc
	downloadHandler http.HandlerFunc
	sessMap         *SafeMap
	pubRing         openpgp.KeyRing
	privRing        openpgp.KeyRing
}

func (a *AptServer) InitAptServer() {
	a.getLocks, _ = NewGovernor(a.MaxGets)
	a.putLocks, _ = NewGovernor(a.MaxPuts)
	a.aptLock, _ = NewGovernor(1)

	a.downloadHandler = makeDownloadHandler(a)
	a.uploadHandler = makeUploadHandler(a)
	a.sessMap = NewSafeMap()
	pubringFile, _ := os.Open("pubring.gpg")
	a.pubRing, _ = openpgp.ReadKeyRing(pubringFile)
	privringFile, _ := os.Open("secring.gpg")
	a.privRing, _ = openpgp.ReadKeyRing(privringFile)
}

func (a *AptServer) Register(r *mux.Router) {
	r.HandleFunc("/repo/{rest:.*}", a.downloadHandler).Methods("GET")
	r.HandleFunc("/package/upload", a.uploadHandler).Methods("POST", "PUT")
	r.HandleFunc("/package/upload/{session}", a.uploadHandler).Methods("GET", "POST", "PUT")
}

func makeDownloadHandler(a *AptServer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a.getLocks.Run(func() {
			file := mux.Vars(r)["rest"]
			realFile := a.TmpDir + "/" + file
			log.Println("req'd " + realFile)
			http.ServeFile(w, r, realFile)
		})
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
		a.putLocks.Run(func() {
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
			if session == "" {
				dispatchRequest(a, &uploadSessionReq{"", w, r, true})
			} else {
				dispatchRequest(a, &uploadSessionReq{session, w, r, false})
			}
		})
	}
}

func dispatchRequest(a *AptServer, r *uploadSessionReq) {
	if r.create {
		err := r.R.ParseMultipartForm(mimeMemoryBufferSize)
		if err != nil {
			http.Error(r.W, err.Error(), http.StatusBadRequest)
			return
		}

		form := r.R.MultipartForm
		files := form.File["debfiles"]
		var changesPart multipart.File
		for _, f := range files {
			if strings.HasSuffix(f.Filename, ".changes") {
				changesPart, err = f.Open()
				break
			}
		}

		if changesPart == nil {
			http.Error(r.W, "No debian changes file in request", http.StatusBadRequest)
			return
		}

		changes, err := ParseDebianChanges(changesPart, &a.pubRing)
		if err != nil {
			http.Error(r.W, err.Error(), http.StatusBadRequest)
			return
		}

		if a.ValidateChanges && !changes.signed {
			http.Error(r.W, "Changes file was not signed", http.StatusBadRequest)
			return
		}

		if a.ValidateChanges && !changes.validated {
			http.Error(r.W, "Changes file could not be validated", http.StatusBadRequest)
			return
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

		r.W.WriteHeader(201)
		r.W.Write(UploadSessionToJSON(us))
		return

	} else {
		var us UploadSessioner
		c := a.sessMap.Get(r.SessionId)
		if c != nil {
			// Move this logic elseqhere
			switch sess := c.(type) {
			case UploadSessioner:
				us = sess
			default:
				http.Error(r.W, "Invalid session map entry", http.StatusInternalServerError)
				return
			}
		} else {
			log.Println("request for unknown session")
			http.NotFound(r.W, r.R)
		}

		switch r.R.Method {
		case "GET":
			{
				j := UploadSessionToJSON(us)
				r.W.Write(j)
				return
			}
		case "PUT", "POST":
			{
				//Add any files we have been passed
				err := r.R.ParseMultipartForm(mimeMemoryBufferSize)
				if err != nil {
					http.Error(r.W, err.Error(), http.StatusBadRequest)
					return
				}
				form := r.R.MultipartForm
				files := form.File["debfiles"]
				for _, f := range files {
					log.Println("Trying to upload: " + f.Filename)
					reader, err := f.Open()
					if err != nil {
						http.Error(r.W, "Can't upload "+f.Filename+" - "+err.Error(), http.StatusBadRequest)
						return
					}
					err = us.AddFile(&ChangesFile{
						Filename: f.Filename,
						data:     reader,
					})
					if err != nil {
						http.Error(r.W, "Can't upload "+f.Filename+" - "+err.Error(), http.StatusBadRequest)
						return
					}
				}

				complete := true
				for _, f := range us.Files() {
					if !f.Uploaded {
						complete = false
					}
				}

				if complete {
					// Need to trigger the upload
					r.W.WriteHeader(200)
					r.W.Write([]byte("File uploads complete"))
				} else {
					r.W.WriteHeader(202)
					r.W.Write([]byte("Feed me more files please"))
				}

				return
			}
		default:
			{
				http.Error(r.W, "unknown method", http.StatusBadRequest)
				return
			}
		}
	}
}
