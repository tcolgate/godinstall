package main

//"crypto/md5"
//"github.com/stapelberg/godebiancontrol"

import (
	"flag"
	"log"
	"net/http"
	"os"
	"regexp"
	"time"

	"code.google.com/p/go.crypto/openpgp"

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
	repoBase := flag.String("repo-base", "/tmp/myrepo", "Location of repository root")
	poolBase := flag.String("pool-base", "/tmp/myrepo/pool", "Location of the pool base")
	tmpDir := flag.String("tmp-dir", "/tmp/up", "Location for temporary storage")
	cookieName := flag.String("cookie-name", "godinstall-sess", "Name for the sessio ookie")
	aftpPath := flag.String("aftp-bin-path", "/usr/bin/apt-ftparchive", "Location of apt-ftparchive binary")
	aftpConfig := flag.String("config", "/etc/aptconfig", "Location of apt-ftparchive configuration file")
	releaseConfig := flag.String("rel-config", "/etc/aptconfig", "Location of apt-ftparchive releases file")
	postUploadHook := flag.String("post-upload-hook", "", "Script to run after for each uploaded file")
	preAftpHook := flag.String("pre-aftp-hook", "", "Script to run before apt-ftparchive")
	postAftpHook := flag.String("post-aftp-hook", "", "Script to run after apt-ftparchive")
	poolPattern := flag.String("pool-pattern", "[a-z]|lib[a-z]", "A pattern to match package prefixes to split into directories in the pool")
	pubringFile := flag.String("gpg-pubring", "", "Public keyring file")
	privringFile := flag.String("gpg-privring", "", "Private keyring file")
	signerIdStr := flag.String("signer-id", "", "Key ID to use for signing releases")

	flag.Parse()

	expire, err := time.ParseDuration(*ttl)
	if err != nil {
		log.Println(err.Error())
		return
	}

	poolRegexp, err := regexp.CompilePOSIX("^(" + *poolPattern + ")")

	if err != nil {
		log.Println(err.Error())
		return
	}

	var pubRing openpgp.EntityList
	if *pubringFile != "" {
		pubringReader, err := os.Open(*pubringFile)
		if err != nil {
			log.Println(err.Error())
			return
		}

		pubRing, err = openpgp.ReadKeyRing(pubringReader)
		if err != nil {
			log.Println(err.Error())
			return
		}
	}

	var privRing openpgp.EntityList
	if *privringFile != "" {
		privringReader, err := os.Open(*privringFile)
		if err != nil {
			log.Println(err.Error())
			return
		}

		privRing, err = openpgp.ReadKeyRing(privringReader)
		if err != nil {
			log.Println(err.Error())
			return
		}
	}

	if *validate {
		if privRing == nil || pubRing == nil {
			log.Println("Validation requested, but keyrings not loaded")
			return
		}
	}

	signerId := getKeyByEmail(privRing, *signerIdStr)
	if signerId == nil {
		log.Println("Can't find signer id in keyring")
		return
	}

	err = signerId.PrivateKey.Decrypt([]byte(""))
	if err != nil {
		log.Println("Can't decrypt private key, " + err.Error())
		return
	}

	server := &AptServer{
		MaxGets:         *maxGets,
		MaxPuts:         *maxPuts,
		RepoBase:        *repoBase,
		PoolBase:        *poolBase,
		TmpDir:          *tmpDir,
		CookieName:      *cookieName,
		TTL:             expire,
		ValidateChanges: *validate,
		ValidateDebs:    *validate,
		AftpPath:        *aftpPath,
		AftpConfig:      *aftpConfig,
		ReleaseConfig:   *releaseConfig,
		PostUploadHook:  *postUploadHook,
		PreAftpHook:     *preAftpHook,
		PostAftpHook:    *postAftpHook,
		PoolPattern:     poolRegexp,
		PubRing:         pubRing,
		PrivRing:        privRing,
		SignerId:        signerId,
	}

	server.InitAptServer()

	r := mux.NewRouter()
	r.HandleFunc("/", rootHandler).Methods("GET")

	server.Register(r)

	http.Handle("/", r)
	http.ListenAndServe(*listenAddress, nil)
}
