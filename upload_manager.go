package main

import (
	"encoding/json"
	"io"
	"net/http"
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
	sessMap  *SafeMap
}

// CompletedUpload describes a finished session, the details of the session,
// and the output of any hooks
type CompletedUpload struct {
	*UploadSession
	PreGenHookOutput  HookOutput
	PostGenHookOutput HookOutput
}

// MarshalJSON implements the json.Marshaler interface to allow
// presentation of a completed session to the user
func (s CompletedUpload) MarshalJSON() (j []byte, err error) {
	resp := struct {
		UploadSession
		PreGenHookOutput  HookOutput
		PostGenHookOutput HookOutput
	}{
		*s.UploadSession,
		s.PreGenHookOutput,
		s.PostGenHookOutput,
	}
	j, err = json.Marshal(resp)
	return
}

// Updater ensures that updates to the repository are serialized.
// it reads from a channel of messages, responds to clients, and
// instigates the actual regernation of the repository
func updater() {
	for {
		select {
		case msg := <-state.UpdateChannel:
			{
				var err error
				respStatus := http.StatusOK
				var respObj interface{}

				session := msg.session
				completedsession := CompletedUpload{UploadSession: session}

				state.Lock.WriteLock()

				hookResult := cfg.PreGenHook.Run(session.Directory())
				if hookResult.err != nil {
					respStatus = http.StatusBadRequest
					respObj = "Pre gen hook failed " + hookResult.Error()
				} else {
					completedsession.PreGenHookOutput = hookResult
				}

				respStatus, respObj, err = state.Archive.AddUpload(session)
				if err == nil {
					hookResult := cfg.PostGenHook.Run(session.ID())
					completedsession.PostGenHookOutput = hookResult
				}

				state.Lock.WriteUnLock()

				if respStatus == http.StatusOK {
					respObj = completedsession
				}

				msg.resp <- newAppResponse(respStatus, respObj)
			}
		}
	}
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
	finished chan UpdateRequest,
) *UploadSessionManager {

	res := &UploadSessionManager{
		TTL:        TTL,
		TmpDir:     tmpDir,
		Store:      store,
		UploadHook: uploadHook,

		finished: finished,
		sessMap:  NewSafeMap(),
	}

	go updater()

	return res
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

	ctx, cancel := context.WithTimeout(context.Background(), usm.TTL)
	s, err := NewUploadSession(
		ctx,
		cancel,
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

	id := s.ID()
	usm.sessMap.Set(id, s)

	go usm.cleanup(ctx, id)

	return id, nil
}

// This is used as a go routine manages the upload session and is used
// to serialize all actions on the given session.
// TODO need to revisit this
func (usm *UploadSessionManager) cleanup(ctx context.Context, id string) {
	select {
	case <-ctx.Done():
		{
			// The sesession has completed
			usm.sessMap.Set(id, nil)
		}
	}
}
