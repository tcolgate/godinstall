package main

//"crypto/md5"
//"github.com/stapelberg/godebiancontrol"

import (
	"flag"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

func rootHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Nothing to see here"))
}

func main() {
	listenAddress := flag.String("listen", ":3000", "ip:port to listen on")
	validate := flag.Bool("validate", true, "Validate signatures on changes and debs")
	ttl := flag.String("ttl", "60s", "Session life time")
	maxGets := flag.Int("max-gets", 4, "Maximum concurrent GETs")
	maxPuts := flag.Int("max-puts", 4, "Maximum concurrent POST/PUTs")
	repoDir := flag.String("repo-dir", "/tmp/myrepo", "Location of repository root")
	tmpDir := flag.String("tmp-dir", "/tmp/up", "Location for temporary storage")
	cookieName := flag.String("cookie-name", "godinstall-sess", "Name for the sessio ookie")
	aftpPath := flag.String("aftp-bin-path", "/usr/bin/apt-ftparchive", "Location of apt-ftparchive binary")
	aftpConfig := flag.String("config", "/etc/aptconfig", "Location of apt-ftparchive configuration file")
	releaseConfig := flag.String("rel-config", "/etc/aptconfig", "Location of apt-ftparchive releases file")
	preAftpHook := flag.String("pre-aftp-hook", "", "Script to run before apt-ftparchive")
	postAftpHook := flag.String("post-aftp-hook", "", "Script to run after apt-ftparchive")

	flag.Parse()

	expire, _ := time.ParseDuration(*ttl)

	server := &AptServer{
		MaxGets:         *maxGets,
		MaxPuts:         *maxPuts,
		RepoDir:         *repoDir,
		TmpDir:          *tmpDir,
		CookieName:      *cookieName,
		TTL:             expire,
		ValidateChanges: *validate,
		ValidateDebs:    *validate,
		AftpPath:        *aftpPath,
		AftpConfig:      *aftpConfig,
		ReleaseConfig:   *releaseConfig,
		PreAftpHook:     *preAftpHook,
		PostAftpHook:    *postAftpHook,
	}

	server.InitAptServer()

	r := mux.NewRouter()
	r.HandleFunc("/", rootHandler).Methods("GET")

	server.Register(r)

	http.Handle("/", r)
	http.ListenAndServe(*listenAddress, nil)
}
