package main

import (
	"fmt"
	"log"
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
	return
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
	default:
		http.Error(w,
			http.StatusText(http.StatusMethodNotAllowed),
			http.StatusMethodNotAllowed)
	}
	return
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
			fmt.Sprintf("\"Error retrieving signing key\"", name, err.Error()),
			http.StatusNotFound)
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
	w.Write([]byte("\"" + key.KeyIdString() + "\""))
	return

}

func doHttpConfigSigningKeyPutHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	_, err := state.Archive.GetDist(name)
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
	kr, err := openpgp.ReadArmoredKeyRing(rdr)
	if err != nil {
		http.Error(w,
			"failed to parse data as  key, "+err.Error(),
			http.StatusInternalServerError)
		return
	}

	log.Println(kr[0])
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
	return
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

	return
}
