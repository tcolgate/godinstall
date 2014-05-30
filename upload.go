package main

import (
	"os"
	"log"
	"time"
)
	//"code.google.com/p/go.crypto/openpgp"
	//"code.google.com/p/go.crypto/openpgp/armor"
  //"github.com/stapelberg/godebiancontrol"

type uploadSession struct {
	SessionId string
  dir string
}

func (s *uploadSession) Close(){
  os.Remove(s.dir)
}

func (a *AptServer) NewUploadSession(sessionId string){
  var s uploadSession
  s.SessionId = sessionId
  s.dir = a.TmpDir + "/" + sessionId

  os.Mkdir(s.dir, os.FileMode(0755))

  a.sessMap.Set(sessionId, &s)
  go pathHandle(a.sessMap, sessionId, a.TTL)
}

func pathHandle(sessMap *SafeMap, s string, timeout time.Duration) {
	time.Sleep(timeout)
  c := sessMap.Get(s)
  if c != nil {
    switch sess := c.(type){
      case *uploadSession:
        log.Println("Close session")
    	  sess.Close()
      default:
        log.Println("Shouldn't get here")
    }
  }else{
    log.Println("Didn't find session")
  }
}
