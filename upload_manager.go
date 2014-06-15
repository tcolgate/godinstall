package main

import (
	"mime/multipart"
	"net/http"
	"time"
)

// Manage upload sessions
type UploadSessionManager interface {
	AddUploadSession(*DebChanges) (string, error)
	UploadSessionStatus(string) AptServerResponder
	UploadSessionAddItems(string, []*multipart.FileHeader) AptServerResponder
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

func (usm *uploadSessionManager) AddUploadSession(changes *DebChanges) (string, error) {
	done := make(chan struct{})

	s := NewUploadSession(
		&usm.aptServer,
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
		case <-time.After(usm.aptServer.TTL):
			{
				s.Close()
			}
		}
	}
}
