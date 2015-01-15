package main

import (
	"log"
	"net/http"

	"golang.org/x/net/context"
)

// Construct the download handler for normal client downloads
func makeHTTPDownloadHandler() appHandler {
	fsHandler := func(ctx context.Context, w http.ResponseWriter, r *http.Request) *appError {
		http.StripPrefix("/repo/", http.FileServer(http.Dir(state.Archive.PublicDir()))).ServeHTTP(w, r)
		return nil
	}
	downloadHandler := func(ctx context.Context, w http.ResponseWriter, r *http.Request) *appError {
		log.Printf("%s %s %s %s", r.Method, r.Proto, r.URL.Path, r.RemoteAddr)
		state.getCount.Add(1)
		return handleWithReadLock(fsHandler, ctx, w, r)
	}
	return downloadHandler
}
