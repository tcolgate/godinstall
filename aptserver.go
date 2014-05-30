package main

//"crypto/md5"
//"github.com/stapelberg/godebiancontrol"

import (
	"code.google.com/p/go-uuid/uuid"
	"github.com/gorilla/mux"
	"log"
	"net/http"
	"mime/multipart"
	"os"
	"time"
	"strings"
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

type uploadSession struct {
	SessionId string
  dir string
}

func (s *uploadSession) Close(){
  os.Remove(s.dir)
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
    err := r.R.ParseMultipartForm(64000000)
    if err != nil {
      http.Error(r.W, err.Error(), http.StatusInternalServerError)
      return
    }

    form := r.R.MultipartForm
    log.Println(form)
    files := form.File["debfiles"]
    log.Println(files)
    var changes multipart.File
    for _, f := range files {
       log.Println(f.Filename)
       if (strings.HasSuffix(f.Filename, ".changes")){
         changes,err = f.Open()
         break
       }
    }

    if changes == nil {
      http.Error(r.W, "No debian changes file in request", http.StatusInternalServerError)
      return
    }

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

    sessMap.Set(r.SessionId, &uploadSession{dir: dir})
    go pathHandle(sessMap, r.SessionId, a.TTL)
  } else {
    c := sessMap.Get(r.SessionId)
    if c != nil {
      r.W.Write([]byte("Got a hit"))
    } else {
      log.Println("request for unknown session")
      http.NotFound(r.W, r.R)
    }
  }
}

func pathHandle(sessMap *SafeMap, s string, timeout time.Duration) {
	time.Sleep(timeout)
  c := sessMap.Get(s)
  if c != nil {
    switch sess := c.(type){
      case *uploadSession:
        log.Println("Close session")
    	  sess.Close()
      default:
        log.Println("Shouldn't get here")
    }
  }else{
    log.Println("Didn't find session")
  }
}
