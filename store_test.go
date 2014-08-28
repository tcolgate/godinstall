package main

import (
	"io/ioutil"
	"os"
	"testing"
)

func makeTestSha1Store(t *testing.T) (Storer, func(), error) {
	testTempDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Errorf("Test setup failed, %v", err)
		return nil, nil, err
	}

	testBaseDir, err := ioutil.TempDir("", "")
	if err != nil {
		os.RemoveAll(testTempDir)
		t.Errorf("Test setup failed, %v", err)
		return nil, nil, err
	}

	clean := func() {
		os.RemoveAll(testTempDir)
		os.RemoveAll(testBaseDir)
	}

	return Sha1Store(testBaseDir, testTempDir), clean, nil
}

func TestSha1Store(t *testing.T) {
	_, clean, _ := makeTestSha1Store(t)
	defer clean()
}

func TestStore(t *testing.T) {
	s, clean, _ := makeTestSha1Store(t)
	defer clean()

	writer, err := s.Store()

	if err != nil {
		t.Errorf("Call to store failed, %v", err)
	}

	writer.Write([]byte("Store some test info"))
	if err != nil {
		t.Errorf("Call to Write failed, %v", err)
	}

	err = writer.Close()
	if err != nil {
		t.Errorf("Call to Close failed, %v", err)
	}

	id, err := writer.Identity()
	if err != nil {
		t.Errorf("Call to Close failed, %v", err)
	}
	t.Log("ID: " + id.String())
}

func TestWriteAfterClose(t *testing.T) {
	s, clean, _ := makeTestSha1Store(t)
	defer clean()

	writer, err := s.Store()

	if err != nil {
		t.Errorf("Call to store failed, %v", err)
	}

	n, err := writer.Write([]byte("Store some test info"))
	if err != nil {
		t.Errorf("Call to Write failed, %v", err)
	}

	err = writer.Close()
	if err != nil {
		t.Errorf("Call to Close failed, %v", err)
	}

	n, err = writer.Write([]byte("Store some test info"))
	if err == nil {
		t.Errorf("Call to Write after Close did not fail, returned %v", n)
	}
}

func TestPrematureIdentity(t *testing.T) {
	s, clean, _ := makeTestSha1Store(t)
	defer clean()

	writer, err := s.Store()

	if err != nil {
		t.Errorf("Call to store failed, %v", err)
	}

	writer.Write([]byte("Store some test info"))
	if err != nil {
		t.Errorf("Call to Write failed, %v", err)
	}

	id, err := writer.Identity()
	if err == nil {
		t.Errorf("Call to Identity before Close did not fail, %v", id)
	}
}

func TestGarbageCollect1(t *testing.T) {
	s, clean, _ := makeTestSha1Store(t)
	defer clean()

	s.GarbageCollect(nil)
	return
}

func TestGarbageCollect2(t *testing.T) {
	s, clean, _ := makeTestSha1Store(t)
	defer clean()

	done := make(chan struct{})

	s.GarbageCollect(done)
	<-done

	return
}
