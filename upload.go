package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"code.google.com/p/go-uuid/uuid"
	"code.google.com/p/go.crypto/openpgp"
)

type UploadSessioner interface {
	SessionID() string
	SessionURL() string
	AddChanges(*DebChanges)
	Changes() *DebChanges
	AddFile(*DebFile)
	Files() map[string]*DebFile
	HandleReq(w http.ResponseWriter, r *http.Request)
	Close()
}

type uploadSession struct {
	SessionId  string // Name of the session
	dir        string // Temporary directory for storage
	keyRing    openpgp.KeyRing
	requireSig bool
	changes    *DebChanges
	files      map[string]*DebFile
}

func NewUploadSessioner(a *AptServer) UploadSessioner {
	var s uploadSession
	s.SessionId = uuid.New()
	s.keyRing = a.pubRing
	s.dir = a.TmpDir + "/" + s.SessionId

	os.Mkdir(s.dir, os.FileMode(0755))

	a.sessMap.Set(s.SessionId, &s)
	go pathHandle(a.sessMap, s.SessionId, a.TTL)

	return &s
}

func (s *uploadSession) SessionID() string {
	return s.SessionId
}

func (s *uploadSession) SessionURL() string {
	return "/package/upload/" + s.SessionId
}

func (s *uploadSession) HandleReq(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		{
			j := UploadSessionToJSON(s)
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

func (s *uploadSession) Changes() *DebChanges {
	return s.changes
}

func (s *uploadSession) AddFile(f *DebFile) {
	s.files[f.Filename] = f
}

func (s *uploadSession) Files() map[string]*DebFile {
	return s.files
}

func UploadSessionToJSON(s UploadSessioner) []byte {
	resp := struct {
		SessionId  string
		SessionURL string
		Changes    DebChanges
	}{
		s.SessionID(),
		s.SessionURL(),
		*s.Changes(),
	}
	j, _ := json.Marshal(resp)
	return j
}
