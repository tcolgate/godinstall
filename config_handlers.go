package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"

	"code.google.com/p/go.crypto/openpgp"
	"github.com/gorilla/mux"
)

// Lots of DRY fail here, can probably clear this by wrapping requests
// that result in a repo update

// This build a function to enumerate the distributions
func httpConfigHandler(w http.ResponseWriter, r *http.Request) *appError {
	switch r.Method {
	case "GET":
		return handleWithReadLock(doHttpConfigGetHandler, w, r)
	default:
		return sendResponse(w, http.StatusMethodNotAllowed, nil)
	}
}

// This build a function to manage the config of a distribution
func doHttpConfigGetHandler(w http.ResponseWriter, r *http.Request) *appError {
	vars := mux.Vars(r)
	name := vars["name"]

	rel, err := state.Archive.GetDist(name)
	switch {
	case err == nil:
		return sendOKResponse(w, rel.Config())
	case os.IsNotExist(err):
		return sendResponse(w, http.StatusNotFound, nil)
	default:
		return &appError{Error: err}
	}
}

// This build a function to enumerate the distributions
func httpConfigSigningKeyHandler(w http.ResponseWriter, r *http.Request) *appError {
	switch r.Method {
	case "GET":
		return handleWithReadLock(doHttpConfigSigningKeyGetHandler, w, r)
	case "PUT", "POST":
		return handleWithWriteLock(doHttpConfigSigningKeyPutHandler, w, r)
	case "DELETE":
		return handleWithWriteLock(doHttpConfigSigningKeyDeleteHandler, w, r)
	default:
		return sendResponse(w, http.StatusMethodNotAllowed, nil)
	}
}

func doHttpConfigSigningKeyGetHandler(w http.ResponseWriter, r *http.Request) *appError {
	vars := mux.Vars(r)
	name := vars["name"]

	rel, err := state.Archive.GetDist(name)
	if err != nil {
		return sendResponse(w, http.StatusNotFound, nil)
	}

	id, err := rel.SignerKey()
	if err != nil {
		return &appError{
			Error: err,
		}
	}

	if id == nil {
		return sendResponse(w, http.StatusBadRequest, "No signing key set")
	}

	key := id.PrimaryKey
	if key == nil {
		return sendResponse(w, http.StatusBadRequest, "No signing key set")
	}

	return sendOKResponse(w, key.KeyIdShortString())
}

func doHttpConfigSigningKeyPutHandler(w http.ResponseWriter, r *http.Request) *appError {
	if !AuthorisedAdmin(w, r) {
		return sendResponse(w, http.StatusUnauthorized, nil)
	}

	vars := mux.Vars(r)
	name := vars["name"]

	p, err := state.Archive.GetDist(name)
	if err != nil {
		return sendResponse(w, http.StatusNotFound, nil)
	}

	rel := p.NewChildRelease()

	id, err := state.Archive.CopyToStore(r.Body)
	if err != nil {
		return &appError{
			Error: errors.New("Failed to copy key to store, " + err.Error()),
		}
	}
	rdr, err := state.Archive.Open(id)
	_, err = openpgp.ReadArmoredKeyRing(rdr)
	if err != nil {
		return sendResponse(w, http.StatusBadRequest, "failed to parse data as  key, "+err.Error())
	}

	cfg := rel.Config()
	if cfg.SigningKeyID.String() == id.String() {
		return doHttpConfigSigningKeyGetHandler(w, r)
	}

	cfg.SigningKeyID = id

	newcfgid, err := state.Archive.AddReleaseConfig(*cfg)
	if err != nil {
		return &appError{
			Error: errors.New("failed to add new release config, " + err.Error()),
		}
	}

	rel.ConfigID = newcfgid
	if !rel.updateReleaseSigFiles() {
		return doHttpConfigSigningKeyGetHandler(w, r)
	}

	newrelid, err := state.Archive.AddRelease(rel)
	if err != nil {
		return &appError{
			Error: errors.New("failed to update key, " + err.Error()),
		}
	}

	err = state.Archive.SetDist(name, newrelid)
	if err != nil {
		return &appError{
			Error: errors.New("failed to update key, " + err.Error()),
		}
	}

	err = state.Archive.ReifyRelease(newrelid)
	if err != nil {
		return &appError{
			Error: errors.New("failed to update key, " + err.Error()),
		}
	}

	return doHttpConfigSigningKeyGetHandler(w, r)
}

func doHttpConfigSigningKeyDeleteHandler(w http.ResponseWriter, r *http.Request) *appError {
	if !AuthorisedAdmin(w, r) {
		return sendResponse(w, http.StatusUnauthorized, nil)
	}
	vars := mux.Vars(r)
	name := vars["name"]

	p, err := state.Archive.GetDist(name)
	if err != nil {
		return sendResponse(w, http.StatusNotFound, nil)
	}

	rel := p.NewChildRelease()
	cfg := rel.Config()
	if cfg.SigningKeyID == nil {
		return doHttpConfigSigningKeyGetHandler(w, r)
	}

	cfg.SigningKeyID = nil

	newcfgid, err := state.Archive.AddReleaseConfig(*cfg)
	if err != nil {
		return &appError{Error: fmt.Errorf("failed to parse add new release config, %v", err)}
	}

	rel.ConfigID = newcfgid
	if !rel.updateReleaseSigFiles() {
		return doHttpConfigSigningKeyGetHandler(w, r)
	}

	newrelid, err := state.Archive.AddRelease(rel)
	if err != nil {
		return &appError{Error: fmt.Errorf("failed to parse add new release, %v", err)}
	}

	err = state.Archive.SetDist(name, newrelid)
	if err != nil {
		return &appError{Error: fmt.Errorf("failed to parse add update release tag, %v", err)}
	}

	err = state.Archive.ReifyRelease(newrelid)
	if err != nil {
		return &appError{Error: fmt.Errorf("failed to update key, %v", err)}
	}

	return sendOKResponse(w, "DELETED")
}

// This build a function to enumerate the distributions
func httpConfigPublicKeysHandler(w http.ResponseWriter, r *http.Request) *appError {
	switch r.Method {
	case "GET":
		return handleWithReadLock(doHttpConfigPublicKeysGetHandler, w, r)
	case "POST":
		return handleWithWriteLock(doHttpConfigPublicKeysPostHandler, w, r)
	case "DELETE":
		return handleWithWriteLock(doHttpConfigPublicKeysDeleteHandler, w, r)
	default:
		return sendResponse(w, http.StatusMethodNotAllowed, nil)
	}
}

// For managing public keys in a config
func doHttpConfigPublicKeysGetHandler(w http.ResponseWriter, r *http.Request) *appError {
	vars := mux.Vars(r)
	name := vars["name"]
	reqid := vars["id"]

	rel, err := state.Archive.GetDist(name)
	if err != nil {
		return sendResponse(w, http.StatusNotFound, nil)
	}

	ids, err := rel.PubRing()
	if err != nil {
		return &appError{Error: fmt.Errorf("Error retrieving signing keys, %v", err)}
	}

	if reqid == "" {
		var keyids []string
		for _, id := range ids {
			key := id.PrimaryKey
			if key != nil {
				keyids = append(keyids, key.KeyIdString())
			}
		}
		return sendOKResponse(w, keyids)
	} else {
		for _, id := range ids {
			key := id.PrimaryKey
			if key.KeyIdString() == reqid {
				// This is a bit boring, should output more
				return sendOKResponse(w, key.KeyIdShortString())
			}
		}
		return sendResponse(w, http.StatusNotFound, nil)
	}
}

func doHttpConfigPublicKeysPostHandler(w http.ResponseWriter, r *http.Request) *appError {
	if !AuthorisedAdmin(w, r) {
		return sendResponse(w, http.StatusUnauthorized, nil)
	}
	vars := mux.Vars(r)
	name := vars["name"]

	p, err := state.Archive.GetDist(name)
	if err != nil {
		return sendResponse(w, http.StatusNotFound, nil)
	}

	rel := p.NewChildRelease()

	id, err := state.Archive.CopyToStore(r.Body)
	if err != nil {
		return &appError{Error: fmt.Errorf("failed to copy key to store, %v", err)}
	}
	rdr, err := state.Archive.Open(id)
	defer rdr.Close()
	kr, err := openpgp.ReadArmoredKeyRing(rdr)
	if err != nil {
		return sendResponse(w, http.StatusBadRequest, "failed to parse data as  key, "+err.Error())
	}

	if len(kr) != 1 {
		return sendResponse(w, http.StatusBadRequest, "upload 1 key at a time")
	}

	key := kr[0]
	known, err := rel.PubRing()
	if err != nil {
		return &appError{Error: fmt.Errorf("while reading keyring, %v", err)}
	}

	for _, k := range known {
		if key.PrimaryKey.KeyIdString() == k.PrimaryKey.KeyIdString() {
			return doHttpConfigPublicKeysGetHandler(w, r)
		}
	}

	c := rel.Config()
	c.PublicKeyIDs = append(c.PublicKeyIDs, id)
	sort.Sort(ByID(c.PublicKeyIDs))

	newcfgid, err := state.Archive.AddReleaseConfig(*c)
	if err != nil {
		return &appError{Error: fmt.Errorf("failed to add new release config , %v", err)}
	}

	rel.ConfigID = newcfgid
	newrelid, err := state.Archive.AddRelease(rel)
	if err != nil {
		return &appError{Error: fmt.Errorf("failed to update key, %v", err)}
	}

	err = state.Archive.SetDist(name, newrelid)
	if err != nil {
		return &appError{Error: fmt.Errorf("failed to update key, %v", err)}
	}

	err = state.Archive.ReifyRelease(newrelid)
	if err != nil {
		return &appError{Error: fmt.Errorf("failed to update key, %v", err)}
	}

	return doHttpConfigPublicKeysGetHandler(w, r)
}

func doHttpConfigPublicKeysDeleteHandler(w http.ResponseWriter, r *http.Request) *appError {
	if !AuthorisedAdmin(w, r) {
		return sendResponse(w, http.StatusUnauthorized, nil)
	}
	vars := mux.Vars(r)
	name := vars["name"]
	id := vars["id"]

	p, err := state.Archive.GetDist(name)
	if err != nil {
		return sendResponse(w, http.StatusNotFound, nil)
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
		return sendResponse(w, http.StatusNotFound, nil)
	}

	c.PublicKeyIDs = finalKeys
	newcfgid, err := state.Archive.AddReleaseConfig(*c)
	if err != nil {
		return &appError{Error: fmt.Errorf("failed to add new release config, %v", err)}
	}

	rel.ConfigID = newcfgid
	newrelid, err := state.Archive.AddRelease(rel)
	if err != nil {
		return &appError{Error: fmt.Errorf("failed to update key, %v", err)}
	}

	err = state.Archive.SetDist(name, newrelid)
	if err != nil {
		return &appError{Error: fmt.Errorf("failed to update key, %v", err)}
	}

	err = state.Archive.ReifyRelease(newrelid)
	if err != nil {
		return &appError{Error: fmt.Errorf("failed to update key, %v", err)}
	}

	return sendOKResponse(w, "DELETED")
}
