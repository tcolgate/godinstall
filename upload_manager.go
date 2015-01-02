package main

import (
	"io"
	"time"
)

// UploadSessionManager is a concreate implmentation of the UploadSessionManager
type UploadSessionManager struct {
	TTL        time.Duration
	TmpDir     *string
	Store      ArchiveStorer
	UploadHook HookRunner

	finished chan UpdateRequest
	sessMap  *SafeMap
}

// NewUploadSessionManager creates a session manager which maintains a set of
// on-going upload sessions, controlling thier permitted life time, temporary
// storage location, and how the contents should be verified
func NewUploadSessionManager(
	TTL time.Duration,
	tmpDir *string,
	store ArchiveStorer,
	uploadHook HookRunner,
	finished chan UpdateRequest,
) *UploadSessionManager {
	return &UploadSessionManager{
		TTL:        TTL,
		TmpDir:     tmpDir,
		Store:      store,
		UploadHook: uploadHook,

		finished: finished,
		sessMap:  NewSafeMap(),
	}
}

// GetSession retrieves a given upload session by the session's id
func (usm *UploadSessionManager) GetSession(sid string) (UploadSession, bool) {
	val := usm.sessMap.Get(sid)
	if val == nil {
		return UploadSession{}, false
	}

	switch t := val.(type) {
	default:
		{
			return UploadSession{}, false
		}
	case UploadSession:
		{
			return UploadSession(t), true
		}
	}
}

// NewSession adds a new upload session based on the details from the passed
// debian changes file.
func (usm *UploadSessionManager) NewSession(rel *Release, changesReader io.ReadCloser, loneDeb bool) (string, error) {
	var err error

	s, err := NewUploadSession(
		rel,
		loneDeb,
		changesReader,
		usm.TmpDir,
		usm.finished,
		usm,
	)

	if err != nil {
		return "", err
	}

	usm.sessMap.Set(s.ID(), s)
	go usm.cleanup(s)

	return s.ID(), nil
}

// This is used as a go routine manages the upload session and is used
// to serialize all actions on the given session.
// TODO need to revisit this
func (usm *UploadSessionManager) cleanup(s UploadSession) {
	select {
	case <-s.Done():
		{
			// The sesession has completed
			usm.sessMap.Set(s.ID(), nil)
		}
	}
}
