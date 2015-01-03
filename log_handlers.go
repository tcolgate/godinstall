package main

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/gorilla/mux"
)

func httpLogHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	curr, err := state.Archive.GetDist(name)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("failed to retrieve store reference for distribution " + name + ", " + err.Error()))
		return
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
			return
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
			return
		}

		if curr.ParentID != nil {
			if !displayTrimmed {
				if trimmerActive {
					if trimAfter > 0 {
						trimAfter--
					} else {

						return
					}
				}
			}
			w.Write([]byte(","))
		} else {
			return
		}
	}
}
