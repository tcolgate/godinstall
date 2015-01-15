package main

import (
	"expvar"
	"log"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"regexp"
	"syscall"

	"github.com/codegangsta/cli"
	"github.com/gorilla/mux"
)

// CmdServe is the implementation of the godinstall "serve" command
func CmdServe(c *cli.Context) {
	listenAddress := c.String("listen")

	logFile := c.String("log-file")
	ttl := c.Duration("ttl")
	maxReqs := c.Int("max-requests")
	repoBase := c.String("repo-base")
	cookieName := c.String("cookie-name")
	uploadHook := c.String("upload-hook")
	preGenHook := c.String("pre-gen-hook")
	postGenHook := c.String("post-gen-hook")
	poolPattern := c.String("default-pool-pattern")
	verifyChanges := c.Bool("default-verify-changes")
	verifyChangesSufficient := c.Bool("default-verify-changes-sufficient")
	acceptLoneDebs := c.Bool("default-accept-lone-debs")
	verifyDebs := c.Bool("default-verify-debs")
	pruneRulesStr := c.String("default-prune")
	autoTrim := c.Bool("default-auto-trim")
	trimLen := c.Int("default-auto-trim-length")

	setupLog(logFile)

	if repoBase == "" {
		log.Fatalln("You must pass --repo-base")
	}

	storeDir := repoBase + "/store"
	tmpDir := repoBase + "/tmp"
	publicDir := repoBase + "/archive"

	dirMustExist(publicDir)
	dirMustExist(storeDir)
	dirMustExist(tmpDir)

	if _, err := ParsePruneRules(pruneRulesStr); err != nil {
		log.Fatalln(err)
	}

	if _, err := regexp.CompilePOSIX("^(" + poolPattern + ")"); err != nil {
		log.Fatalln(err)
	}

	state.Archive = NewAptBlobArchive(
		&storeDir,
		&tmpDir,
		&publicDir,
		ReleaseConfig{
			VerifyChanges:           verifyChanges,
			VerifyChangesSufficient: verifyChangesSufficient,
			VerifyDebs:              verifyDebs,
			AcceptLoneDebs:          acceptLoneDebs,
			PruneRules:              pruneRulesStr,
			AutoTrim:                autoTrim,
			AutoTrimLength:          trimLen,
			PoolPattern:             poolPattern,
		},
	)

	cfg.CookieName = cookieName
	cfg.PreGenHook = NewScriptHook(&preGenHook)
	cfg.PostGenHook = NewScriptHook(&postGenHook)

	state.Lock = NewGovernor(maxReqs)
	state.getCount = expvar.NewInt("GetRequests")

	state.SessionManager = NewUploadSessionManager(
		ttl,
		&tmpDir,
		state.Archive,
		NewScriptHook(&uploadHook),
	)

	r := mux.NewRouter()

	r.HandleFunc("/debug/pprof/", pprof.Index)
	r.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	r.HandleFunc("/debug/pprof/profile", pprof.Profile)
	r.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	r.Handle("/debug/vars", http.DefaultServeMux)

	r.PathPrefix("/repo/").Handler(appHandler(makeHTTPDownloadHandler()))
	r.PathPrefix("/upload").Handler(appHandler(httpUploadHandler))

	r.Handle("/dists", appHandler(httpDistsHandler))
	r.Handle("/dists/{name}", appHandler(httpDistsHandler))
	r.Handle("/dists/{name}/config", appHandler(httpConfigHandler))
	r.Handle("/dists/{name}/config/signingkey", appHandler(httpConfigSigningKeyHandler))
	r.Handle("/dists/{name}/config/publickeys", appHandler(httpConfigPublicKeysHandler))
	r.Handle("/dists/{name}/config/publickeys/{id}", appHandler(httpConfigPublicKeysHandler))
	r.Handle("/dists/{name}/log", appHandler(httpLogHandler))
	r.Handle("/dists/{name}/upload", appHandler(httpUploadHandler))
	r.Handle("/dists/{name}/upload/{session}", appHandler(httpUploadHandler))

	http.ListenAndServe(listenAddress, r)
}

// setupLog manages the provided log file so we can do log rotation in
// a hippee stylee
func setupLog(logFile string) {
	logready := make(chan struct{})
	sighup := make(chan os.Signal, 1)

	go func() {
		initial := true
		logWriter := os.Stderr
		for {
			if logFile != "-" {
				newLog, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0660)
				if err != nil {
					log.SetOutput(os.Stderr)
					logFile = "-"
					log.Printf("Error opening logfile, %v", err)
				} else {
					log.SetOutput(newLog)
					if logWriter != os.Stderr {
						logWriter.Close()
					}
				}
			}
			if initial {
				initial = false
				logready <- struct{}{}
			}
			<-sighup
		}
	}()
	signal.Notify(sighup, syscall.SIGHUP)

	<-logready
}

// dirMustExist creates the directory if it does not already exist
// fatal error if it can't
func dirMustExist(dir string) {
	if _, patherr := os.Stat(dir); os.IsNotExist(patherr) {
		if err := os.Mkdir(dir, 0777); err != nil {
			log.Fatal(err)
		}
	}
}
