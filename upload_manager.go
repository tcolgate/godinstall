package main

import (
	"mime/multipart"
	"net/http"
	"time"

	"code.google.com/p/go.crypto/openpgp"
)

// Manage upload sessions
type UploadSessionManager interface {
	AddUploadSession(*DebChanges) (string, error)
	UploadSessionStatus(string) AptServerResponder
	UploadSessionAddItems(string, []*multipart.FileHeader) AptServerResponder
}

type uploadSessionManager struct {
	TTL             time.Duration
	TmpDir          *string
	UploadHook      HookRunner
	ValidateChanges bool
	ValidateDebs    bool
	PubRing         openpgp.KeyRing

	finished chan UploadSessioner
	sessMap  *SafeMap
}

func NewUploadSessionManager(
	TTL time.Duration,
	tmpDir *string,
	uploadHook HookRunner,
	validateChanges bool,
	validateDebs bool,
	pubRing openpgp.KeyRing,
) UploadSessionManager {
	return &uploadSessionManager{
		TTL:             TTL,
		TmpDir:          tmpDir,
		UploadHook:      uploadHook,
		ValidateChanges: validateChanges,
		ValidateDebs:    validateDebs,
		PubRing:         pubRing,

		sessMap: NewSafeMap(),
	}
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

func (usm *uploadSessionManager) AddUploadSession(changes *DebChanges) (string, error) {
	done := make(chan struct{})

	s := NewUploadSession(
		changes,
		done,
	)

	usm.sessMap.Set(s.SessionID(), s)
	go usm.handler(s)

	return s.SessionID(), nil
}

func (usm *uploadSessionManager) UploadSessionStatus(s string) (resp AptServerResponder) {
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

func (usm *uploadSessionManager) UploadSessionAddItems(
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
			resp = session.AddItem(&ChangesItem{
				Filename: f.Filename,
				data:     reader,
			})
		}
	}

	return
}

// Go routine for handling upload sessions
func (usm *uploadSessionManager) handler(s UploadSessioner) {
	defer func() {
		usm.sessMap.Set(s.SessionID(), nil)
	}()

	for {
		select {
		case <-s.DoneChan():
			{
				return
			}
		case <-time.After(usm.TTL):
			{
				s.Close()
			}
		}
	}
}
