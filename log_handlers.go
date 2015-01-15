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
		return handleWithReadLock(doHttpLogGetHandler, ctx, w, r)
	default:
		return sendResponse(w, http.StatusMethodNotAllowed, nil)
	}
}

func doHttpLogGetHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) *appError {
	vars := mux.Vars(r)
	name := vars["name"]

	curr, err := state.Archive.GetDist(name)
	switch {
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

		if !displayTrimmed {
			if !trimmerActive && curr.TrimAfter != 0 {
				trimmerActive = true
				trimAfter = curr.TrimAfter
			}
		}

		curr, err = state.Archive.GetRelease(curr.ParentID)
		if err != nil {
			log.Println("Could not get parent, " + err.Error())
			continue
		}

		if curr.ParentID != nil {
			if !displayTrimmed {
				if trimmerActive {
					if trimAfter > 0 {
						trimAfter--
					} else {
						return nil
					}
				}
			}
			w.Write([]byte(","))
		} else {
			return nil
		}
	}
	return nil
}
