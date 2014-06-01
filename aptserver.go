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

	"code.google.com/p/go-uuid/uuid"
	"code.google.com/p/go.crypto/openpgp"
	"github.com/gorilla/mux"
)

type AptServer struct {
	MaxGets    int
	MaxPuts    int
	RepoDir    string
	TmpDir     string
	CookieName string
	TTL        time.Duration

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

			if session == "" {
				session := uuid.New()
				dispatchRequest(a, &uploadSessionReq{session, w, r, true})
			} else {
				dispatchRequest(a, &uploadSessionReq{session, w, r, false})
			}
		})
	}
}

func dispatchRequest(a *AptServer, r *uploadSessionReq) {
	if r.create {
		err := r.R.ParseMultipartForm(64000000)
		if err != nil {
			http.Error(r.W, err.Error(), http.StatusInternalServerError)
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
			http.Error(r.W, "No debian changes file in request", http.StatusInternalServerError)
			return
		}

		changes, err := ParseDebianChanges(changesPart, &a.pubRing)
		if err != nil {
			http.Error(r.W, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Printf("%q", changes)

		// This should probably move into the upload session constructor
		s := r.SessionId
		us := a.NewUploadSession(s)
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

		// I should write a JSON reponse here with the session Id
		r.W.Write([]byte("Upload session created: " + s))
		return

	} else {
		c := a.sessMap.Get(r.SessionId)
		if c != nil {
			// Move this logic elseqhere
			switch sess := c.(type) {
			case *uploadSession:
				sess.HandleReq(r.W, r.R)
			default:
				log.Println("Shouldn't get here")
			}
		} else {
			log.Println("request for unknown session")
			http.NotFound(r.W, r.R)
		}
	}
}
