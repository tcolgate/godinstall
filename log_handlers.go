package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"golang.org/x/net/context"
)

func httpLogHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) *appError {
	switch r.Method {
	case "GET":
		return handleWithReadLock(doHTTPLogGetHandler, ctx, w, r)
	default:
		return sendResponse(w, http.StatusMethodNotAllowed, nil)
	}
}

func doHTTPLogGetHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) *appError {
	vars := mux.Vars(r)
	name := vars["name"]

	curr, err := state.Archive.GetDist(name)
	switch {
	case err == nil:
	case os.IsNotExist(err):
		return sendResponse(w, http.StatusNotFound, nil)
	default:
		return &appError{Error: fmt.Errorf("failed to retrieve store reference, %v", err)}
	}

	w.Write([]byte("["))
	defer w.Write([]byte("]"))

	displayTrimmed := false
	trimmerActive := false
	trimAfter := int32(0)

	for {
		output, err := json.Marshal(curr)
		if err != nil {
			log.Println("Could not marshal json object, " + err.Error())
			continue
		}
		w.Write(output)

		if !displayTrimmed && !trimmerActive && curr.TrimAfter != 0 {
			trimmerActive = true
			trimAfter = curr.TrimAfter
		}

		curr, err = state.Archive.GetRelease(curr.ParentID)
		if err != nil {
			log.Println("Could not get parent, " + err.Error())
			return nil
		}

		// Reached end of history
		if state.Archive.EmptyFileID().String() == curr.ParentID.String() ||
			curr.Release.String() == "" {
			return nil
		}

		if curr.ParentID == nil {
			return nil
		}

		if !displayTrimmed && trimmerActive {
			if trimAfter == 0 {
				return nil
			}
			trimAfter--
		}

		w.Write([]byte(","))
	}
}
