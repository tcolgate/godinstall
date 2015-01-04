package main

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
)

// This build a function to enumerate the distributions
func httpDistsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		handleWithReadLock(doHttpDistsGetHandler, w, r)
	case "PUT":
		handleWithWriteLock(doHttpDistsPutHandler, w, r)
	case "DELETE":
		handleWithWriteLock(doHttpDistsDeleteHandler, w, r)
	default:
		http.Error(w,
			http.StatusText(http.StatusMethodNotAllowed),
			http.StatusMethodNotAllowed)
	}
	return
}

func doHttpDistsGetHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name, nameGiven := vars["name"]
	dists := state.Archive.Dists()
	if !nameGiven {
		SendOKResponse(w, dists)
	} else {
		_, ok := dists[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		rel, err := state.Archive.GetDist(name)
		SendOKOrErrorResponse(w, rel, err, http.StatusInternalServerError)
	}
}

func doHttpDistsPutHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name, _ := vars["name"]
	dists := state.Archive.Dists()
	var rel *Release
	var err error

	_, exists := dists[name]
	if exists {
		http.Error(w,
			"cannot update release directly",
			http.StatusConflict)
	} else {
		seedrel := Release{
			Suite:    name,
			CodeName: name,
			Version:  "0",
		}
		rootid, err := state.Archive.GetReleaseRoot(seedrel)
		if err != nil {
			http.Error(w,
				"Get reelase root failed"+err.Error(),
				http.StatusInternalServerError)
			return
		}

		emptyidx, err := state.Archive.EmptyReleaseIndex()
		if err != nil {
			http.Error(w,
				"Get rempty release index failed"+err.Error(),
				http.StatusInternalServerError)
			return
		}
		newrelid, err := NewRelease(state.Archive, rootid, emptyidx, []ReleaseLogAction{})
		if err != nil {
			http.Error(w,
				"Create new release failed, "+err.Error(),
				http.StatusInternalServerError)
			return
		}
		err = state.Archive.SetDist(name, newrelid)
		if err != nil {
			http.Error(w,
				"Setting new dist tag failed, "+err.Error(),
				http.StatusInternalServerError)
			return
		}
		rel, err = state.Archive.GetDist(name)
		if err != nil {
			http.Error(w,
				"retrieving new dist tag failed, "+err.Error(),
				http.StatusInternalServerError)
			return
		}
	}

	output, err := json.Marshal(rel)
	if err != nil {
		http.Error(w,
			"failed to retrieve distribution details, "+err.Error(),
			http.StatusInternalServerError)
	}
	w.Write(output)
}

func doHttpDistsDeleteHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name, nameGiven := vars["name"]
	dists := state.Archive.Dists()
	if !nameGiven {
		http.Error(w,
			http.StatusText(http.StatusMethodNotAllowed),
			http.StatusMethodNotAllowed)
	} else {
		_, ok := dists[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		err := state.Archive.DeleteDist(name)
		if err != nil {
			http.Error(w,
				"failed to retrieve distribution details, "+err.Error(),
				http.StatusInternalServerError)
		}
	}
}
