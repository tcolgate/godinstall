package main

import (
	"log"
	"net/http"
)

// Construct the download handler for normal client downloads
func makeHTTPDownloadHandler() http.HandlerFunc {
	fsHandler := http.StripPrefix("/repo/", http.FileServer(http.Dir(state.Archive.PublicDir())))
	downloadHandler := func(w http.ResponseWriter, r *http.Request) {
		handleWithReadLock(fsHandler.ServeHTTP, w, r)
		log.Printf("%s %s %s %s", r.Method, r.Proto, r.URL.Path, r.RemoteAddr)
		state.getCount.Add(1)
	}
	return downloadHandler
}
