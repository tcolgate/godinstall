package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"

	"code.google.com/p/go.crypto/openpgp"
	"github.com/gorilla/mux"
	"golang.org/x/net/context"
)

// Lots of DRY fail here, can probably clear this by wrapping requests
// that result in a repo update

// This build a function to enumerate the distributions
func httpConfigHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) *appError {
	switch r.Method {
	case "GET":
		return handleWithReadLock(doHTTPConfigGetHandler, ctx, w, r)
	case "PUT":
		return handleWithWriteLock(doHTTPConfigPutHandler, ctx, w, r)
	default:
		return sendResponse(w, http.StatusMethodNotAllowed, nil)
	}
}

// This build a function to view the config of a distribution
func doHTTPConfigGetHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) *appError {
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

// This build a function to update the config of a distribution
func doHTTPConfigPutHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) *appError {

	// We use this to determine if the fields in the
	// update were included or left out
	type configUpdate struct {
		PruneRules              *string
		VerifyChanges           *bool
		AcceptLoneDebs          *bool
		PoolPattern             *string
		VerifyDebs              *bool
		AutoTrimLength          *int
		AutoTrim                *bool
		VerifyChangesSufficient *bool
	}

	vars := mux.Vars(r)
	name := vars["name"]

	rel, err := state.Archive.GetDist(name)
	switch {
	case err == nil:
	case os.IsNotExist(err):
		return sendResponse(w, http.StatusNotFound, nil)
	default:
		return &appError{Error: err}
	}

	cfg := rel.Config()

	decoder := json.NewDecoder(r.Body)
	var d configUpdate
	err = decoder.Decode(&d)
	if err != nil {
		return sendResponse(w, http.StatusBadRequest, nil)
	}

	var acts []ReleaseLogAction

	if d.PruneRules != nil && *d.PruneRules != cfg.PruneRules {
		acts = append(acts, ReleaseLogAction{
			Type:        ActionCONFIGCHANGE,
			Description: fmt.Sprintf("%s changed from %v to %v", cfg.PruneRules, *d.PruneRules),
		})
		cfg.PruneRules = *d.PruneRules
	}

	if d.VerifyChanges != nil && *d.VerifyChanges != cfg.VerifyChanges {
		acts = append(acts, ReleaseLogAction{
			Type:        ActionCONFIGCHANGE,
			Description: fmt.Sprintf("VerifyChanges changed from %v to %v", cfg.VerifyChanges, *d.VerifyChanges),
		})
		cfg.VerifyChanges = *d.VerifyChanges
	}

	if d.AcceptLoneDebs != nil && *d.AcceptLoneDebs != cfg.AcceptLoneDebs {
		acts = append(acts, ReleaseLogAction{
			Type:        ActionCONFIGCHANGE,
			Description: fmt.Sprintf("AcceptLoneDebs changed from %v to %v", cfg.AcceptLoneDebs, *d.AcceptLoneDebs),
		})
		cfg.AcceptLoneDebs = *d.AcceptLoneDebs
	}

	if d.PoolPattern != nil && *d.PoolPattern != cfg.PoolPattern {
		acts = append(acts, ReleaseLogAction{
			Type:        ActionCONFIGCHANGE,
			Description: fmt.Sprintf("PoolPattern changed from %v to %v", cfg.PoolPattern, *d.PoolPattern),
		})
		cfg.PoolPattern = *d.PoolPattern
	}

	if d.VerifyDebs != nil && *d.VerifyDebs != cfg.VerifyDebs {
		acts = append(acts, ReleaseLogAction{
			Type:        ActionCONFIGCHANGE,
			Description: fmt.Sprintf("VerifyDebs changed from %v to %v", cfg.VerifyDebs, *d.VerifyDebs),
		})
		cfg.VerifyDebs = *d.VerifyDebs
	}

	if d.AutoTrimLength != nil && *d.AutoTrimLength != cfg.AutoTrimLength {
		acts = append(acts, ReleaseLogAction{
			Type:        ActionCONFIGCHANGE,
			Description: fmt.Sprintf("AutoTrimLength changed from %v to %v", cfg.AutoTrimLength, *d.AutoTrimLength),
		})
		cfg.AutoTrimLength = *d.AutoTrimLength
	}

	if d.AutoTrim != nil && *d.AutoTrim != cfg.AutoTrim {
		acts = append(acts, ReleaseLogAction{
			Type:        ActionCONFIGCHANGE,
			Description: fmt.Sprintf("AutoTrim changed from %v to %v", cfg.AutoTrim, *d.AutoTrim),
		})
		cfg.AutoTrim = *d.AutoTrim
	}

	if d.VerifyChangesSufficient != nil && *d.VerifyChangesSufficient != cfg.VerifyChangesSufficient {
		acts = append(acts, ReleaseLogAction{
			Type:        ActionCONFIGCHANGE,
			Description: fmt.Sprintf("VerifyChangesSufficient changed from %v to %v", cfg.VerifyChangesSufficient, *d.VerifyChangesSufficient),
		})
		cfg.VerifyChangesSufficient = *d.VerifyChangesSufficient
	}

	if len(acts) == 0 {
		// No actions, do nothing
		return doHTTPConfigGetHandler(ctx, w, r)
	}

	n := rel.NewChild()
	n.Actions = acts

	newcfgid, err := state.Archive.AddReleaseConfig(*cfg)
	if err != nil {
		return &appError{
			Error: errors.New("failed to add new release config, " + err.Error()),
		}
	}

	n.ConfigID = newcfgid

	// Since the pool pattern may have changed, we need to update the release
	n.updateReleasefiles()

	newrelid, err := state.Archive.AddRelease(n)
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

	return doHTTPConfigGetHandler(ctx, w, r)
}

// This build a function to enumerate the distributions
func httpConfigSigningKeyHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) *appError {
	switch r.Method {
	case "GET":
		return handleWithReadLock(doHTTPConfigSigningKeyGetHandler, ctx, w, r)
	case "PUT", "POST":
		return handleWithWriteLock(doHTTPConfigSigningKeyPutHandler, ctx, w, r)
	case "DELETE":
		return handleWithWriteLock(doHTTPConfigSigningKeyDeleteHandler, ctx, w, r)
	default:
		return sendResponse(w, http.StatusMethodNotAllowed, nil)
	}
}

func doHTTPConfigSigningKeyGetHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) *appError {
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

func doHTTPConfigSigningKeyPutHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) *appError {
	if !AuthorisedAdmin(ctx, w, r) {
		return sendResponse(w, http.StatusUnauthorized, nil)
	}

	vars := mux.Vars(r)
	name := vars["name"]

	p, err := state.Archive.GetDist(name)
	if err != nil {
		return sendResponse(w, http.StatusNotFound, nil)
	}

	rel := p.NewChild()

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
		return doHTTPConfigSigningKeyGetHandler(ctx, w, r)
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
		return doHTTPConfigSigningKeyGetHandler(ctx, w, r)
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

	return doHTTPConfigSigningKeyGetHandler(ctx, w, r)
}

func doHTTPConfigSigningKeyDeleteHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) *appError {
	if !AuthorisedAdmin(ctx, w, r) {
		return sendResponse(w, http.StatusUnauthorized, nil)
	}
	vars := mux.Vars(r)
	name := vars["name"]

	p, err := state.Archive.GetDist(name)
	if err != nil {
		return sendResponse(w, http.StatusNotFound, nil)
	}

	rel := p.NewChild()
	cfg := rel.Config()
	if cfg.SigningKeyID == nil {
		return doHTTPConfigSigningKeyGetHandler(ctx, w, r)
	}

	cfg.SigningKeyID = nil

	newcfgid, err := state.Archive.AddReleaseConfig(*cfg)
	if err != nil {
		return &appError{Error: fmt.Errorf("failed to parse add new release config, %v", err)}
	}

	rel.ConfigID = newcfgid
	if !rel.updateReleaseSigFiles() {
		return doHTTPConfigSigningKeyGetHandler(ctx, w, r)
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
func httpConfigPublicKeysHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) *appError {
	switch r.Method {
	case "GET":
		return handleWithReadLock(doHTTPConfigPublicKeysGetHandler, ctx, w, r)
	case "POST":
		return handleWithWriteLock(doHTTPConfigPublicKeysPostHandler, ctx, w, r)
	case "DELETE":
		return handleWithWriteLock(doHTTPConfigPublicKeysDeleteHandler, ctx, w, r)
	default:
		return sendResponse(w, http.StatusMethodNotAllowed, nil)
	}
}

// For managing public keys in a config
func doHTTPConfigPublicKeysGetHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) *appError {
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
	}
	for _, id := range ids {
		key := id.PrimaryKey
		if key.KeyIdString() == reqid {
			// This is a bit boring, should output more
			return sendOKResponse(w, key.KeyIdShortString())
		}
	}
	return sendResponse(w, http.StatusNotFound, nil)
}

func doHTTPConfigPublicKeysPostHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) *appError {
	if !AuthorisedAdmin(ctx, w, r) {
		return sendResponse(w, http.StatusUnauthorized, nil)
	}
	vars := mux.Vars(r)
	name := vars["name"]

	p, err := state.Archive.GetDist(name)
	if err != nil {
		return sendResponse(w, http.StatusNotFound, nil)
	}

	rel := p.NewChild()

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
			return doHTTPConfigPublicKeysGetHandler(ctx, w, r)
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

	return doHTTPConfigPublicKeysGetHandler(ctx, w, r)
}

func doHTTPConfigPublicKeysDeleteHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) *appError {
	if !AuthorisedAdmin(ctx, w, r) {
		return sendResponse(w, http.StatusUnauthorized, nil)
	}
	vars := mux.Vars(r)
	name := vars["name"]
	id := vars["id"]

	p, err := state.Archive.GetDist(name)
	if err != nil {
		return sendResponse(w, http.StatusNotFound, nil)
	}

	rel := p.NewChild()

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
