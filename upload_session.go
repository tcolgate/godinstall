package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"code.google.com/p/go-uuid/uuid"
	"code.google.com/p/go.crypto/openpgp"
)

// UploadSessioner defines an interface for managing an session used for
// uploading a set of files for integration with the repository, and communicating
// the status and completion of the session
type UploadSessioner interface {
	ID() string                              // return the UUID for this session
	BranchName() string                      // return the release this update is for
	Directory() string                       // return the base directory for the verified uploaded files
	Items() map[string]*ChangesItem          // return the changes file for this session
	AddItem(*ChangesItem) AptServerResponder // Add the given item to this session
	Close()                                  // Close, and clear up, any remaining files
	Done() chan struct{}                     // This returns a channel that anounces copletion
	Status() AptServerResponder              // Return the status of this session
	json.Marshaler                           // All session implementations should serialize to JSON
}

// Base session information
type uploadSession struct {
	SessionID    string             // Name of the session
	branchName   string             // The release this is meant for
	validateDebs bool               // Validate uploaded. deb files
	keyRing      openpgp.EntityList // Keyring for validation
	dir          string             // Temporary directory for storage
	uploadHook   HookRunner         // A hook to run after a successful upload
	requireSig   bool               // Check debian package signatures
	store        ArchiveStorer      // Blob store to keep files in
	finished     chan UpdateRequest // A channel to anounce completion and trigger a repo update
	changes      *ChangesFile       // The changes file for this session
	ttl          time.Duration      // How long should this session stick around for

	// Channels for requests
	// TODO revisit this
	incoming  chan addItemMsg   // New item upload requests
	close     chan closeMsg     // A channel for close messages
	getstatus chan getStatusMsg // A channel for responding to status requests

	// output session
	// TODO revisit this
	done chan struct{} // A channel to be informed of closure on
}

func (s *uploadSession) ID() string {
	return s.SessionID
}

func (s *uploadSession) Directory() string {
	return s.dir
}

func (s *uploadSession) BranchName() string {
	return s.branchName
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

// NewUploadSession creates a session for uploading using a changes
// file to describe the set of files to be uploaded
func NewUploadSession(
	branchName string,
	changes *ChangesFile,
	validateDebs bool,
	keyRing openpgp.EntityList,
	tmpDirBase *string,
	store ArchiveStorer,
	uploadHook HookRunner,
	finished chan UpdateRequest,
	TTL time.Duration,
) UploadSessioner {
	var s uploadSession
	s.branchName = branchName
	s.validateDebs = validateDebs
	s.keyRing = keyRing
	s.done = make(chan struct{})
	s.finished = finished
	s.SessionID = uuid.New()
	s.changes = changes
	s.uploadHook = uploadHook
	s.store = store
	s.dir = *tmpDirBase + "/" + s.SessionID
	s.store = store
	s.ttl = TTL

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
func (s *uploadSession) handler() {
	s.store.DisableGarbageCollector()

	defer func() {
		err := os.RemoveAll(s.dir)
		if err != nil {
			log.Println(err)
		}
		s.store.EnableGarbageCollector()
		close(s.done)
	}()

	timeout := time.After(s.ttl)
	for {
		select {
		case <-timeout:
			{
				return
			}
		case <-s.close:
			{
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

				c := make(chan AptServerResponder)
				s.finished <- UpdateRequest{
					session: s,
					resp:    c,
				}

				updateresp := <-c
				msg.resp <- updateresp

				// Need to do the update and return the response
				return
			}
		}
	}
}

func (s *uploadSession) Close() {
	s.close <- closeMsg{}
}

func (s *uploadSession) Done() chan struct{} {
	return s.done
}

func (s *uploadSession) Status() AptServerResponder {
	c := make(chan AptServerResponder)
	go func() {
		s.getstatus <- getStatusMsg{
			resp: c,
		}
	}()
	resp := <-c
	return resp
}

func (s *uploadSession) AddItem(upload *ChangesItem) AptServerResponder {
	c := make(chan AptServerResponder)
	go func() {
		s.incoming <- addItemMsg{
			file: upload,
			resp: c,
		}
	}()
	resp := <-c
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

	storeFilename := s.dir + "/" + upload.Filename
	blob, err := s.store.Store()
	hasher := MakeWriteHasher(blob)
	if err != nil {
		return errors.New("Upload to store failed: " + err.Error())
	}

	_, err = io.Copy(hasher, upload.data)
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

	if s.changes.loneDeb {
		expectedFile.Md5 = md5
		expectedFile.Sha1 = sha1
		expectedFile.Sha256 = sha256
	}

	if !s.changes.loneDeb && (expectedFile.Md5 != md5 ||
		expectedFile.Sha1 != sha1 ||
		expectedFile.Sha256 != sha256) {
		err = errors.New("Uploaded file hashes do not match")
		return err
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
				err = errors.New("Package could not be validated")
				return err
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
