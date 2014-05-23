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

func main() {

	maxGets := 4
	maxPuts := 4
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

	aptLock := make(chan int,1)
	aptLock <- 1

	r := mux.NewRouter()
	r.HandleFunc("/", rootHandler).Methods("GET")
	r.HandleFunc("/repo", repoHandler).Methods("GET")

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

func repoHandler(w http.ResponseWriter, r *http.Request) {
	//params := mux.Vars(r)
	w.Write([]byte("Hello2"))
}

func pathHandle(dir string) {
	log.Println("delay: " + dir)
	time.Sleep(5 * time.Second)
	log.Println("deleting: " + dir)
	os.Remove(dir)
}

func makeUploadHandler(
	tmpDir string,
	cookieName string,
	expire time.Duration) func(http.ResponseWriter, *http.Request) {

	return func(w http.ResponseWriter, r *http.Request) {

		// These should be configurable and closure'd in

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

			go pathHandle(dir)

		} else {
			w.Write([]byte("Hello3 " + session))
		}

	}
}
