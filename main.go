package main

//"crypto/md5"
//"github.com/stapelberg/godebiancontrol"

import (
	"code.google.com/p/go-uuid/uuid"
	"github.com/gorilla/mux"
	"net/http"
	"os"
	"time"
)

func main() {
	maxGets := 4
	maxPuts := 4
	repoDir := "/tmp/myrepo"
	tmpDir := "/tmp"
	cookieName := "godinstall-sess"
	expire, _ := time.ParseDuration("15s")

	getLocks := make(chan int, maxGets)
	for i := 0; i < maxGets; i++ {
		getLocks <- 1
	}

	putLocks := make(chan int, maxPuts)
	for i := 0; i < maxPuts; i++ {
		putLocks <- 1
	}

	aptLock := make(chan int, 1)
	aptLock <- 1

	r := mux.NewRouter()
	r.HandleFunc("/", rootHandler).Methods("GET")
	r.HandleFunc("/repo/{rest:.*}", makeRepoHandler(getLocks, repoDir)).Methods("GET")

	handler := makeUploadHandler(tmpDir, cookieName, expire)
	r.HandleFunc("/package/upload", handler).Methods("POST", "PUT")
	r.HandleFunc("/package/upload/{session}", handler).Methods("POST", "PUT")
	http.Handle("/", r)
	http.ListenAndServe(":3000", nil)
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	//params := mux.Vars(r)
	w.Write([]byte("Nothing to see here"))
}

/*
 * This handler servers the repo itself, we use a channel to
 * rate limit the requests, and to let us lock out all requests
 * when trying to update the archive
 */
func makeRepoHandler(lockChan chan int, dir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lock := <-lockChan
		defer func() { lockChan <- lock }()

		file := mux.Vars(r)["rest"]
		realFile := dir + "/" + file

		http.ServeFile(w, r, realFile)
	}
}


type uploadReq struct {
  S string
  W http.ResponseWriter
  R *http.Request
}

/*
 *
 */
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
  for{
    select{
    case d := <- done:
      _,ok := sessMap[d]
      if ok {
        delete(sessMap,d)
      } else {
      }
    case r := <-reqs:
      c,ok := sessMap[r.S]
      if ok {
        c <- r
      } else {
      }
    }
  }
}

/*
 */
func makeUploadHandler(
	tmpDir string,
	cookieName string,
	expire time.Duration) func(http.ResponseWriter, *http.Request) {

  dispatch := make(chan *uploadReq)
  complete := make(chan string)

  go dispatcher(dispatch, complete)

	return func(w http.ResponseWriter, r *http.Request) {
		// Did we get a session
		session, found := mux.Vars(r)["session"]

		//maybe in a cookie?
		if !found {
			cookie, err := r.Cookie(cookieName)
			if err == nil {
				session = cookie.Value
			}
		}

		if session == "" {
			session := uuid.New()
			cookie := http.Cookie{
				Name:     cookieName,
				Value:    session,
				Expires:  time.Now().Add(expire),
				HttpOnly: false,
				Path:     "/package/upload"}
			http.SetCookie(w, &cookie)
			w.Write([]byte(uuid.New()))

			dir := tmpDir + "/" + session
			os.Mkdir(dir, os.FileMode(0755))

			go pathHandle(dir, expire)

		} else {
			w.Write([]byte("Hello3 " + session))
      dispatch <- &uploadReq{session, w, r}
		}
	}
}
