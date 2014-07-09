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
	"net/http"
	"os"
	"os/exec"

	"code.google.com/p/go-uuid/uuid"
)

type UploadSessioner interface {
	SessionID() string
	AddItem(*ChangesItem) AptServerResponder
	Close()
	DoneChan() chan struct{}
	Status() AptServerResponder
	json.Marshaler
}

type uploadSession struct {
	SessionId  string // Name of the session
	changes    *DebChanges
	dir        string // Temporary directory for storage
	requireSig bool
	uploadHook HookRunner

	// Channels for requests
	incoming  chan addItemMsg
	close     chan closeMsg // A channel for close messages
	getstatus chan getStatusMsg

	// output session
	done chan struct{}
}

func NewUploadSession(
	changes *DebChanges,
	tmpDirBase *string,
	uploadHook HookRunner,
	done chan struct{},
) UploadSessioner {
	var s uploadSession
	s.done = done
	s.SessionId = uuid.New()
	s.changes = changes
	s.uploadHook = uploadHook
	s.dir = *tmpDirBase + "/" + s.SessionId

	os.Mkdir(s.dir, os.FileMode(0755))
	os.Mkdir(s.dir+"/upload", os.FileMode(0755))

	s.incoming = make(chan addItemMsg)
	s.close = make(chan closeMsg)
	s.getstatus = make(chan getStatusMsg)

	go s.handler()

	return &s
}

type closeMsg struct{}

type addItemMsg struct {
	file *ChangesItem
	resp chan AptServerResponder
}

type getStatusMsg struct {
	resp chan AptServerResponder
}

// All item additions to this session are
// serialized through this routine
func (s *uploadSession) handler() {
	defer func() {
		err := os.RemoveAll(s.dir)
		if err != nil {
			log.Println(err)
		}
		msg := new(struct{})
		s.done <- *msg
	}()
	for {
		select {
		case <-s.close:
			{
				msg := new(struct{})
				s.done <- *msg
				return
			}
		case msg := <-s.getstatus:
			{
				msg.resp <- AptServerMessage(http.StatusOK, s)
			}
		case msg := <-s.incoming:
			{
				err := s.doAddItem(msg.file)

				if err != nil {
					msg.resp <- AptServerMessage(http.StatusBadRequest, err.Error())
					break
				}

				complete := true
				for _, f := range s.changes.Files {
					if !f.Uploaded {
						complete = false
					}
				}

				if !complete {
					msg.resp <- AptServerMessage(http.StatusAccepted, s)
					break
				}

				// Need to do the update and return the response
				return
			}
		}
	}
}

func (s *uploadSession) SessionID() string {
	return s.SessionId
}

func (s *uploadSession) Close() {
	s.close <- closeMsg{}
}

func (s *uploadSession) DoneChan() chan struct{} {
	return s.done
}

func (s *uploadSession) Status() AptServerResponder {
	done := make(chan AptServerResponder)
	go func() {
		s.getstatus <- getStatusMsg{
			resp: done,
		}
	}()
	resp := <-done
	return resp
}

func (s *uploadSession) AddItem(upload *ChangesItem) AptServerResponder {
	done := make(chan AptServerResponder)
	go func() {
		s.incoming <- addItemMsg{
			file: upload,
			resp: done,
		}
	}()
	resp := <-done
	return resp
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

	err = s.uploadHook.Run(tmpFilename)
	if !err.(*exec.ExitError).Success() {
		return errors.New("Post upload hook failed, ")
	}

	if err == nil {
		os.Rename(tmpFilename, storeFilename)
		expectedFile.Uploaded = true
	}

	return
}

func (s *uploadSession) MarshalJSON() (j []byte, err error) {
	resp := struct {
		SessionId string
		Changes   DebChanges
	}{
		s.SessionId,
		*s.changes,
	}
	j, err = json.Marshal(resp)
	return
}

// Upload session stores keep the state for an upload
// session. We need to be able to mock this out to
// avoid testing disk content
type UploadSessionStorer interface {
}

// On disk storage for upload content
type uploadDiskStorer struct {
}

func NewUploadDiskStorer() UploadSessionStorer {
	newstore := uploadDiskStorer{}
	return newstore
}

// RAM storage for upload content, used for testing
type uploadMemStorer struct {
}

func NewUploadMemStorer() UploadSessionStorer {
	newstore := uploadMemStorer{}
	return newstore
}
