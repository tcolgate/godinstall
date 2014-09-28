package main

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"testing"
)

var storeTestPrefixDepth = 3
var storeTestString = "Store some test info"
var storeTestStringHash = "d83bc8150b1469193705c6e2e166db5963be38bf"
var storeTestNullStringHash = "da39a3ee5e6b4b0d3255bfef95601890afd80709"

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

	return Sha1Store(testBaseDir, testTempDir, storeTestPrefixDepth), clean, nil
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
		return
	}

	writer.Write([]byte(storeTestString))
	if err != nil {
		t.Errorf("Call to Write failed, %v", err)
		return
	}

	// Close with no additional reference
	err = writer.Close()
	if err != nil {
		t.Errorf("Call to Close failed, %v", err)
		return
	}

	id, err := writer.Identity()
	if err != nil {
		t.Errorf("Call to Close failed, %v", err)
		return
	}

	if id.String() != storeTestStringHash {
		t.Errorf("Incorrect hash, %v, expected %v", id, storeTestStringHash)
		return
	}

	reader, err := s.Open(id)
	if err != nil {
		t.Errorf("open blob by id failed, %v", err)
		return
	}

	storedData := make([]byte, 1000)
	n, err := io.ReadFull(reader, storedData)
	if n == 0 {
		t.Errorf("read from blob failed, %v", err)
		return
	}

	if string(storedData[0:n]) != storeTestString {
		t.Errorf("wrong data in  blob , %v", string(storedData[0:n]))
		return
	}

}

func TestStoreTwice(t *testing.T) {
	s, clean, _ := makeTestSha1Store(t)
	defer clean()

	// Store content first time
	writer, err := s.Store()

	if err != nil {
		t.Errorf("Call to store failed, %v", err)
		return
	}

	writer.Write([]byte(storeTestString))
	if err != nil {
		t.Errorf("Call to Write failed, %v", err)
		return
	}

	// Close with no additional reference
	err = writer.Close()
	if err != nil {
		t.Errorf("Call to Close failed, %v", err)
		return
	}

	id, err := writer.Identity()
	if err != nil {
		t.Errorf("Call to Close failed, %v", err)
		return
	}

	if id.String() != storeTestStringHash {
		t.Errorf("Incorrect hash, %v, expected %v", id, storeTestStringHash)
		return
	}

	// Store content a second time
	writer, err = s.Store()

	if err != nil {
		t.Errorf("Call to store failed, %v", err)
		return
	}

	writer.Write([]byte(storeTestString))
	if err != nil {
		t.Errorf("Call to Write failed, %v", err)
		return
	}

	err = writer.Close()
	if err != nil {
		t.Errorf("Call to Close failed, %v", err)
		return
	}

	id, err = writer.Identity()
	if err != nil {
		t.Errorf("Call to Close failed, %v", err)
		return
	}

	if id.String() != storeTestStringHash {
		t.Errorf("Incorrect hash, %v, expected %v", id, storeTestStringHash)
		return
	}
}

func TestStoreTwiceWithGC(t *testing.T) {
	s, clean, _ := makeTestSha1Store(t)
	defer clean()

	// Store content first time
	writer, err := s.Store()

	if err != nil {
		t.Errorf("Call to store failed, %v", err)
		return
	}

	writer.Write([]byte(storeTestString))
	if err != nil {
		t.Errorf("Call to Write failed, %v", err)
		return
	}

	err = writer.Close()
	if err != nil {
		t.Errorf("Call to Close failed, %v", err)
		return
	}

	id, err := writer.Identity()
	if err != nil {
		t.Errorf("Call to Close failed, %v", err)
		return
	}

	if id.String() != storeTestStringHash {
		t.Errorf("Incorrect hash, %v, expected %v", id, storeTestStringHash)
		return
	}

	// Store content a second time
	writer, err = s.Store()

	if err != nil {
		t.Errorf("Call to store failed, %v", err)
		return
	}

	writer.Write([]byte(storeTestString))
	if err != nil {
		t.Errorf("Call to Write failed, %v", err)
		return
	}

	err = writer.Close()
	if err != nil {
		t.Errorf("Call to Close failed, %v", err)
		return
	}

	id, err = writer.Identity()
	if err != nil {
		t.Errorf("Call to Close failed, %v", err)
		return
	}

	if id.String() != storeTestStringHash {
		t.Errorf("Incorrect hash, %v, expected %v", id, storeTestStringHash)
		return
	}
}

func TestStoreNullString(t *testing.T) {
	s, clean, _ := makeTestSha1Store(t)
	defer clean()

	emptyId := s.EmptyFileID()
	if emptyId.String() != storeTestNullStringHash {
		t.Errorf("Incorrect hash for empty blob %v, expected %v", emptyId.String(), storeTestNullStringHash)
		return
	}

	writer, err := s.Store()

	if err != nil {
		t.Errorf("Call to store failed, %v", err)
		return
	}

	err = writer.Close()
	if err != nil {
		t.Errorf("Call to Close failed, %v", err)
		return
	}

	id, err := writer.Identity()
	if err != nil {
		t.Errorf("Call to Close failed, %v", err)
		return
	}

	if bytes.Compare(id, emptyId) != 0 {
		t.Errorf("Incorrect hash for NULL string, %v, expected %v", id.String(), storeTestNullStringHash)
		return
	}
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

func TestReferences(t *testing.T) {
	s, clean, _ := makeTestSha1Store(t)
	defer clean()

	writer, err := s.Store()

	if err != nil {
		t.Errorf("Call to store failed, %v", err)
		return
	}

	writer.Write([]byte(storeTestString))
	if err != nil {
		t.Errorf("Call to Write failed, %v", err)
		return
	}

	// Close with no additional reference
	err = writer.Close()
	if err != nil {
		t.Errorf("Call to Close failed, %v", err)
		return
	}

	id, err := writer.Identity()
	if err != nil {
		t.Errorf("Call to Close failed, %v", err)
		return
	}

	if id.String() != storeTestStringHash {
		t.Errorf("Incorrect hash, %v, expected %v", id, storeTestStringHash)
		return
	}

	err = s.SetRef("myref", id)
	if err != nil {
		t.Errorf("Call to SetRef failed, %v", err)
		return
	}

	refid, err := s.GetRef("myref")
	if err != nil {
		t.Errorf("Call to GetRef failed, %v", err)
		return
	}

	if refid.String() != id.String() {
		t.Errorf("Stored and retirnved references differ")
		return
	}

	reader, err := s.Open(refid)
	if err != nil {
		t.Errorf("open blob by id failed, %v", err)
		return
	}

	storedData := make([]byte, 1000)
	n, err := io.ReadFull(reader, storedData)
	if n == 0 {
		t.Errorf("read from blob failed, %v", err)
		return
	}

	if string(storedData[0:n]) != storeTestString {
		t.Errorf("wrong data in  blob , %v", string(storedData[0:n]))
		return
	}
}

func TestReferences2(t *testing.T) {
	s, clean, _ := makeTestSha1Store(t)
	defer clean()

	writer, err := s.Store()

	if err != nil {
		t.Errorf("Call to store failed, %v", err)
		return
	}

	writer.Write([]byte(storeTestString))
	if err != nil {
		t.Errorf("Call to Write failed, %v", err)
		return
	}

	// Close with no additional reference
	err = writer.Close()
	if err != nil {
		t.Errorf("Call to Close failed, %v", err)
		return
	}

	id, err := writer.Identity()
	if err != nil {
		t.Errorf("Call to Close failed, %v", err)
		return
	}

	if id.String() != storeTestStringHash {
		t.Errorf("Incorrect hash, %v, expected %v", id, storeTestStringHash)
		return
	}

	err = s.SetRef("my/deep/ref", id)
	if err != nil {
		t.Errorf("Call to SetRef failed, %v", err)
		return
	}

	refid, err := s.GetRef("my/deep/ref")
	if err != nil {
		t.Errorf("Call to GetRef failed, %v", err)
		return
	}

	if refid.String() != id.String() {
		t.Errorf("Stored and retirnved references differ")
		return
	}

	reader, err := s.Open(refid)
	if err != nil {
		t.Errorf("open blob by id failed, %v", err)
		return
	}

	storedData := make([]byte, 1000)
	n, err := io.ReadFull(reader, storedData)
	if n == 0 {
		t.Errorf("read from blob failed, %v", err)
		return
	}

	if string(storedData[0:n]) != storeTestString {
		t.Errorf("wrong data in  blob , %v", string(storedData[0:n]))
		return
	}

}
