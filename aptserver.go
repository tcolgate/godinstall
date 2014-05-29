package main

//"crypto/md5"
//"github.com/stapelberg/godebiancontrol"

import (
	"code.google.com/p/go-uuid/uuid"
	"github.com/gorilla/mux"
	"log"
	"net/http"
	"os"
	"time"
)

type AptServer struct {
	MaxGets    int
	MaxPuts    int
	RepoDir    string
	TmpDir     string
	CookieName string
	TTL        time.Duration

	getLocks        chan int
	putLocks        chan int
	aptLock         chan int
	uploadHandler   http.HandlerFunc
	downloadHandler http.HandlerFunc
}

func (a *AptServer) InitAptServer() {
	a.getLocks = make(chan int, a.MaxGets)
	for i := 0; i < a.MaxGets; i++ {
		a.getLocks <- 1
	}

	a.putLocks = make(chan int, a.MaxPuts)
	for i := 0; i < a.MaxPuts; i++ {
		a.putLocks <- 1
	}

	a.aptLock = make(chan int, 1)
	a.aptLock <- 1
	a.downloadHandler = makeDownloadHandler(a)
	a.uploadHandler = makeUploadHandler(a)
}

func (a *AptServer) Register(r *mux.Router) {
	r.HandleFunc("/repo/{rest:.*}", a.downloadHandler).Methods("GET")
	r.HandleFunc("/package/upload", a.uploadHandler).Methods("POST", "PUT")
	r.HandleFunc("/package/upload/{session}", a.uploadHandler).Methods("POST", "PUT")
}

func makeDownloadHandler(a *AptServer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lock := <-a.getLocks
		defer func() { a.getLocks <- lock }()

		file := mux.Vars(r)["rest"]
		realFile := a.TmpDir + "/" + file

		log.Println("req'd " + realFile)

		http.ServeFile(w, r, realFile)
	}
}

type uploadSessionReq struct {
	SessionId string
	W         http.ResponseWriter
	R         *http.Request
	create    bool // This is a request to create a new upload session
}

func makeUploadHandler(a *AptServer) (f func(w http.ResponseWriter, r *http.Request)) {
  sessMap := NewSafeMap()

	f = func(w http.ResponseWriter, r *http.Request) {
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
			dispatchRequest(a, sessMap, &uploadSessionReq{session, w, r, true})
		} else {
			dispatchRequest(a, sessMap, &uploadSessionReq{session, w, r, false})
		}
	}

	return
}

func dispatchRequest(a *AptServer, sessMap *SafeMap, r *uploadSessionReq) {
  if r.create {
    // We've been asked to create an existing session
    // Should never get here
    s := r.SessionId
    cookie := http.Cookie{
      Name:     a.CookieName,
      Value:    s,
      Expires:  time.Now().Add(a.TTL),
      HttpOnly: false,
      Path:     "/package/upload",
    }
    http.SetCookie(r.W, &cookie)

    dir := a.TmpDir + "/" + s
    os.Mkdir(dir, os.FileMode(0755))
    go pathHandle(sessMap, dir, a.TTL)

    sessMap.Set(r.SessionId, r)
  } else {
    c := sessMap.Get(r.SessionId)
    if c != nil {
      //c(r.W, r.R)
      r.W.Write([]byte("Got a hit"))
    } else {
      log.Println("request for unknown session")
      http.NotFound(r.W, r.R)
    }
  }
}

func pathHandle(sessMap *SafeMap, dir string, timeout time.Duration) {
	time.Sleep(timeout)
	os.Remove(dir)
}
