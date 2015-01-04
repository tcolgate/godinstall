package main

import (
	"fmt"
	"net/http"
	"strings"

	"code.google.com/p/go.crypto/openpgp"
	"github.com/gorilla/mux"
)

// This build a function to enumerate the distributions
func httpConfigHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		handleWithReadLock(doHttpConfigGetHandler, w, r)
	default:
		http.Error(w,
			http.StatusText(http.StatusMethodNotAllowed),
			http.StatusMethodNotAllowed)
	}
}

// This build a function to manage the config of a distribution
func doHttpConfigGetHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	rel, err := state.Archive.GetDist(name)
	if err != nil {
		http.Error(w,
			fmt.Sprintf("distribution %v not found, %s", name, err.Error()),
			http.StatusNotFound)
		return
	}

	SendOKResponse(w, rel.Config())
}

// This build a function to enumerate the distributions
func httpConfigSigningKeyHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		handleWithReadLock(doHttpConfigSigningKeyGetHandler, w, r)
	case "PUT", "POST":
		handleWithWriteLock(doHttpConfigSigningKeyPutHandler, w, r)
	case "DELETE":
		handleWithWriteLock(doHttpConfigSigningKeyDeleteHandler, w, r)
	default:
		http.Error(w,
			http.StatusText(http.StatusMethodNotAllowed),
			http.StatusMethodNotAllowed)
	}
}

func doHttpConfigSigningKeyGetHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	rel, err := state.Archive.GetDist(name)
	if err != nil {
		http.Error(w,
			fmt.Sprintf("distribution %v not found, %s", name, err.Error()),
			http.StatusNotFound)
		return
	}

	id, err := rel.SignerKey()
	if err != nil {
		http.Error(w,
			fmt.Sprintf("\"Error retrieving signing key, %s\"", err.Error()),
			http.StatusInternalServerError)
		return
	}

	if id == nil {
		http.Error(w,
			fmt.Sprintf("\"No signing key set\""),
			http.StatusNotFound)
		return
	}

	key := id.PrimaryKey
	if key == nil {
		http.Error(w,
			fmt.Sprintf("\"No signing key set\""),
			http.StatusNotFound)
		return
	}
	SendOKResponse(w, key.KeyIdShortString())
}

func doHttpConfigSigningKeyPutHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	rel, err := state.Archive.GetDist(name)
	if err != nil {
		http.Error(w,
			fmt.Sprintf("distribution %v not found, %s", name, err.Error()),
			http.StatusNotFound)
		return
	}

	id, err := state.Archive.CopyToStore(r.Body)
	if err != nil {
		http.Error(w,
			"failed to copy key to store, "+err.Error(),
			http.StatusInternalServerError)
		return
	}
	rdr, err := state.Archive.Open(id)
	_, err = openpgp.ReadArmoredKeyRing(rdr)
	if err != nil {
		http.Error(w,
			"failed to parse data as  key, "+err.Error(),
			http.StatusInternalServerError)
		return
	}

	cfg := rel.Config()

	cfg.SigningKeyID = id

	newcfgid, err := state.Archive.AddReleaseConfig(*cfg)
	if err != nil {
		http.Error(w,
			"failed to parse add new release config, "+err.Error(),
			http.StatusInternalServerError)
		return
	}

	rel.ConfigID = newcfgid
	newrelid, err := state.Archive.AddRelease(rel)
	if err != nil {
		http.Error(w,
			"failed to parse add new release, "+err.Error(),
			http.StatusInternalServerError)
		return
	}

	err = state.Archive.SetDist(name, newrelid)
	if err != nil {
		http.Error(w,
			"failed to parse add update release rag, "+err.Error(),
			http.StatusInternalServerError)
		return
	}

	doHttpConfigSigningKeyGetHandler(w, r)
}

func doHttpConfigSigningKeyDeleteHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	rel, err := state.Archive.GetDist(name)
	if err != nil {
		http.Error(w,
			fmt.Sprintf("distribution %v not found, %s", name, err.Error()),
			http.StatusNotFound)
		return
	}

	cfg := rel.Config()
	cfg.SigningKeyID = nil

	newcfgid, err := state.Archive.AddReleaseConfig(*cfg)
	if err != nil {
		http.Error(w,
			"failed to parse add new release config, "+err.Error(),
			http.StatusInternalServerError)
		return
	}

	rel.ConfigID = newcfgid
	newrelid, err := state.Archive.AddRelease(rel)
	if err != nil {
		http.Error(w,
			"failed to parse add new release, "+err.Error(),
			http.StatusInternalServerError)
		return
	}

	err = state.Archive.SetDist(name, newrelid)
	if err != nil {
		http.Error(w,
			"failed to parse add update release rag, "+err.Error(),
			http.StatusInternalServerError)
		return
	}

	doHttpConfigSigningKeyGetHandler(w, r)
}

// This build a function to enumerate the distributions
func httpConfigPublicKeysHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		handleWithReadLock(doHttpConfigPublicKeysGetHandler, w, r)
		//	case "PUT", "POST":
		//		handleWithWriteLock(doHttpConfigSigningKeyPutHandler, w, r)
	default:
		http.Error(w,
			http.StatusText(http.StatusMethodNotAllowed),
			http.StatusMethodNotAllowed)
	}
}

// For managing public keys in a config
func doHttpConfigPublicKeysGetHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]
	reqid := vars["id"]

	rel, err := state.Archive.GetDist(name)
	if err != nil {
		http.Error(w,
			fmt.Sprintf("distribution %v not found, %s", name, err.Error()),
			http.StatusNotFound)
		return
	}

	ids, err := rel.PubRing()
	if err != nil {
		http.Error(w,
			fmt.Sprintf("\"Error retrieving signing keys\"", name, err.Error()),
			http.StatusNotFound)
		return
	}
	if reqid == "" {
		var keyids []string
		for _, id := range ids {
			key := id.PrimaryKey
			if key != nil {
				keyids = append(keyids, "\""+key.KeyIdString()+"\"")
			}
		}
		w.Write([]byte("[" + strings.Join(keyids, ",") + "]"))
	} else {
		found := false
		output := ""
		for _, id := range ids {
			key := id.PrimaryKey
			if key.KeyIdString() == reqid {
				output = reqid
			}
		}
		if found {
			w.Write([]byte(output))
		} else {
			http.NotFound(w, r)
		}
	}
}
