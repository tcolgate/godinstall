package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"code.google.com/p/go-uuid/uuid"
)

// UploadFile holds information about a file that has been uploaded. It
// includes the storage location. If it is a deb, or dsc, additional information
// about signatures is provided
type UploadFile struct {
	Name             string
	Received         bool
	Size             int64
	SignedBy         []string   `json:",omitempty"`
	UploadHookResult HookOutput `json:",omitempty"`

	pkg       DebPackageInfoer
	reader    io.Reader
	storeID   StoreID
	controlID StoreID
}

// UploadSession holds the information relating to an active upload session
type UploadSession struct {
	SessionID   string                 // Name of the session
	ReleaseName string                 // The release this is meant for
	Expecting   map[string]*UploadFile // The files we are expecting in this upload
	LoneDeb     bool                   // Is user attempting to upload a lone deb
	Complete    bool                   // The files we are expecting in this upload
	TTL         time.Duration          // How long should this session stick around for

	usm       *UploadSessionManager
	dir       string       // Temporary directory for storage
	changes   *ChangesFile // The changes file for this session
	changesID StoreID      // The raw changes file as uploaded

	// Channels for requests
	// TODO revisit this
	finished  chan UpdateRequest // A channel to anounce completion and trigger a repo update
	incoming  chan addItemMsg    // New item upload requests
	close     chan closeMsg      // A channel for close messages
	getstatus chan getStatusMsg  // A channel for responding to status requests

	// output session
	// TODO revisit this
	done chan struct{} // A channel to be informed of closure on
}

// ID returns the ID of this session
func (s *UploadSession) ID() string {
	return s.SessionID
}

// Directory  returns the working directory of this session
func (s *UploadSession) Directory() string {
	return s.dir
}

// MarshalJSON implements the json.Marshal interface
func (s *UploadSession) MarshalJSON() (j []byte, err error) {
	return json.Marshal(*s)
}

// NewUploadSession creates a session for uploading using a changes
// file to describe the set of files to be uploaded
func NewUploadSession(
	releaseName string,
	changesReader io.Reader,
	tmpDirBase *string,
	finished chan UpdateRequest,
	loneDeb bool,
	uploadSessionManager *UploadSessionManager,
) (UploadSession, error) {
	var s UploadSession
	s.SessionID = uuid.New()
	s.ReleaseName = releaseName
	s.usm = uploadSessionManager
	s.TTL = s.usm.TTL
	s.LoneDeb = loneDeb
	s.done = make(chan struct{})
	s.finished = finished
	s.dir = *tmpDirBase + "/" + s.SessionID
	s.Expecting = make(map[string]*UploadFile, 0)

	os.Mkdir(s.dir, os.FileMode(0755))

	s.incoming = make(chan addItemMsg)
	s.close = make(chan closeMsg)
	s.getstatus = make(chan getStatusMsg)

	if !s.LoneDeb {
		changesWriter, err := s.usm.Store.Store()
		if err != nil {
			return UploadSession{}, err
		}
		io.Copy(changesWriter, changesReader)
		changesWriter.Close()
		s.changesID, err = changesWriter.Identity()
		if err != nil {
			return UploadSession{}, err
		}

		changesReader, _ = s.usm.Store.Open(s.changesID)
		changes, err := ParseDebianChanges(changesReader, s.usm.PubRing)

		if s.usm.VerifyChanges && !changes.Control.Signed && !loneDeb {
			err = errors.New("Changes file was not signed")
			return UploadSession{}, err
		}

		if s.usm.VerifyChanges && !changes.Control.SignatureVerified && !loneDeb {
			err = errors.New("Changes file could not be verified")
			return UploadSession{}, err
		}

		s.changes = &changes
		if err != nil {
			return UploadSession{}, err
		}

		s.Expecting = map[string]*UploadFile{}
		for k := range changes.FileHashes {
			s.Expecting[k.Name] = &UploadFile{Name: k.Name}
		}
	}

	go s.handler()

	return s, nil
}

type closeMsg struct{}

type addItemMsg struct {
	file *UploadFile
	resp chan AptServerResponder
}

type getStatusMsg struct {
	resp chan AptServerResponder
}

// All item additions to this session are
// serialized through this routine
func (s *UploadSession) handler() {
	s.usm.Store.DisableGarbageCollector()

	defer func() {
		err := os.RemoveAll(s.dir)
		if err != nil {
			log.Println(err)
		}
		s.usm.Store.EnableGarbageCollector()
		close(s.done)
	}()

	timeout := time.After(s.TTL)
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
				err := s.doAddFile(msg.file)

				if err != nil {
					msg.resp <- AptServerMessage(http.StatusBadRequest, err.Error())
					break
				}

				complete := true
				for _, uf := range s.Expecting {
					if !uf.Received {
						complete = false
					}
				}

				if !complete {
					msg.resp <- AptServerMessage(http.StatusAccepted, s)
					break
				} else {
					s.Complete = true
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

// Close is used to indicate that a session is finished
func (s *UploadSession) Close() {
	s.close <- closeMsg{}
}

// Done returns a channels that can be used to be informed of
// the completiong of a session
func (s *UploadSession) Done() chan struct{} {
	return s.done
}

// Status returns a server response indivating the running state
// of the session
func (s *UploadSession) Status() AptServerResponder {
	c := make(chan AptServerResponder)
	s.getstatus <- getStatusMsg{
		resp: c,
	}
	return <-c
}

// AddFile adds an uploaded file to the given session, taking hashes,
// and placing it in the archive store.
func (s *UploadSession) AddFile(upload *UploadFile) AptServerResponder {
	c := make(chan AptServerResponder)
	s.incoming <- addItemMsg{
		file: upload,
		resp: c,
	}
	return <-c
}

func (s *UploadSession) doAddFile(upload *UploadFile) (err error) {
	var uf *UploadFile
	var expectedFileIdx ChangesFilesIndex
	var ok bool

	if !s.LoneDeb {
		uf, ok = s.Expecting[upload.Name]
		if !ok {
			return errors.New("File not listed in upload set")
		}
		if uf.Received {
			return errors.New("File already uploaded")
		}

		// Check that there is an upload slot
		ok = false
		for k := range s.changes.FileHashes {
			if upload.Name == k.Name {
				expectedFileIdx, ok = k, true
				break
			}
		}
		if !ok {
			return errors.New("File not listed in upload hashes")
		}
	}

	storeFilename := s.dir + "/" + upload.Name
	blob, err := s.usm.Store.Store()
	hasher := MakeWriteHasher(blob)
	if err != nil {
		return errors.New("Upload to store failed: " + err.Error())
	}

	_, err = io.Copy(hasher, upload.reader)
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

	md5 := hasher.MD5Sum()
	sha1 := hasher.SHA1Sum()
	sha256 := hasher.SHA256Sum()
	size, _ := s.usm.Store.Size(id)

	switch {
	case strings.HasSuffix(upload.Name, ".deb"):
		{
			f, _ := s.usm.Store.Open(id)
			defer f.Close()
			pkg := NewDebPackage(f, s.usm.PubRing)
			_, err = pkg.Name()
			if err != nil {
				return errors.New("upload deb failed,  " + err.Error())
			}
			f.Close()

			if s.LoneDeb {
				s.changes, err = LoneChanges(pkg, upload.Name, s.ReleaseName)
				if err != nil {
					return errors.New("Generating changes file failed,  " + err.Error())
				}
				changesWriter, err := s.usm.Store.Store()
				if err != nil {
					return errors.New("Generating changes file failed,  " + err.Error())
				}
				FormatChangesFile(changesWriter, s.changes)
				changesWriter.Close()
				s.changesID, err = changesWriter.Identity()
				if err != nil {
					return errors.New("Generating changes file failed,  " + err.Error())
				}

				newFile := *upload
				s.Expecting[upload.Name] = &newFile
				uf = s.Expecting[upload.Name]
			}

			uf.pkg = pkg

			// Store the control information
			ctrl, err := uf.pkg.Control()
			if err != nil {
				return errors.New("Retrieving control file failed, " + err.Error())
			}

			ctrl.Data[0].SetValue("Size", strconv.FormatInt(size, 10))
			ctrl.Data[0].SetValue("MD5sum", hex.EncodeToString(md5))
			ctrl.Data[0].SetValue("SHA1", hex.EncodeToString(sha1))
			ctrl.Data[0].SetValue("SHA256", hex.EncodeToString(sha256))

			uf.controlID, err = s.usm.Store.AddControlFile(*ctrl)
			if err != nil {
				return errors.New("Storing control file failed, " + err.Error())
			}

			// We should verify the signature
			if s.usm.VerifyDebs {
				signed, _ := uf.pkg.IsSigned()
				verified, _ := uf.pkg.IsVerified()

				if signed && verified {
					signedBy, _ := uf.pkg.SignedBy()
					for k := range signedBy.Identities {
						uf.SignedBy = append(uf.SignedBy, k)
					}
				} else {
					err = errors.New("Package could not be verified")
					return err
				}
			}
		}
	case strings.HasSuffix(upload.Name, ".dsc"):
		{
			f, _ := s.usm.Store.Open(id)
			defer f.Close()
			ctrl, err := ParseDebianControl(f, s.usm.PubRing)
			if err != nil {
				return errors.New("Parsing dsc failed, " + err.Error())
			}
			uf.controlID, err = s.usm.Store.AddControlFile(ctrl)
			if err != nil {
				return errors.New("Storing control file failed, " + err.Error())
			}
		}
	}

	if !s.LoneDeb && (bytes.Compare(s.changes.FileHashes[expectedFileIdx]["md5"], md5) != 0 ||
		bytes.Compare(s.changes.FileHashes[expectedFileIdx]["sha1"], sha1) != 0 ||
		bytes.Compare(s.changes.FileHashes[expectedFileIdx]["sha256"], sha256) != 0) {
		err = errors.New("Uploaded file hashes do not match")
		return err
	}

	err = s.usm.Store.Link(id, storeFilename)
	if err != nil {
		return errors.New("Error linking store file: " + err.Error())
	}

	uf.storeID = id
	uf.Size = size
	uf.UploadHookResult = s.usm.UploadHook.Run(storeFilename)
	if uf.UploadHookResult.err != nil {
		os.Remove(storeFilename)
		err = errors.New("Upload " + uf.UploadHookResult.Error())
	}

	uf.Received = true

	return
}
