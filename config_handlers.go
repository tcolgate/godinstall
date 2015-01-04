package main

import (
	"fmt"
	"log"
	"net/http"
	"sort"

	"code.google.com/p/go.crypto/openpgp"
	"github.com/gorilla/mux"
)

// Lots of DRY fail here, can probably clear this by wrapping requests
// that result in a repo update

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
		SendDefaultResponse(w, http.StatusNotFound)
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
		SendDefaultResponse(w, http.StatusNotFound)
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
	if !AuthorisedAdmin(w, r) {
		return
	}

	vars := mux.Vars(r)
	name := vars["name"]

	p, err := state.Archive.GetDist(name)
	if err != nil {
		SendDefaultResponse(w, http.StatusNotFound)
		return
	}

	rel := p.NewChildRelease()

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
	if cfg.SigningKeyID.String() == id.String() {
		doHttpConfigSigningKeyGetHandler(w, r)
		return
	}

	cfg.SigningKeyID = id

	newcfgid, err := state.Archive.AddReleaseConfig(*cfg)
	if err != nil {
		http.Error(w,
			"failed to add new release config, "+err.Error(),
			http.StatusInternalServerError)
		return
	}

	rel.ConfigID = newcfgid
	if !rel.updateReleaseSigFiles() {
		doHttpConfigSigningKeyGetHandler(w, r)
		return
	}

	newrelid, err := state.Archive.AddRelease(rel)
	if err != nil {
		http.Error(w,
			"failed to update key, "+err.Error(),
			http.StatusInternalServerError)
		return
	}

	err = state.Archive.SetDist(name, newrelid)
	if err != nil {
		http.Error(w,
			"failed to update key, "+err.Error(),
			http.StatusInternalServerError)
		return
	}

	err = state.Archive.ReifyRelease(newrelid)
	if err != nil {
		http.Error(w,
			"failed to update key, "+err.Error(),
			http.StatusInternalServerError)
		return
	}

	doHttpConfigSigningKeyGetHandler(w, r)
}

func doHttpConfigSigningKeyDeleteHandler(w http.ResponseWriter, r *http.Request) {
	if !AuthorisedAdmin(w, r) {
		return
	}
	vars := mux.Vars(r)
	name := vars["name"]

	p, err := state.Archive.GetDist(name)
	if err != nil {
		SendDefaultResponse(w, http.StatusNotFound)
		return
	}

	rel := p.NewChildRelease()
	cfg := rel.Config()
	if cfg.SigningKeyID == nil {
		doHttpConfigSigningKeyGetHandler(w, r)
		return
	}

	cfg.SigningKeyID = nil

	newcfgid, err := state.Archive.AddReleaseConfig(*cfg)
	if err != nil {
		http.Error(w,
			"failed to parse add new release config, "+err.Error(),
			http.StatusInternalServerError)
		return
	}

	rel.ConfigID = newcfgid
	if !rel.updateReleaseSigFiles() {
		doHttpConfigSigningKeyGetHandler(w, r)
		return
	}

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

	err = state.Archive.ReifyRelease(newrelid)
	if err != nil {
		http.Error(w,
			"failed to update key, "+err.Error(),
			http.StatusInternalServerError)
		return
	}

	SendOKResponse(w, "DELETED")
}

// This build a function to enumerate the distributions
func httpConfigPublicKeysHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		handleWithReadLock(doHttpConfigPublicKeysGetHandler, w, r)
	case "POST":
		handleWithWriteLock(doHttpConfigPublicKeysPostHandler, w, r)
	case "DELETE":
		handleWithWriteLock(doHttpConfigPublicKeysDeleteHandler, w, r)
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
		SendDefaultResponse(w, http.StatusNotFound)
		return
	}

	ids, err := rel.PubRing()
	if err != nil {
		http.Error(w,
			fmt.Sprintf("\"Error retrieving signing keys\"", name, err.Error()),
			http.StatusInternalServerError)
		return
	}

	if reqid == "" {
		var keyids []string
		for _, id := range ids {
			key := id.PrimaryKey
			if key != nil {
				keyids = append(keyids, key.KeyIdString())
			}
		}
		SendOKResponse(w, keyids)
	} else {
		for _, id := range ids {
			key := id.PrimaryKey
			if key.KeyIdString() == reqid {
				// This is a bit boring, should output more
				SendOKResponse(w, key.KeyIdShortString())
				return
			}
		}
		http.NotFound(w, r)
	}
}

func doHttpConfigPublicKeysPostHandler(w http.ResponseWriter, r *http.Request) {
	if !AuthorisedAdmin(w, r) {
		return
	}
	vars := mux.Vars(r)
	name := vars["name"]

	p, err := state.Archive.GetDist(name)
	if err != nil {
		SendDefaultResponse(w, http.StatusNotFound)
		return
	}

	rel := p.NewChildRelease()

	id, err := state.Archive.CopyToStore(r.Body)
	if err != nil {
		http.Error(w,
			"failed to copy key to store, "+err.Error(),
			http.StatusInternalServerError)
		return
	}
	rdr, err := state.Archive.Open(id)
	defer rdr.Close()
	kr, err := openpgp.ReadArmoredKeyRing(rdr)
	if err != nil {
		http.Error(w,
			"failed to parse data as  key, "+err.Error(),
			http.StatusInternalServerError)
		return
	}

	if len(kr) != 1 {
		http.Error(w,
			"upload 1 key at a time",
			http.StatusInternalServerError)
		return
	}

	key := kr[0]
	known, err := rel.PubRing()
	if err != nil {
		http.Error(w,
			"while reading keyring, "+err.Error(),
			http.StatusInternalServerError)
		return
	}

	for _, k := range known {
		if key.PrimaryKey.KeyIdString() == k.PrimaryKey.KeyIdString() {
			doHttpConfigPublicKeysGetHandler(w, r)
			return
		}
	}

	c := rel.Config()
	c.PublicKeyIDs = append(c.PublicKeyIDs, id)
	sort.Sort(ByID(c.PublicKeyIDs))

	newcfgid, err := state.Archive.AddReleaseConfig(*c)
	if err != nil {
		http.Error(w,
			"failed to add new release config, "+err.Error(),
			http.StatusInternalServerError)
		return
	}

	rel.ConfigID = newcfgid
	newrelid, err := state.Archive.AddRelease(rel)
	if err != nil {
		http.Error(w,
			"failed to update key, "+err.Error(),
			http.StatusInternalServerError)
		return
	}

	err = state.Archive.SetDist(name, newrelid)
	if err != nil {
		http.Error(w,
			"failed to update key, "+err.Error(),
			http.StatusInternalServerError)
		return
	}

	err = state.Archive.ReifyRelease(newrelid)
	if err != nil {
		http.Error(w,
			"failed to update key, "+err.Error(),
			http.StatusInternalServerError)
		return
	}

	doHttpConfigPublicKeysGetHandler(w, r)
}

func doHttpConfigPublicKeysDeleteHandler(w http.ResponseWriter, r *http.Request) {
	if !AuthorisedAdmin(w, r) {
		return
	}
	vars := mux.Vars(r)
	name := vars["name"]
	id := vars["id"]

	p, err := state.Archive.GetDist(name)
	if err != nil {
		SendDefaultResponse(w, http.StatusNotFound)
		return
	}

	rel := p.NewChildRelease()

	c := rel.Config()

	found := false
	finalKeys := []StoreID{}
	for _, k := range c.PublicKeyIDs {
		rdr, err := rel.store.Open(k)
		if err != nil {
			log.Printf("reading key failed, %v", err)
			continue
		}
		kr, err := openpgp.ReadArmoredKeyRing(rdr)
		if err != nil {
			log.Printf("reading keyring from store failed, %v", err)
			continue
		}
		if len(kr) != 1 {
			log.Printf("reading keyring from store failed, len was %v", len(kr))
			continue
		}

		key := kr[0]
		if key.PrimaryKey.KeyIdString() == id {
			found = true
		} else {
			finalKeys = append(finalKeys, k)
		}
	}

	if !found {
		SendDefaultResponse(w, http.StatusNotFound)
		return
	}

	c.PublicKeyIDs = finalKeys
	newcfgid, err := state.Archive.AddReleaseConfig(*c)
	if err != nil {
		http.Error(w,
			"failed to add new release config, "+err.Error(),
			http.StatusInternalServerError)
		return
	}

	rel.ConfigID = newcfgid
	newrelid, err := state.Archive.AddRelease(rel)
	if err != nil {
		http.Error(w,
			"failed to update key, "+err.Error(),
			http.StatusInternalServerError)
		return
	}

	err = state.Archive.SetDist(name, newrelid)
	if err != nil {
		http.Error(w,
			"failed to update key, "+err.Error(),
			http.StatusInternalServerError)
		return
	}

	err = state.Archive.ReifyRelease(newrelid)
	if err != nil {
		http.Error(w,
			"failed to update key, "+err.Error(),
			http.StatusInternalServerError)
		return
	}

	SendOKResponse(w, "DELETED")
}
