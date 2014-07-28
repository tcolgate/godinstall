package main

// Package GoDInstall implements a web service for serving, and manipulating
// debian Apt repositories. The original motivation was to provide a synchronous
// interface for package upload. A package is available for download from the
// repository at the point when the server confirms the package has been
// uploaded.
//   It is primarily aimed at use in continuous delivery processes.

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

// HTTP handler for the server /
func rootHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Nothing to see here"))
}

// Looks an email address up in a pgp keyring
func getKeyByEmail(keyring openpgp.EntityList, email string) *openpgp.Entity {
	for _, entity := range keyring {
		for _, ident := range entity.Identities {
			if ident.UserId.Email == email {
				return entity
			}
		}
	}

	return nil
}

func main() {
	// Setup CLI flags
	listenAddress := flag.String("listen", ":3000", "ip:port to listen on")
	acceptLoneDebs := flag.Bool("accpetLoneDebs", true, "Accept individual debs for upload")
	validateChanges := flag.Bool("validateChanges", true, "Validate signatures on changes files")
	validateChangesSufficient := flag.Bool("validateChangesSufficient", true, "If we are given a signed chnages file, we wont validate individual debs")
	validateDebs := flag.Bool("validateDebs", false, "Validate signatures on deb files")
	ttl := flag.String("ttl", "60s", "Session life time")
	maxReqs := flag.Int("max-requests", 4, "Maximum concurrent requests")
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
	signerEmail := flag.String("signer-email", "", "Key Email to use for signing releases")

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

	if *validateChanges || *validateDebs {
		if privRing == nil || pubRing == nil {
			log.Println("Validation requested, but keyrings not loaded")
			return
		}
	}

	signerId := getKeyByEmail(privRing, *signerEmail)
	if signerId == nil {
		log.Println("Can't find signer id in keyring")
		return
	}

	err = signerId.PrivateKey.Decrypt([]byte(""))
	if err != nil {
		log.Println("Can't decrypt private key, " + err.Error())
		return
	}

	updateChan := make(chan UpdateRequest)

	aptRepo := aptRepo{
		repoBase,
		poolBase,
		poolRegexp,
	}

	aptGenerator := NewAptFtpArchiveGenerator(
		&aptRepo,
		aftpPath,
		aftpConfig,
		releaseConfig,
		privRing,
		signerId,
	)

	uploadSessionManager := NewUploadSessionManager(
		expire,
		tmpDir,
		NewScriptHook(postUploadHook),
		*validateChanges,
		*validateChangesSufficient,
		*validateDebs,
		pubRing,
		updateChan,
	)

	server := &AptServer{
		MaxReqs:        *maxReqs,
		CookieName:     *cookieName,
		PreAftpHook:    NewScriptHook(preAftpHook),
		PostAftpHook:   NewScriptHook(postAftpHook),
		AcceptLoneDebs: acceptLoneDebs,

		Repo:           &aptRepo,
		AptGenerator:   aptGenerator,
		SessionManager: uploadSessionManager,
		UpdateChannel:  updateChan,
	}

	server.InitAptServer()

	r := mux.NewRouter()
	r.HandleFunc("/", rootHandler).Methods("GET")

	server.Register(r)

	http.Handle("/", r)
	http.ListenAndServe(*listenAddress, nil)
}
