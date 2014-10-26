package main

// Package GoDInstall implements a web service for serving, and manipulating
// debian Apt repositories. The original motivation was to provide a synchronous
// interface for package upload. A package is available for download from the
// repository at the point when the server confirms the package has been
// uploaded.
//   It is primarily aimed at use in continuous delivery processes.

import (
	_ "expvar"
	"flag"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"regexp"
	"time"

	"code.google.com/p/go.crypto/openpgp"
	"github.com/codegangsta/cli"
)

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
	app := cli.NewApp()
	app.Name = "godinstall"
	app.Usage = "dynamic apt repository server"

	app.Commands = []cli.Command{
		cli.Command{
			Name: "serve",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "l, listen",
					Value: ":3000",
					Usage: "The listen address",
				},
				cli.StringFlag{
					Name:  "t, ttl",
					Value: "60s",
					Usage: "Upload session will be terminated after the TTL",
				},
				cli.IntFlag{
					Name:  "max-requests",
					Value: 4,
					Usage: "Maximum concurrent requests",
				},
				cli.StringFlag{
					Name:  "repo-base",
					Value: "",
					Usage: "Location of repository root",
				},
				cli.StringFlag{
					Name:  "cookie-name",
					Value: "godinstall-sess",
					Usage: "Name for the sessio cookie",
				},
				cli.StringFlag{
					Name:  "upload-hook",
					Value: "",
					Usage: "Script to run after for each uploaded file",
				},
				cli.StringFlag{
					Name:  "pre-gen-hook",
					Value: "",
					Usage: "Script to run before archive regeneration",
				},
				cli.StringFlag{
					Name:  "post-gen-hook",
					Value: "",
					Usage: "Script to run after archive regeneration",
				},
				cli.StringFlag{
					Name:  "pool-pattern",
					Value: "[a-z]|lib[a-z]",
					Usage: "A pattern to match package prefixes to split into directories in the pool",
				},
				cli.BoolTFlag{
					Name:  "validate-changes",
					Usage: "Validate signatures on changes files",
				},
				cli.BoolTFlag{
					Name:  "validate-changes-sufficient",
					Usage: "If we are given a signed chnages file, we wont validate individual debs",
				},
				cli.BoolTFlag{
					Name:  "accept-lone-debs",
					Usage: "Accept individual debs for upload",
				},
				cli.BoolTFlag{
					Name:  "validate-debs",
					Usage: "Validate signatures on deb files",
				},
				cli.StringFlag{
					Name:  "gpg-pubring",
					Value: "",
					Usage: "Public keyring file",
				},
				cli.StringFlag{
					Name:  "gpg-privring",
					Value: "",
					Usage: "Private keyring file",
				},
				cli.StringFlag{
					Name:  "signer-email",
					Value: "",
					Usage: "Key Email to use for signing releases",
				},
				cli.StringFlag{
					Name:  "prune",
					Value: ".*_*-*",
					Usage: "Rules for package pruning",
				},
			},
			Usage:  "run a repository server",
			Action: CmdServe,
		},
	}

	app.Run(os.Args)
}

// CmdServe is the implementation of the godinstall "serve" command
func CmdServe(c *cli.Context) {
	// Setup CLI flags
	listenAddress := c.String("listen")
	ttl := c.String("ttl")
	maxReqs := c.Int("max-requests")
	repoBase := c.String("repo-base")
	cookieName := c.String("cookie-name")
	uploadHook := c.String("upload-hook")
	preGenHook := c.String("pre-gen-hook")
	postGenHook := c.String("post-gen-hook")
	poolPattern := c.String("pool-pattern")
	validateChanges := c.Bool("validate-changes")
	validateChangesSufficient := c.Bool("validate-changes-sufficient")
	acceptLoneDebs := c.Bool("accept-lone-debs")
	validateDebs := c.Bool("validate-debs")
	pubringFile := c.String("gpg-pubring")
	privringFile := c.String("gpg-privring")
	signerEmail := c.String("signer-email")
	pruneRulesStr := c.String("prune")

	flag.Parse()

	if repoBase == "" {
		log.Println("You must pass --repo-base")
		return
	}

	expire, err := time.ParseDuration(ttl)
	if err != nil {
		log.Println(err.Error())
		return
	}

	poolRegexp, err := regexp.CompilePOSIX("^(" + poolPattern + ")")

	if err != nil {
		log.Println(err.Error())
		return
	}

	var pubRing openpgp.EntityList
	if pubringFile != "" {
		pubringReader, err := os.Open(pubringFile)
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
	if privringFile != "" {
		privringReader, err := os.Open(privringFile)
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

	if validateChanges || validateDebs {
		if privRing == nil || pubRing == nil {
			log.Println("Validation requested, but keyrings not loaded")
			return
		}
	}

	var signerID *openpgp.Entity
	if signerEmail != "" {
		signerID = getKeyByEmail(privRing, signerEmail)
		if signerID == nil {
			log.Println("Can't find signer id in keyring")
			return
		}

		err = signerID.PrivateKey.Decrypt([]byte(""))
		if err != nil {
			log.Println("Can't decrypt private key, " + err.Error())
			return
		}
	}

	updateChan := make(chan UpdateRequest)

	base := repoBase + "/archive"
	storeDir := repoBase + "/store"
	tmpDir := repoBase + "/tmp"

	_, patherr := os.Stat(base)
	if os.IsNotExist(patherr) {
		err = os.Mkdir(base, 0777)
		if err != nil {
			log.Println(err.Error())
			return
		}
	}
	_, patherr = os.Stat(storeDir)
	if os.IsNotExist(patherr) {
		err = os.Mkdir(storeDir, 0777)
		if err != nil {
			log.Println(err.Error())
			return
		}
	}

	_, patherr = os.Stat(tmpDir)
	if os.IsNotExist(patherr) {
		err = os.Mkdir(tmpDir, 0777)
		if err != nil {
			log.Println(err.Error())
			return
		}
	}

	aptRepo := aptRepo{
		&base,
		poolRegexp,
	}

	repoStore := NewRepoBlobStore(storeDir, tmpDir)

	pruneRules, err := ParsePruneRules(pruneRulesStr)
	if err != nil {
		log.Println(err.Error())
		return
	}

	aptGenerator := NewAptBlobArchiveGenerator(
		&aptRepo,
		privRing,
		signerID,
		repoStore,
		pruneRules,
	)

	uploadSessionManager := NewUploadSessionManager(
		expire,
		&tmpDir,
		repoStore,
		NewScriptHook(&uploadHook),
		validateChanges,
		validateChangesSufficient,
		validateDebs,
		pubRing,
		updateChan,
	)

	server := &AptServer{
		MaxReqs:        maxReqs,
		CookieName:     cookieName,
		PreGenHook:     NewScriptHook(&preGenHook),
		PostGenHook:    NewScriptHook(&postGenHook),
		AcceptLoneDebs: acceptLoneDebs,

		Repo:           &aptRepo,
		AptGenerator:   aptGenerator,
		SessionManager: uploadSessionManager,
		UpdateChannel:  updateChan,
		PubRing:        pubRing,
	}

	server.InitAptServer()
	server.Register(http.DefaultServeMux)
	http.ListenAndServe(listenAddress, http.DefaultServeMux)
}
