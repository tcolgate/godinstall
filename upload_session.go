package main

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"os/exec"

	"code.google.com/p/go-uuid/uuid"
	"code.google.com/p/go.crypto/openpgp"
)

type UploadSessioner interface {
	SessionID() string
	SessionURL() string
	Changes() *DebChanges
	IsComplete() bool
	AddItem(*ChangesItem) (bool, error)
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
	incoming   chan addItemMsg
	close      chan closeMsg
}

type closeMsg struct{}

func NewUploadSession(
	changes *DebChanges,
	keyRing openpgp.EntityList,
	postUploadHook string,
	tmpDir string,
) UploadSessioner {
	var s uploadSession
	s.SessionId = uuid.New()
	s.changes = changes
	s.keyRing = keyRing
	s.postHook = postUploadHook
	s.dir = tmpDir + "/" + s.SessionId

	os.Mkdir(s.Dir(), os.FileMode(0755))
	os.Mkdir(s.Dir()+"/upload", os.FileMode(0755))

	s.incoming = make(chan addItemMsg)
	s.close = make(chan closeMsg)

	go s.handler()

	return &s
}

type addItemResp struct {
	complete bool
	err      error
}

type addItemMsg struct {
	file *ChangesItem
	resp chan addItemResp
}

// All item additions to this session are
// serialized through this routine
func (s *uploadSession) handler() {
	for {
		select {
		case <-s.close:
			{
				os.RemoveAll(s.dir)
			}
		case item := <-s.incoming:
			{
				err := s.doAddItem(item.file)
				item.resp <- addItemResp{s.IsComplete(), err}
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
	s.close <- closeMsg{}
}

func (s *uploadSession) Changes() *DebChanges {
	return s.changes
}

func (s *uploadSession) AddItem(upload *ChangesItem) (bool, error) {
	done := make(chan addItemResp)
	go func() {
		s.incoming <- addItemMsg{
			file: upload,
			resp: done,
		}
	}()
	resp := <-done
	return resp.complete, resp.err
}

func (s *uploadSession) doAddItem(upload *ChangesItem) (err error) {
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
	log.Println(s)
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
