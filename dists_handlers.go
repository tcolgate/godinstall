package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"golang.org/x/net/context"
)

// This build a function to enumerate the distributions
func httpDistsHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) *appError {
	switch r.Method {
	case "GET":
		return handleWithReadLock(doHTTPDistsGetHandler, ctx, w, r)
	case "PUT":
		return handleWithWriteLock(doHTTPDistsPutHandler, ctx, w, r)
	case "DELETE":
		return handleWithWriteLock(doHTTPDistsDeleteHandler, ctx, w, r)
	default:
		return sendResponse(w, http.StatusMethodNotAllowed, nil)
	}
}

func doHTTPDistsGetHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) *appError {
	vars := mux.Vars(r)
	name, nameGiven := vars["name"]
	dists := state.Archive.Dists()
	if !nameGiven {
		return sendResponse(w, http.StatusOK, dists)
	}

	rel, err := state.Archive.GetDist(name)
	switch {
	case err == nil:
		return sendOKResponse(w, rel)
	case os.IsNotExist(err):
		return sendResponse(w, http.StatusNotFound, nil)
	default:
		return &appError{Error: err}
	}
}

func doHTTPDistsPutHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) *appError {
	if !AuthorisedAdmin(ctx, w, r) {
		return sendResponse(w, http.StatusUnauthorized, nil)
	}
	vars := mux.Vars(r)
	name, _ := vars["name"]
	dists := state.Archive.Dists()
	var rel *Release
	var err error

	_, exists := dists[name]
	if exists {
		return sendResponse(w, http.StatusConflict, "cannot update release directly")
	}

	seedrel := Release{
		Suite:    name,
		CodeName: name,
		Version:  "0",
	}
	rootid, err := state.Archive.GetReleaseRoot(seedrel)
	if err != nil {
		return &appError{Error: fmt.Errorf("Get reelase root failed, %v", err)}
	}

	emptyidx, err := state.Archive.EmptyReleaseIndex()
	if err != nil {
		return &appError{Error: fmt.Errorf("Get rempty release index failed, %v", err)}
	}
	newrelid, err := NewRelease(state.Archive, rootid, emptyidx, []ReleaseLogAction{})
	if err != nil {
		return &appError{Error: fmt.Errorf("Create new release failed, %v", err)}
	}
	err = state.Archive.SetDist(name, newrelid)
	if err != nil {
		return &appError{Error: fmt.Errorf("Setting new dist tag failed, %v", err)}
	}
	rel, err = state.Archive.GetDist(name)
	if err != nil {
		return &appError{Error: fmt.Errorf("retrieving new dist tag failed, %v", err)}
	}

	return sendOKResponse(w, rel)
}

func doHTTPDistsDeleteHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) *appError {
	if !AuthorisedAdmin(ctx, w, r) {
		return sendResponse(w, http.StatusUnauthorized, nil)
	}
	vars := mux.Vars(r)
	name, nameGiven := vars["name"]
	dists := state.Archive.Dists()

	if !nameGiven {
		return sendResponse(w, http.StatusBadRequest, nil)
	}

	_, ok := dists[name]
	if !ok {
		return sendResponse(w, http.StatusNotFound, nil)
	}

	err := state.Archive.DeleteDist(name)
	if err != nil {
		return &appError{Error: fmt.Errorf("failed to retrieve distribution details,%v", err)}
	}

	return nil
}
