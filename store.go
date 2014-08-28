package main

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"io/ioutil"
)

// An interface to a content addressable file store

type StoreID []byte

func (i StoreID) String() string {
	return hex.EncodeToString(i)
}

type Storer interface {
	Store() (StoreWriteCloser, error) // Write something to the store
	Open(StoreID) (io.Reader, error)  // Open a file by id
	Link(StoreID, ...string) error    // Link a file id to a given location
	Delete(StoreID) error             // Delete an file by id
	GarbageCollect(chan<- struct{})   // Remove all files with no external links
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

	writer := &hashedStoreWriter{
		hasher: h,
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

func (t *sha1Store) GarbageCollect(done chan<- struct{}) {
	go func() {
		msg := new(struct{})
		done <- *msg
	}()
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
	io.WriteCloser
	CloseAndLink(string) (StoreID, error) // Return the
	Identity() (StoreID, error)           // Return the
}

type hashedStoreWriter struct {
	writer io.Writer
	hasher hash.Hash
	closed bool
}

func (w *hashedStoreWriter) Write(p []byte) (n int, err error) {
	if w.closed {
		return 0, errors.New("Attempt to write to closed file")
	}

	return w.writer.Write(p)
}

func (w *hashedStoreWriter) Close() (err error) {
	w.closed = true
	return
}

func (w *hashedStoreWriter) CloseAndLink(target string) (id StoreID, err error) {
	return
}

func (w *hashedStoreWriter) Identity() (id StoreID, err error) {
	if !w.closed {
		return nil, errors.New("Identitty called before file storage was compete")
	}

	return StoreID(w.hasher.Sum(nil)), nil
}
