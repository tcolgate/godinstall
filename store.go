package main

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"io/ioutil"
	"os"
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
	tempDir     string
	baseDir     string
	prefixDepth int
}

func (t *sha1Store) storeIdToPathName(id StoreID) (string, string) {
	idStr := id.String()
	prefix := idStr[0:t.prefixDepth]

	filePath := t.baseDir + "/"
	for c := range prefix {
		filePath = filePath + string(prefix[c]) + "/"
	}

	fileName := filePath + idStr

	return fileName, filePath
}

func (t *sha1Store) Store() (StoreWriteCloser, error) {
	file, err := ioutil.TempFile(t.tempDir, "blob")
	if err != nil {
		return nil, err
	}

	h := sha1.New()
	mwriter := io.MultiWriter(file, h)

	doneChan := make(chan string)
	completeChan := make(chan error)
	writer := &hashedStoreWriter{
		hasher:   h,
		writer:   mwriter,
		done:     doneChan,
		complete: completeChan,
	}

	go func() {
		defer os.Remove(file.Name())
		extraLink := <-doneChan
		id, _ := writer.Identity()
		name, path := t.storeIdToPathName(id)

		err := os.MkdirAll(path, 0755)
		if err != nil {
			err = errors.New("Failed to create blob directory " + err.Error())
			writer.complete <- err
			return
		}

		err = os.Link(file.Name(), name)
		if err != nil {
			err = errors.New("Failed to link blob  " + err.Error())
			writer.complete <- err
			return
		}

		if extraLink != "" {
			err = os.Link(name, extraLink)
			if err != nil {
				err = errors.New("Failed to link blob  " + err.Error())
				writer.complete <- err
				return
			}
		}

		writer.complete <- err
	}()

	return writer, nil
}

func (t *sha1Store) Open(id StoreID) (io.Reader, error) {
	name, _ := t.storeIdToPathName(id)
	reader, err := os.Open(name)

	return reader, err
}

func (t *sha1Store) Link(id StoreID, targets ...string) error {
	name, _ := t.storeIdToPathName(id)
	var err error

	for t := range targets {
		err = os.Link(name, targets[t])
		if err != nil {
			break
		}
	}
	return err
}

func (t *sha1Store) Delete(id StoreID) error {
	name, _ := t.storeIdToPathName(id)
	return os.Remove(name)
}

func (t *sha1Store) GarbageCollect(done chan<- struct{}) {
	go func() {
		msg := new(struct{})
		done <- *msg
	}()
	return
}

// Create a store using hex encoded sha1 strings of ingested
// blobs
func Sha1Store(
	baseDir string, // Base directory of the persistant store
	tempDir string, // Temporary directory for ingesting files
	prefixDepth int, // How many chars to use for directory prefixes
) Storer {
	store := &sha1Store{
		tempDir:     tempDir,
		baseDir:     baseDir,
		prefixDepth: prefixDepth,
	}

	return store
}

// StoreWriterCloser is used to arite a file to the file store.
// Write data, and call Close() when done. After calling Close,
// Identitiy will return the ID of the item in the store.
type StoreWriteCloser interface {
	io.WriteCloser
	CloseAndLink(string) error  // Return the
	Identity() (StoreID, error) // Return the
}

type hashedStoreWriter struct {
	writer   io.Writer
	hasher   hash.Hash
	closed   bool
	done     chan string // Indicate completion, pass a filename to create a link atomically
	complete chan error
}

func (w *hashedStoreWriter) Write(p []byte) (n int, err error) {
	if w.closed {
		return 0, errors.New("attempt to write to closed file")
	}

	return w.writer.Write(p)
}

func (w *hashedStoreWriter) Close() (err error) {
	return w.CloseAndLink("")
}

func (w *hashedStoreWriter) CloseAndLink(target string) error {
	if w.closed {
		return errors.New("attempt to close closed file")
	}

	w.closed = true
	w.done <- target

	return <-w.complete
}

func (w *hashedStoreWriter) Identity() (id StoreID, err error) {
	if !w.closed {
		return nil, errors.New("Identitty called before file storage was compete")
	}

	return StoreID(w.hasher.Sum(nil)), nil
}