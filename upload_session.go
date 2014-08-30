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
	"strings"

	"code.google.com/p/go-uuid/uuid"
	"code.google.com/p/go.crypto/openpgp"
)

// UploadItem describes a specific item to be uploaded along
// with the changes file
type UploadItem struct {
	Filename         string
	Size             int64
	Md5              string
	Sha1             string
	Sha256           string
	Uploaded         bool
	SignedBy         []string
	UploadHookResult HookOutput

	data io.Reader
}

// This defines an interface to an individual upload session
type UploadSessioner interface {
	SessionID() string                      // return the UUID for this session
	Directory() string                      // returnt he base directory for the verified uploaded files
	Items() map[string]*UploadItem          // return the changes file for this session
	AddItem(*UploadItem) AptServerResponder // Add the given item to this session
	Close()                                 // Close, and clear up, any remaining files
	DoneChan() chan struct{}                // This returns a channel that anounces copletion
	Status() AptServerResponder             // Return the status of this session
	json.Marshaler                          // All session implementations should serialize to JSON
}

// An UploadSession for uploading using a changes file
type changesSession struct {
	SessionId    string             // Name of the session
	changes      *DebChanges        // The changes file for this session
	validateDebs bool               // Validate uploaded. deb files
	keyRing      openpgp.EntityList // Keyring for validation
	dir          string             // Temporary directory for storage
	requireSig   bool               // Check debian package signatures
	uploadHook   HookRunner         // A hook to run after a successful upload
	store        Storer             // Blob store to keep files in

	// Channels for requests
	// TODO revisit this
	incoming  chan addItemMsg   // New item upload requests
	close     chan closeMsg     // A channel for close messages
	getstatus chan getStatusMsg // A channel for responding to status requests

	// output session
	// TODO revisit this
	done     chan struct{}      // A channel to be informed of closure on
	finished chan UpdateRequest // A channel to anounce completion and trigger a repo update
}

func NewChangesSession(
	changes *DebChanges,
	validateDebs bool,
	keyRing openpgp.EntityList,
	tmpDirBase *string,
	store Storer,
	uploadHook HookRunner,
	done chan struct{},
	finished chan UpdateRequest,
) UploadSessioner {
	var s changesSession
	s.validateDebs = validateDebs
	s.keyRing = keyRing
	s.done = done
	s.finished = finished
	s.SessionId = uuid.New()
	s.changes = changes
	s.uploadHook = uploadHook
	s.store = store
	s.dir = *tmpDirBase + "/" + s.SessionId
	s.store = store

	os.Mkdir(s.dir, os.FileMode(0755))

	s.incoming = make(chan addItemMsg)
	s.close = make(chan closeMsg)
	s.getstatus = make(chan getStatusMsg)

	go s.handler()

	return &s
}

type closeMsg struct{}

type addItemMsg struct {
	file *UploadItem
	resp chan AptServerResponder
}

type getStatusMsg struct {
	resp chan AptServerResponder
}

// All item additions to this session are
// serialized through this routine
func (s *changesSession) handler() {
	defer func() {
		err := os.RemoveAll(s.dir)
		if err != nil {
			log.Println(err)
		}
		s.store.GarbageCollect()
		var msg struct{}
		s.done <- msg
	}()
	for {
		select {
		case <-s.close:
			{
				var msg struct{}
				s.done <- msg
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

				// We're done, lets call out to the server to update
				// with the contents of this session

				updater := make(chan AptServerResponder)
				s.finished <- UpdateRequest{
					session: s,
					resp:    updater,
				}

				updateresp := <-updater
				msg.resp <- updateresp

				// Need to do the update and return the response
				return
			}
		}
	}
}

func (s *changesSession) SessionID() string {
	return s.SessionId
}

func (s *changesSession) Directory() string {
	return s.dir
}

func (s *changesSession) Items() map[string]*UploadItem {
	return s.changes.Files
}

func (s *changesSession) Close() {
	s.close <- closeMsg{}
}

func (s *changesSession) DoneChan() chan struct{} {
	return s.done
}

func (s *changesSession) Status() AptServerResponder {
	done := make(chan AptServerResponder)
	go func() {
		s.getstatus <- getStatusMsg{
			resp: done,
		}
	}()
	resp := <-done
	return resp
}

func (s *changesSession) AddItem(upload *UploadItem) AptServerResponder {
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

func (s *changesSession) doAddItem(upload *UploadItem) (err error) {
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
	storeFilename := s.dir + "/" + upload.Filename
	defer func() {
		if err != nil {
			log.Println(err)
		}
		s.store.GarbageCollect()
	}()

	blob, err := s.store.Store()
	if err != nil {
		return errors.New("Upload to store failed: " + err.Error())
	}

	_, err = io.Copy(blob, tee)
	if err != nil {
		return errors.New("Upload to store failed: " + err.Error())
	}

	err = blob.Close()
	if err != nil {
		return errors.New("Upload to store failed: " + err.Error())
	}

	id, err := blob.Identity()
	if err != nil {
		return errors.New("Retrieving upload blob id failed: " + err.Error())
	}
	log.Println("ID: " + id.String())

	md5 := hex.EncodeToString(md5er.Sum(nil))
	sha1 := hex.EncodeToString(sha1er.Sum(nil))
	sha256 := hex.EncodeToString(sha256er.Sum(nil))

	if expectedFile.Md5 != md5 ||
		expectedFile.Sha1 != sha1 ||
		expectedFile.Sha256 != sha256 {
		return errors.New("Uploaded file hashes do not match")
	}

	if strings.HasSuffix(upload.Filename, ".deb") {
		// We should verify the signature
		f, _ := s.store.Open(id)
		if s.validateDebs {
			pkg := NewDebPackage(f, s.keyRing)

			signed, _ := pkg.IsSigned()
			validated, _ := pkg.IsValidated()

			if signed && validated {
				signedBy, _ := pkg.SignedBy()
				i := 0
				for k, _ := range signedBy.Identities {
					expectedFile.SignedBy[i] = k
					i++
				}
			} else {
				return errors.New("Package could not be validated")
			}
		}
	}

	err = s.store.Link(id, storeFilename)
	if err != nil {
		return errors.New("Error linking store file: " + err.Error())
	}

	expectedFile.UploadHookResult = s.uploadHook.Run(storeFilename)
	if expectedFile.UploadHookResult.err != nil {
		os.Remove(storeFilename)
		err = errors.New("Upload " + expectedFile.UploadHookResult.Error())
	}

	expectedFile.Uploaded = true

	return
}

func (s *changesSession) MarshalJSON() (j []byte, err error) {
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

// An UploadSession for uploading lone deb packages
type loneDebSession struct {
	SessionId    string             // Name of the session
	validateDebs bool               // Validate uploaded. deb files
	keyRing      openpgp.EntityList // Keyring for validation
	dir          string             // Temporary directory for storage
	store        Storer             // Blob store for file storage
	requireSig   bool               // Check debian package signatures
	uploadHook   HookRunner         // A hook to run after a successful upload
	file         *UploadItem

	// output session
	finished chan UpdateRequest // A channel to anounce completion and trigger a repo update
}

func NewLoneDebSession(
	validateDebs bool,
	keyRing openpgp.EntityList,
	tmpDirBase *string,
	store Storer,
	uploadHook HookRunner,
	finished chan UpdateRequest,
) UploadSessioner {
	var s loneDebSession
	s.validateDebs = validateDebs
	s.keyRing = keyRing
	s.finished = finished
	s.SessionId = uuid.New()
	s.uploadHook = uploadHook
	s.dir = *tmpDirBase + "/" + s.SessionId
	s.store = store

	os.Mkdir(s.dir, os.FileMode(0755))

	return &s
}

func (s *loneDebSession) SessionID() string {
	return s.SessionId
}

func (s *loneDebSession) Directory() string {
	return s.dir
}

// Should never get called
func (s *loneDebSession) Close() {
	return
}

// Should never get called
func (s *loneDebSession) DoneChan() chan struct{} {
	dummy := make(chan struct{})
	return dummy
}

func (s *loneDebSession) Status() AptServerResponder {
	return AptServerMessage(http.StatusBadRequest, "Not done yet")
}

func (s *loneDebSession) AddItem(upload *UploadItem) (resp AptServerResponder) {
	defer os.RemoveAll(s.dir)
	storeFilename := s.dir + "/" + upload.Filename

	md5er := md5.New()
	sha1er := sha1.New()
	sha256er := sha256.New()
	hasher := io.MultiWriter(md5er, sha1er, sha256er)
	tee := io.TeeReader(upload.data, hasher)

	blob, err := s.store.Store()
	if err != nil {
		resp = AptServerMessage(
			http.StatusBadRequest,
			"Package upload to store failed, "+err.Error(),
		)
		return
	}

	upload.Size, err = io.Copy(blob, tee)
	if err != nil {
		resp = AptServerMessage(
			http.StatusBadRequest,
			"Package upload to store failed, "+err.Error(),
		)
		return
	}

	err = blob.Close()
	if err != nil {
		resp = AptServerMessage(
			http.StatusBadRequest,
			"Package upload to store failed, "+err.Error(),
		)
		return
	}

	id, err := blob.Identity()
	if err != nil {
		resp = AptServerMessage(
			http.StatusBadRequest,
			"Retrieving upload blob id failed: "+err.Error(),
		)
		return
	}

	log.Println("ID: " + id.String())

	if err != nil {
		resp = AptServerMessage(
			http.StatusBadRequest,
			"Package upload failed, "+err.Error(),
		)
		return
	}

	pkgfile, err := s.store.Open(id)
	if err != nil {
		resp = AptServerMessage(
			http.StatusBadRequest,
			"Package upload failed, "+err.Error(),
		)
		return
	}

	pkg := NewDebPackage(pkgfile, s.keyRing)

	err = pkg.Parse()
	if err != nil {
		resp = AptServerMessage(
			http.StatusBadRequest,
			"Package file not valid, "+err.Error(),
		)
		return
	}

	signers := make([]string, 1)
	if s.validateDebs {
		signed, _ := pkg.IsSigned()
		validated, _ := pkg.IsValidated()

		if signed && validated {
			signedBy, _ := pkg.SignedBy()
			i := 0
			for k, _ := range signedBy.Identities {
				signers[i] = k
				i++
			}
		} else {
			resp = AptServerMessage(
				http.StatusBadRequest,
				"Package could not be validated",
			)
			return
		}
	}

	err = s.store.Link(id, storeFilename)
	if err != nil {
		resp = AptServerMessage(
			http.StatusBadRequest,
			"Package file not valid, "+err.Error(),
		)
		return
	}

	hookResult := s.uploadHook.Run(storeFilename)
	if hookResult.err != nil {
		resp = AptServerMessage(
			http.StatusBadRequest,
			"Upload "+hookResult.Error(),
		)
		return
	}

	s.file = upload
	s.file.UploadHookResult = hookResult
	s.file.Uploaded = true
	s.file.Md5 = hex.EncodeToString(md5er.Sum(nil))
	s.file.Sha1 = hex.EncodeToString(sha1er.Sum(nil))
	s.file.Sha256 = hex.EncodeToString(sha256er.Sum(nil))
	s.file.SignedBy = signers

	// We're done, lets call out to the server to update
	// with the contents of this session
	updater := make(chan AptServerResponder)
	s.finished <- UpdateRequest{
		session: s,
		resp:    updater,
	}

	updateresp := <-updater

	return updateresp
}

func (s *loneDebSession) Items() map[string]*UploadItem {
	result := make(map[string]*UploadItem)
	result[s.file.Filename] = s.file
	return result
}

func (s *loneDebSession) MarshalJSON() (j []byte, err error) {
	resp := struct {
		SessionId string
		File      UploadItem
	}{
		s.SessionId,
		*s.file,
	}
	j, err = json.Marshal(resp)
	return
}
