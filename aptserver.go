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

func makeUploadHandler(a *AptServer) (f func(w http.ResponseWriter, r *http.Request)) {
	dispatch := make(chan *uploadReq)
	complete := make(chan string)

	go dispatcher(dispatch, complete)

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
			cookie := http.Cookie{
				Name:     a.CookieName,
				Value:    session,
				Expires:  time.Now().Add(a.TTL),
				HttpOnly: false,
				Path:     "/package/upload"}
			http.SetCookie(w, &cookie)
			w.Write([]byte(uuid.New()))

			dir := a.TmpDir + "/" + session
			os.Mkdir(dir, os.FileMode(0755))

			go pathHandle(dir, a.TTL)

		} else {
			w.Write([]byte("Hello3 " + session))
			dispatch <- &uploadReq{session, w, r}
		}
	}

	return
}

type uploadReq struct {
	S string
	W http.ResponseWriter
	R *http.Request
}

func pathHandle(dir string, timeout time.Duration) {
	expired := make(chan bool)

	go func() {
		time.Sleep(timeout)
		expired <- true
	}()

	defer os.Remove(dir)

	for {
		select {
		case <-expired:
			return
		}
	}
}

func dispatcher(reqs chan *uploadReq, done chan string) {
	var sessMap map[string]chan *uploadReq
	for {
		select {
		case d := <-done:
			_, ok := sessMap[d]
			if ok {
				delete(sessMap, d)
			} else {
			}
		case r := <-reqs:
			c, ok := sessMap[r.S]
			if ok {
				c <- r
			} else {
			}
		}
	}
}
