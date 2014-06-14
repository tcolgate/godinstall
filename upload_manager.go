package main

import "time"

// Manage upload sessions
type UploadSessionManager interface {
	GetSession(string) (UploadSessioner, bool)
	AddUploadSession(*DebChanges) (UploadSessioner, error)
}

type uploadSessionManager struct {
	sessMap   *SafeMap
	aptServer AptServer
	finished  chan UploadSessioner
}

func NewUploadSessionManager(a AptServer) UploadSessionManager {
	usm := uploadSessionManager{}
	usm.sessMap = NewSafeMap()
	usm.aptServer = a

	return &usm
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

func (usm *uploadSessionManager) AddUploadSession(changes *DebChanges) (UploadSessioner, error) {
	s := NewUploadSession(
		changes,
		usm.aptServer.PubRing,
		usm.aptServer.PostUploadHook,
		usm.aptServer.TmpDir,
	)

	go usm.handler(s)

	return s, nil
}

// Go routine for handling upload sessions
func (usm *uploadSessionManager) handler(s UploadSessioner) {
	usm.sessMap.Set(s.SessionID(), s)

	defer func() {
		usm.sessMap.Set(s.SessionID(), nil)
		s.Close()
	}()

	for {
		select {
		case <-time.After(usm.aptServer.TTL):
			{
				return
			}
		}
	}
}
