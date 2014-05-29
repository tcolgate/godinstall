package main

//"crypto/md5"
//"github.com/stapelberg/godebiancontrol"

import (
	"github.com/gorilla/mux"
	"net/http"
	"time"
	"flag"
)

func rootHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Nothing to see here"))
}

func main() {
  var listenAddress = flag.String("listen", ":3000", "ip:port to listen on")

  flag.Parse()

	expire, _ := time.ParseDuration("15s")

  server := &AptServer{
	  MaxGets: 4,
	  MaxPuts: 4,
	  RepoDir: "/tmp/myrepo",
	  TmpDir: "/tmp",
	  CookieName: "godinstall-sess",
    TTL: expire,
  }

  server.InitAptServer()

	r := mux.NewRouter()
	r.HandleFunc("/", rootHandler).Methods("GET")

  server.Register(r)

	http.Handle("/", r)
	http.ListenAndServe(*listenAddress, nil)
}
