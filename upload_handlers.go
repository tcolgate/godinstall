package main

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

// This build a function to despatch upload requests
func httpUploadHandler(w http.ResponseWriter, r *http.Request) {

	vars := mux.Vars(r)

	branchName, ok := vars["name"]
	if !ok {
		branchName = "master"
	}

	session, found := vars["session"]

	var resp *ServerResponse

	//Maybe in a cookie?
	if !found {
		cookie, err := r.Cookie(cfg.CookieName)
		if err == nil {
			session = cookie.Value
		}
	}

	switch r.Method {
	case "GET":
		{
			s, ok := state.SessionManager.GetSession(session)
			if !ok {
				resp = NewServerResponse(http.StatusNotFound, "File not found")
				w.WriteHeader(resp.StatusCode)
				w.Write(resp.Message)
				return
			}

			resp = s.Status()
		}
	case "PUT", "POST":
		{
			changesReader, otherParts, err := ChangesFromHTTPRequest(r)
			if err != nil {
				resp = NewServerResponse(http.StatusBadRequest, err.Error())
				w.WriteHeader(resp.StatusCode)
				w.Write(resp.Message)
				return
			}

			if session == "" {
				// We don't have an active session, lets create one
				rel, err := state.Archive.GetDist(branchName)
				if err != nil {
					w.WriteHeader(http.StatusNotFound)
					w.Write([]byte("Unknown distribution " + branchName + ", " + err.Error()))
					return
				}

				var loneDeb bool
				if changesReader == nil {
					if !rel.Config().AcceptLoneDebs {
						err = errors.New("No debian changes file in request")
						resp = NewServerResponse(http.StatusBadRequest, err.Error())
						w.WriteHeader(resp.StatusCode)
						w.Write(resp.Message)
						return
					}

					if len(otherParts) != 1 {
						err = errors.New("Too many files in upload request without changes file present")
						resp = NewServerResponse(http.StatusBadRequest, err.Error())
						w.WriteHeader(resp.StatusCode)
						w.Write(resp.Message)
						return
					}

					if !strings.HasSuffix(otherParts[0].Filename, ".deb") {
						err = errors.New("Lone files for upload must end in .deb")
						resp = NewServerResponse(http.StatusBadRequest, err.Error())
						w.WriteHeader(resp.StatusCode)
						w.Write(resp.Message)
						return
					}

					loneDeb = true
				}

				session, err = state.SessionManager.NewSession(rel, changesReader, loneDeb)
				if err != nil {
					resp = NewServerResponse(http.StatusBadRequest, err.Error())
					w.WriteHeader(resp.StatusCode)
					w.Write(resp.Message)
					return
				}

				cookie := http.Cookie{
					Name:     cfg.CookieName,
					Value:    session,
					Expires:  time.Now().Add(cfg.TTL),
					HttpOnly: false,
					Path:     "/upload",
				}
				http.SetCookie(w, &cookie)
			}
			if err != nil {
				resp = NewServerResponse(http.StatusBadRequest, err.Error())
				w.WriteHeader(resp.StatusCode)
				w.Write(resp.Message)
				return
			}

			sess, ok := state.SessionManager.GetSession(session)
			if !ok {
				resp = NewServerResponse(http.StatusNotFound, "File Not Found")
				w.WriteHeader(resp.StatusCode)
				w.Write(resp.Message)
				return
			}

			resp = sess.Status()

			for _, part := range otherParts {
				fh, err := part.Open()
				if err != nil {
					resp = NewServerResponse(http.StatusBadRequest, fmt.Sprintf("Error opening mime item, %s", err.Error()))
					w.WriteHeader(resp.StatusCode)
					w.Write(resp.Message)
					return
				}

				uf := UploadFile{
					Name:   part.Filename,
					reader: fh,
				}
				resp = sess.AddFile(&uf)
			}
		}
	}

	if resp.StatusCode == 0 {
		http.Error(w, "AptServer response statuscode not set", http.StatusInternalServerError)
	} else {
		w.WriteHeader(resp.StatusCode)
		w.Write(resp.Message)
	}
}
