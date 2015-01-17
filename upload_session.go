package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"strconv"
	"strings"

	"golang.org/x/net/context"

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
	SessionID         string                 // Name of the session
	ReleaseName       string                 // The release this is meant for
	Expecting         map[string]*UploadFile // The files we are expecting in this upload
	LoneDeb           bool                   // Is user attempting to upload a lone deb
	Complete          bool                   // The files we are expecting in this upload
	PreGenHookOutput  *HookOutput            `json:",omitempty"`
	PostGenHookOutput *HookOutput            `json:",omitempty"`

	usm       *UploadSessionManager
	release   *Release
	dir       string       // Temporary directory for storage
	changes   *ChangesFile // The changes file for this session
	changesID StoreID      // The raw changes file as uploaded
	err       error

	// Channels for requests
	// TODO revisit this
	incoming  chan addItemMsg   // New item upload requests
	getstatus chan getStatusMsg // A channel for responding to status requests
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
	ctx context.Context,
	cancel context.CancelFunc,
	rel *Release,
	loneDeb bool,
	changesReader io.ReadCloser,
	tmpDirBase *string,
	uploadSessionManager *UploadSessionManager,
) (UploadSession, error) {
	var s UploadSession
	s.SessionID = uuid.New()
	s.ReleaseName = rel.Suite
	s.release = rel
	s.usm = uploadSessionManager
	s.dir = *tmpDirBase + "/" + s.SessionID
	s.Expecting = make(map[string]*UploadFile, 0)
	s.LoneDeb = loneDeb

	os.Mkdir(s.dir, os.FileMode(0755))

	s.incoming = make(chan addItemMsg)
	s.getstatus = make(chan getStatusMsg)

	if !s.LoneDeb {
		var err error
		s.changesID, err = s.usm.Store.CopyToStore(changesReader)
		if err != nil {
			return UploadSession{}, err
		}

		kr, err := s.release.PubRing()
		if err != nil {
			return UploadSession{}, errors.New("Reading pubring failed failed,  " + err.Error())
		}

		changesReader, _ = s.usm.Store.Open(s.changesID)
		changes, err := ParseDebianChanges(changesReader, kr)

		if rel.Config().VerifyChanges && !changes.Control.Signed {
			err = errors.New("Changes file was not signed")
			return UploadSession{}, err
		}

		if rel.Config().VerifyChanges && !changes.Control.SignatureVerified {
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

	go s.handler(ctx, cancel)

	return s, nil
}

type addItemMsg struct {
	file *UploadFile
	resp chan UploadSession
}

type getStatusMsg struct {
	resp chan UploadSession
}

// All item additions to this session are
// serialized through this routine
func (s *UploadSession) handler(ctx context.Context, cancel context.CancelFunc) {
	s.usm.Store.DisableGarbageCollector()

	defer func() {
		err := os.RemoveAll(s.dir)
		if err != nil {
			log.Println(err)
		}
		s.usm.Store.EnableGarbageCollector()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-s.getstatus:
			{
				msg.resp <- *s
			}
		case msg := <-s.incoming:
			{
				if s.err = s.doAddFile(msg.file); s.err != nil {
					msg.resp <- *s
					break
				}

				complete := true
				for _, uf := range s.Expecting {
					if !uf.Received {
						complete = false
					}
				}

				if !complete {
					msg.resp <- *s
					break
				} else {
					s.Complete = true
				}

				s.usm.MergeSession(s)
				msg.resp <- *s
			}
		}
	}
}

// Status returns a server response indivating the running state
// of the session
func (s *UploadSession) Status() UploadSession {
	c := make(chan UploadSession)
	s.getstatus <- getStatusMsg{
		resp: c,
	}
	return <-c
}

// AddFile adds an uploaded file to the given session, taking hashes,
// and placing it in the archive store.
func (s *UploadSession) AddFile(name string, r io.Reader) UploadSession {
	u := &UploadFile{
		Name:   name,
		reader: r,
	}
	c := make(chan UploadSession)
	s.incoming <- addItemMsg{
		file: u,
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
	if err != nil {
		return errors.New("Upload to store failed: " + err.Error())
	}
	hasher := MakeWriteHasher(blob)

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
			kr, err := s.release.PubRing()
			if err != nil {
				return errors.New("Reading pubring failed failed,  " + err.Error())
			}
			pkg := NewDebPackage(f, kr)
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
			if s.release.Config().VerifyDebs &&
				(!s.release.Config().VerifyChangesSufficient || s.LoneDeb) {
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
			kr, err := s.release.PubRing()
			if err != nil {
				return errors.New("Reading pubring failed failed,  " + err.Error())
			}
			ctrl, err := ParseDebianControl(f, kr)
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

	return nil
}

// AddFile adds an uploaded file to the given session, taking hashes,
// and placing it in the archive store.
func (s *UploadSession) Err() error {
	c := make(chan UploadSession)
	s.getstatus <- getStatusMsg{
		resp: c,
	}
	status := <-c
	return status.err
}
