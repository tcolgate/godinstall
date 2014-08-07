package main

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	"code.google.com/p/go.crypto/openpgp"
)

// UploadSessionManager is responsible for maintaing a set of upload
// session  It creates sessions, times them out, amd acts as a request
// muxer to pass requests on to invidiuvidual managers
type UploadSessionManager interface {
	AddDeb(*multipart.FileHeader) AptServerResponder
	AddChangesSession(io.Reader) (string, error)
	Status(string) AptServerResponder
	AddItems(string, []*multipart.FileHeader) AptServerResponder
}

// uploadSessionManager is a concreate implmentation of the UploadSessionManager
type uploadSessionManager struct {
	TTL                       time.Duration
	TmpDir                    *string
	UploadHook                HookRunner
	ValidateChanges           bool
	ValidateChangesSufficient bool
	ValidateDebs              bool
	PubRing                   openpgp.EntityList

	finished chan UpdateRequest
	sessMap  *SafeMap
}

func NewUploadSessionManager(
	TTL time.Duration,
	tmpDir *string,
	uploadHook HookRunner,
	validateChanges bool,
	validateChangesSufficient bool,
	validateDebs bool,
	pubRing openpgp.EntityList,
	finished chan UpdateRequest,
) UploadSessionManager {
	return &uploadSessionManager{
		TTL:                       TTL,
		TmpDir:                    tmpDir,
		UploadHook:                uploadHook,
		ValidateChanges:           validateChanges,
		ValidateChangesSufficient: validateChangesSufficient,
		ValidateDebs:              validateDebs,
		PubRing:                   pubRing,

		finished: finished,
		sessMap:  NewSafeMap(),
	}
}

// This retrieves a given upload session by the session's id
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

// Add a Deb, this probably shouldn't be here, but we don't
// need an upladSession, as they are focused too much on
// changes files
func (usm *uploadSessionManager) AddDeb(upload *multipart.FileHeader) (resp AptServerResponder) {
	s := NewLoneDebSession(
		usm.ValidateDebs,
		usm.PubRing,
		usm.TmpDir,
		usm.UploadHook,
		usm.finished,
	)

	reader, _ := upload.Open()
	resp = s.AddItem(&UploadItem{
		Filename: upload.Filename,
		data:     reader,
	})

	return
}

// Add a new upload session based on the details from the passed
// debian changes file.
func (usm *uploadSessionManager) AddChangesSession(changesReader io.Reader) (string, error) {
	var err error

	changes, err := ParseDebianChanges(changesReader, usm.PubRing)
	if err != nil {
		return "", err
	}

	if usm.ValidateChanges && !changes.signed {
		err = errors.New("Changes file was not signed")
		return "", err
	}

	if usm.ValidateChanges && !changes.validated {
		err = errors.New("Changes file could not be validated")
		return "", err
	}

	// Should we check signatures on individual debs?
	var validateDebSign bool
	if usm.ValidateChanges && changes.validated && usm.ValidateChangesSufficient {
		validateDebSign = false
	} else {
		validateDebSign = usm.ValidateDebs
	}

	s := NewChangesSession(
		changes,
		validateDebSign,
		usm.PubRing,
		usm.TmpDir,
		usm.UploadHook,
		make(chan struct{}),
		usm.finished,
	)

	usm.sessMap.Set(s.SessionID(), s)
	go usm.handler(s)

	return s.SessionID(), nil
}

// This retrieves the status of a given session as a
// HTTP response.
// TODO Should probably refactor this to just return the
// status and and error and consutrct the response elswhere
func (usm *uploadSessionManager) Status(s string) (resp AptServerResponder) {
	session, ok := usm.GetSession(s)

	if !ok {
		resp = AptServerMessage(
			http.StatusNotFound,
			"Unknown Session",
		)
	} else {
		resp = session.Status()
	}

	return
}

// This add am uploaded file containued in the mime section,
// to the session identified by the string
func (usm *uploadSessionManager) AddItems(
	s string,
	otherParts []*multipart.FileHeader) (resp AptServerResponder) {

	session, ok := usm.GetSession(s)

	if !ok {
		resp = AptServerMessage(
			http.StatusCreated,
			"Unknown Session",
		)
	}

	resp = AptServerMessage(
		http.StatusCreated,
		session,
	)

	if len(otherParts) > 0 {
		for _, f := range otherParts {
			reader, _ := f.Open()
			resp = session.AddItem(&UploadItem{
				Filename: f.Filename,
				data:     reader,
			})
		}
	}

	return
}

// This is used as a go routine manages the upload session and is used
// to serialize all actions on the given session.
// TODO need to revisit this
func (usm *uploadSessionManager) handler(s UploadSessioner) {
	defer func() {
		usm.sessMap.Set(s.SessionID(), nil)
	}()

	for {
		select {
		case <-s.DoneChan():
			{
				// The sesession has completed
				return
			}
		case <-time.After(usm.TTL):
			{
				// The sesession has timeout out,
				// tell it to close down
				s.Close()
			}
		}
	}
}
