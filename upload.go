package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"code.google.com/p/go.crypto/openpgp"
)

type uploadSession struct {
	SessionId  string // Name of the session
	dir        string // Temporary directory for storage
	keyRing    openpgp.KeyRing
	requireSig bool
	changes    *DebChanges
}

func (a *AptServer) NewUploadSession(sessionId string) *uploadSession {
	var s uploadSession
	s.SessionId = sessionId
	s.keyRing = a.pubRing
	s.dir = a.TmpDir + "/" + sessionId

	os.Mkdir(s.dir, os.FileMode(0755))

	a.sessMap.Set(sessionId, &s)
	go pathHandle(a.sessMap, sessionId, a.TTL)

	return &s
}

func (s *uploadSession) HandleReq(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		{
			j, _ := json.Marshal(*s.changes)
			w.Write(j)
			return
		}
	case "PUT", "POST":
		{
			// This response code will depend on upload status
			w.WriteHeader(202)
			w.Write([]byte("Upload some junk"))
			return
		}
	default:
		{
			http.Error(w, "unknown method", http.StatusInternalServerError)
			return
		}
	}
}

func (s *uploadSession) Close() {
	os.Remove(s.dir)
}

func pathHandle(sessMap *SafeMap, s string, timeout time.Duration) {
	time.Sleep(timeout)
	c := sessMap.Get(s)
	if c != nil {
		switch sess := c.(type) {
		case *uploadSession:
			log.Println("Close session")
			sess.Close()
			sessMap.Delete(s)
		default:
			log.Println("Shouldn't get here")
		}
	} else {
		log.Println("Didn't find session")
	}
}

func (s *uploadSession) AddChanges(c *DebChanges) {
	s.changes = c
}
