package main

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"time"

	"code.google.com/p/go-uuid/uuid"
	"code.google.com/p/go.crypto/openpgp"
)

// Manage upload sessions
type UploadSessionManager interface {
	GetSession(string) (UploadSessioner, bool)
	NewUploadSession(*DebChanges) (UploadSessioner, error)
}

type uploadSessionManager struct {
	sessMap   *SafeMap
	aptServer AptServer
	finished  chan UploadSessioner
}

func NewUploadSessionManager(a AptServer) UploadSessionManager {
	usm := uploadSessionManager{}
	usm.sessMap = NewSafeMap()
	usm.aptServer = a

	return &usm
}

func (usm *uploadSessionManager) GetSession(sid string) (UploadSessioner, bool) {
	val := usm.sessMap.Get(sid)
	if val == nil {
		return nil, false
	}

	switch t := val.(type) {
	default:
		{
			return nil, false
		}
	case UploadSessioner:
		{
			return t.(UploadSessioner), true
		}
	}
}

type UploadSessioner interface {
	SessionID() string
	SessionURL() string
	AddChanges(*DebChanges)
	Changes() *DebChanges
	IsComplete() bool
	AddFile(*ChangesItem) error
	Dir() string
	Files() map[string]*ChangesItem
	Close()
	json.Marshaler
}

type uploadSession struct {
	SessionId  string // Name of the session
	changes    *DebChanges
	dir        string // Temporary directory for storage
	keyRing    openpgp.KeyRing
	requireSig bool
	postHook   string
}

func (usm *uploadSessionManager) NewUploadSession(changes *DebChanges) (UploadSessioner, error) {
	var s uploadSession
	s.SessionId = uuid.New()
	s.keyRing = usm.aptServer.PubRing
	s.dir = usm.aptServer.TmpDir + "/" + s.SessionId
	s.postHook = usm.aptServer.PostUploadHook

	os.Mkdir(s.dir, os.FileMode(0755))
	os.Mkdir(s.dir+"/upload", os.FileMode(0755))

	s.AddChanges(changes)

	go usm.handler(&s)

	return &s, nil
}

// Go routine for handling upload sessions
func (usm *uploadSessionManager) handler(s UploadSessioner) {
	usm.sessMap.Set(s.SessionID(), s)

	defer func() {
		usm.sessMap.Set(s.SessionID(), nil)
		s.Close()
	}()

	for {
		select {
		case <-time.After(usm.aptServer.TTL):
			{
				return
			}
		}
	}
}

func (s *uploadSession) SessionID() string {
	return s.SessionId
}

func (s *uploadSession) SessionURL() string {
	return "/package/upload/" + s.SessionId
}

func (s *uploadSession) Close() {
	os.RemoveAll(s.dir)
}

func (s *uploadSession) AddChanges(c *DebChanges) {
	s.changes = c
}

func (s *uploadSession) Changes() *DebChanges {
	return s.changes
}

func (s *uploadSession) AddFile(upload *ChangesItem) (err error) {
	// Check that there is an upload slot
	expectedFile, ok := s.changes.Files[upload.Filename]
	if !ok {
		return errors.New("File not listed in upload set")
	}

	if expectedFile.Uploaded {
		return errors.New("File already uploaded")
	}

	md5er := md5.New()
	sha1er := sha1.New()
	sha256er := sha256.New()
	hasher := io.MultiWriter(md5er, sha1er, sha256er)
	tee := io.TeeReader(upload.data, hasher)
	tmpFilename := s.dir + "/upload/" + upload.Filename
	storeFilename := s.dir + "/" + upload.Filename
	tmpfile, err := os.Create(tmpFilename)
	if err != nil {
		return errors.New("Upload temporary file failed, " + err.Error())
	}
	defer func() {
		if err != nil {
			os.Remove(tmpFilename)
		}
	}()

	_, err = io.Copy(tmpfile, tee)
	if err != nil {
		return errors.New("Upload failed: " + err.Error())
	}

	md5 := hex.EncodeToString(md5er.Sum(nil))
	sha1 := hex.EncodeToString(sha1er.Sum(nil))
	sha256 := hex.EncodeToString(sha256er.Sum(nil))

	if expectedFile.Md5 != md5 ||
		expectedFile.Sha1 != sha1 ||
		expectedFile.Sha256 != sha256 {
		return errors.New("Uploaded file hashes do not match")
	}

	if s.postHook != "" {
		err = exec.Command(s.postHook, tmpFilename).Run()
		if !err.(*exec.ExitError).Success() {
			return errors.New("Post upload hook failed, ")
		}
	}

	if err == nil {
		os.Rename(tmpFilename, storeFilename)
		expectedFile.Uploaded = true
	}

	return
}

func (s *uploadSession) Dir() string {
	return s.dir
}

func (s *uploadSession) Files() map[string]*ChangesItem {
	return s.changes.Files
}

func (s *uploadSession) IsComplete() bool {
	complete := true
	for _, f := range s.Files() {
		if !f.Uploaded {
			complete = false
		}
	}
	return complete
}

func (s *uploadSession) MarshalJSON() (j []byte, err error) {
	resp := struct {
		SessionId  string
		SessionURL string
		Changes    DebChanges
	}{
		s.SessionID(),
		s.SessionURL(),
		*s.Changes(),
	}
	j, err = json.Marshal(resp)
	return
}
