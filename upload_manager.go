package main

import (
	"io"
	"time"

	"golang.org/x/net/context"
)

// UploadSessionManager is a concreate implmentation of the UploadSessionManager
type UploadSessionManager struct {
	TTL        time.Duration
	TmpDir     *string
	Store      ArchiveStorer
	UploadHook HookRunner

	finished chan UpdateRequest
	sessMap  *safeMap
}

// UpdateRequest contains the information needed to
// request an update, only regeneration is supported
// at present
type UpdateRequest struct {
	resp    chan *appError
	session *UploadSession
}

// NewUploadSessionManager creates a session manager which maintains a set of
// on-going upload sessions, controlling thier permitted life time, temporary
// storage location, and how the contents should be verified
func NewUploadSessionManager(
	TTL time.Duration,
	tmpDir *string,
	store ArchiveStorer,
	uploadHook HookRunner,
) *UploadSessionManager {

	finished := make(chan UpdateRequest)

	res := &UploadSessionManager{
		TTL:        TTL,
		TmpDir:     tmpDir,
		Store:      store,
		UploadHook: uploadHook,

		finished: finished,
		sessMap:  NewSafeMap(),
	}

	go res.updater()

	return res
}

// GetSession retrieves a given upload session by the session's id
func (usm *UploadSessionManager) GetSession(sid string) (s UploadSession, ok bool) {
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

	ctx, _ := context.WithTimeout(context.Background(), usm.TTL)
	s, err := NewUploadSession(
		ctx,
		rel,
		loneDeb,
		changesReader,
		usm.TmpDir,
		usm,
	)

	if err != nil {
		return "", err
	}

	id := s.ID()
	usm.sessMap.Set(id, s)

	go func() {
		<-ctx.Done()
		usm.sessMap.Set(id, nil)
	}()

	return id, nil
}

// mergeSession Merges the provided upload session into the
// release it was uploaded to.
func (usm *UploadSessionManager) mergeSession(s *UploadSession) *appError {
	c := make(chan *appError)
	usm.finished <- UpdateRequest{
		session: s,
		resp:    c,
	}

	return <-c
}

// Updater ensures that updates to the repository are serialized.
// it reads from a channel of messages, responds to clients, and
// instigates the actual regernation of the repository
func (usm *UploadSessionManager) updater() {
	for {
		select {
		case msg := <-usm.finished:
			{
				var apperr *appError

				s := msg.session

				state.Lock.WriteLock()

				hookResult := cfg.PreGenHook.Run(s.Directory())
				s.PreGenHookOutput = &hookResult

				if err := state.Archive.AddUpload(s); err == nil {
					hookResult := cfg.PostGenHook.Run(s.ID())
					s.PostGenHookOutput = &hookResult
				} else {
					apperr = &appError{Error: err}
				}
				state.Lock.WriteUnLock()

				msg.resp <- apperr
			}
		}
	}
}
