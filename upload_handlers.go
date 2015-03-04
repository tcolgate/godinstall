package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"golang.org/x/net/context"
)

// This build a function to despatch upload requests
func httpUploadHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) *appError {

	vars := mux.Vars(r)

	branchName, ok := vars["name"]
	if !ok {
		branchName = "master"
	}

	session, found := vars["session"]

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
				return sendResponse(w, http.StatusNotFound, nil)
			}

			return sendOKResponse(w, s.Status())
		}
	case "PUT", "POST":
		{
			changesReader, otherParts, err := ChangesFromHTTPRequest(r)
			if err != nil {
				return sendResponse(w, http.StatusBadRequest, err.Error())
			}

			if session == "" {
				// We don't have an active session, lets create one
				rel, err := state.Archive.GetDist(branchName)
				switch {
				case err == nil:
				case os.IsNotExist(err):
					return sendResponse(w, http.StatusNotFound, nil)
				default:
					return &appError{Error: err}
				}

				var loneDeb bool
				if changesReader == nil {
					if !rel.Config().AcceptLoneDebs {
						return sendResponse(w, http.StatusBadRequest, "No debian changes file in request")
					}

					if len(otherParts) != 1 {
						return sendResponse(w, http.StatusBadRequest, "Too many files in upload request without changes file present")
					}

					if !strings.HasSuffix(otherParts[0].Filename, ".deb") {
						return sendResponse(w, http.StatusBadRequest, "Lone files for upload must end in .deb")
					}

					loneDeb = true
				}

				session, err = state.SessionManager.NewSession(rel, changesReader, loneDeb)
				if err != nil {
					return &appError{Error: fmt.Errorf("failed creating session, %v", err)}
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

			sess, ok := state.SessionManager.GetSession(session)
			if !ok {
				return sendResponse(w, http.StatusNotFound, nil)
			}

			for _, part := range otherParts {
				fh, err := part.Open()
				if err != nil {
					return sendResponse(w, http.StatusBadRequest, "Error opening mime item, "+err.Error())
				}

				sess = sess.AddFile(part.Filename, fh)
				if sess.Err() != nil {
					return sendResponse(w, http.StatusBadRequest, "Error , "+sess.Err().Error())
				}
			}

			if err := sess.Err(); err != nil {
				switch err {
				default:
					return &appError{
						Code:  http.StatusInternalServerError,
						Error: err,
					}
				}
			}

			resp := sess.Status()
			return sendOKResponse(w, resp)
		}
	default:
		return sendResponse(w, http.StatusMethodNotAllowed, nil)
	}
}
