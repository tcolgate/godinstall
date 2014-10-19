package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
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
	StoreID          StoreID
	Size             int64
	Md5              string
	Sha1             string
	Sha256           string
	Uploaded         bool
	SignedBy         []string
	UploadHookResult HookOutput

	data io.Reader
}

// UploadSessioner defines an interface for managing an session used for
// uploading a set of files for integration with the repository, and communicating
// the status and completion of the session
type UploadSessioner interface {
	ID() string                              // return the UUID for this session
	Directory() string                       // returnt he base directory for the verified uploaded files
	Items() map[string]*ChangesItem          // return the changes file for this session
	AddItem(*ChangesItem) AptServerResponder // Add the given item to this session
	Close()                                  // Close, and clear up, any remaining files
	DoneChan() chan struct{}                 // This returns a channel that anounces copletion
	Status() AptServerResponder              // Return the status of this session
	json.Marshaler                           // All session implementations should serialize to JSON
}

// Base session information
type uploadSession struct {
	SessionID    string             // Name of the session
	validateDebs bool               // Validate uploaded. deb files
	keyRing      openpgp.EntityList // Keyring for validation
	dir          string             // Temporary directory for storage
	uploadHook   HookRunner         // A hook to run after a successful upload
	requireSig   bool               // Check debian package signatures
	store        RepoStorer         // Blob store to keep files in
	finished     chan UpdateRequest // A channel to anounce completion and trigger a repo update
	changes      *ChangesFile       // The changes file for this session
}

func (s *uploadSession) ID() string {
	return s.SessionID
}

func (s *uploadSession) Directory() string {
	return s.dir
}

func (s *uploadSession) MarshalJSON() (j []byte, err error) {
	resp := struct {
		SessionID string
		Changes   ChangesFile
	}{
		s.SessionID,
		*s.changes,
	}
	j, err = json.Marshal(resp)
	return
}

func (s *uploadSession) Items() map[string]*ChangesItem {
	return s.changes.Files
}

// An UploadSession for uploading using a changes file
type changesSession struct {
	uploadSession

	// Channels for requests
	// TODO revisit this
	incoming  chan addItemMsg   // New item upload requests
	close     chan closeMsg     // A channel for close messages
	getstatus chan getStatusMsg // A channel for responding to status requests

	// output session
	// TODO revisit this
	done chan struct{} // A channel to be informed of closure on
}

// NewChangesSession creates a session for uploading using a changes
// file to describe the set of files to be uploaded
func NewChangesSession(
	changes *ChangesFile,
	validateDebs bool,
	keyRing openpgp.EntityList,
	tmpDirBase *string,
	store RepoStorer,
	uploadHook HookRunner,
	done chan struct{},
	finished chan UpdateRequest,
) UploadSessioner {
	var s changesSession
	s.validateDebs = validateDebs
	s.keyRing = keyRing
	s.done = done
	s.finished = finished
	s.SessionID = uuid.New()
	s.changes = changes
	s.uploadHook = uploadHook
	s.store = store
	s.dir = *tmpDirBase + "/" + s.SessionID
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
	file *ChangesItem
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

func (s *changesSession) AddItem(upload *ChangesItem) AptServerResponder {
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

func (s *changesSession) doAddItem(upload *ChangesItem) (err error) {
	// Check that there is an upload slot
	expectedFile, ok := s.changes.Files[upload.Filename]
	if !ok {
		return errors.New("File not listed in upload set")
	}

	if expectedFile.Uploaded {
		return errors.New("File already uploaded")
	}

	hasher := MakeWriteHasher(ioutil.Discard)
	tee := io.TeeReader(upload.data, hasher)
	storeFilename := s.dir + "/" + upload.Filename
	s.store.DisableGarbageCollector()

	defer func() {
		if err != nil {
			log.Println(err)
			s.store.EnableGarbageCollector()
		}
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
	expectedFile.StoreID = id

	md5 := hex.EncodeToString(hasher.MD5Sum())
	sha1 := hex.EncodeToString(hasher.SHA1Sum())
	sha256 := hex.EncodeToString(hasher.SHA256Sum())

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
				for k := range signedBy.Identities {
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

	expectedFile.Size, _ = s.store.Size(id)

	expectedFile.UploadHookResult = s.uploadHook.Run(storeFilename)
	if expectedFile.UploadHookResult.err != nil {
		os.Remove(storeFilename)
		err = errors.New("Upload " + expectedFile.UploadHookResult.Error())
	}

	expectedFile.Uploaded = true

	return
}

// An UploadSession for uploading lone deb packages
type loneDebSession struct {
	uploadSession
}

// NewLoneDebSession creates an upload session for use when a lone
// debian package is being uploaded, without a changes file
func NewLoneDebSession(
	validateDebs bool,
	keyRing openpgp.EntityList,
	tmpDirBase *string,
	store RepoStorer,
	uploadHook HookRunner,
	finished chan UpdateRequest,
) UploadSessioner {
	var s loneDebSession
	s.validateDebs = validateDebs
	s.keyRing = keyRing
	s.finished = finished
	s.SessionID = uuid.New()
	s.uploadHook = uploadHook
	s.dir = *tmpDirBase + "/" + s.SessionID
	s.store = store

	os.Mkdir(s.dir, os.FileMode(0755))

	return &s
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

func (s *loneDebSession) AddItem(upload *ChangesItem) (resp AptServerResponder) {
	defer os.RemoveAll(s.dir)
	storeFilename := s.dir + "/" + upload.Filename

	var changes ChangesFile
	var err error

	changes.Files = make(map[string]*ChangesItem)
	hasher := MakeWriteHasher(ioutil.Discard)
	tee := io.TeeReader(upload.data, hasher)

	s.store.DisableGarbageCollector()
	defer func() {
		if err != nil {
			log.Println(err)
			s.store.EnableGarbageCollector()
		}
	}()

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
		changes.signed, _ = pkg.IsSigned()
		changes.validated, _ = pkg.IsValidated()

		if changes.signed && changes.validated {
			signedBy, _ := pkg.SignedBy()
			i := 0
			for k := range signedBy.Identities {
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

	changes.Files[upload.Filename] = upload

	upload.UploadHookResult = hookResult
	upload.Uploaded = true
	upload.Md5 = hex.EncodeToString(hasher.MD5Sum())
	upload.Sha1 = hex.EncodeToString(hasher.SHA1Sum())
	upload.Sha256 = hex.EncodeToString(hasher.SHA256Sum())
	upload.SignedBy = signers
	upload.StoreID = id
	upload.Size, _ = s.store.Size(id)

	s.changes = &changes

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
