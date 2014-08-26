package main

import (
	"crypto/sha1"
	"errors"
	"hash"
	"io"
	"io/ioutil"
)

// An interface to a content addressable file store

type StoreID string

type Storer interface {
	Store() (StoreWriteCloser, error) // Write something to the store
	Open(StoreID) (io.Reader, error)  // Open a file by id
	Link(StoreID, ...string) error    // Link a file id to a given location
	Delete(StoreID) error             // Delete an file by id
	GarbageCollect()                  // Remove all files with no external links
}

type sha1Store struct {
	tempDir string
	baseDir string
}

func (t *sha1Store) Store() (StoreWriteCloser, error) {
	file, err := ioutil.TempFile(t.tempDir, "blob")
	if err != nil {
		return nil, err
	}

	h := sha1.New()

	mwriter := io.MultiWriter(file, h)

	writer := &sha1StoreWriter{
		sha1er: h,
		writer: mwriter,
	}
	return writer, nil
}

func (t *sha1Store) Open(id StoreID) (io.Reader, error) {
	return nil, nil
}

func (t *sha1Store) Link(id StoreID, targets ...string) error {
	return nil
}

func (t *sha1Store) Delete(id StoreID) error {
	return nil
}

func (t *sha1Store) GarbageCollect() {
	return
}

// Create a sha1
func Sha1Store(baseDir string, tempDir string) Storer {
	store := &sha1Store{
		tempDir: tempDir,
		baseDir: baseDir,
	}

	return store
}

// StoreWriterCloser is used to arite a file to the file store.
// Write data, and call Close() when done. After calling Close,
// Identitiy will return the ID of the item in the store.
type StoreWriteCloser interface {
	io.Writer
	io.Closer
	Identity() (StoreID, error) // Return the
}

type sha1StoreWriter struct {
	writer io.Writer
	sha1er hash.Hash
	closed bool
}

func (t *sha1StoreWriter) Write(p []byte) (n int, err error) {
	return
}

func (t *sha1StoreWriter) Close() (err error) {
	return
}

func (t *sha1StoreWriter) Identity() (id StoreID, err error) {
	if !t.closed {
		return "", errors.New("Identitty called before file storage was compete")
	}

	return StoreID(t.sha1er.Sum(nil)), nil
}
