package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"time"

	"code.google.com/p/go.crypto/openpgp"
	"code.google.com/p/go.crypto/openpgp/clearsign"
)

type uploadSession struct {
	SessionId string // Name of the session
	dir       string // Temporary directory for storage
	keyRing   openpgp.KeyRing
	changes   *DebChanges
}

func (s *uploadSession) HandleReq(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		{
			w.Write([]byte("Display some status info here"))
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

func (s *uploadSession) AddChanges(f multipart.File) (err error) {
	b, err := ioutil.ReadAll(f)
	if err != nil {
		return
	}

	msg, rest := clearsign.Decode(b)
	if len(rest) != 0 {
		err = fmt.Errorf("changes file not signed")
		return
	}

	br := bytes.NewReader(msg.Bytes)
	signer, err := openpgp.CheckDetachedSignature(s.keyRing, br, msg.ArmoredSignature.Body)
	if err != nil {
		return
	}

	log.Println(signer)

	br = bytes.NewReader(msg.Plaintext)
	s.changes, err = ParseDebianChanges(br)
	if err != nil {
		return
	}

	return
}
