// Package store is a content addressable file store
// TODO(tcm): Abstract out file operations to allow alternate backing stores
package store

import (
	"crypto/sha1"
	"errors"
	"hash"
	"io"
	"io/ioutil"
	"os"
)

// Walker is used to enumerate a store. Given a StoreID as an
// argument, it returns all the StoreIDs that the contents of the
// input store item may refer to
type Walker func(ID) []ID

// Storer is an interface to a content addressable blob store. Files
// can be written to it, and then accesed and referred to based on an
// ID representing the content of the written item.
type Storer interface {
	Store() (WriteCloser, error) // Write something to the store

	Open(ID) (io.ReadCloser, error) // Open a file by id
	Size(ID) (int64, error)         // Open a file by id
	Link(ID, ...string) error       // Link a file id to a given location
	UnLink(ID) error                // UnLink a blob
	ForEach(f func(ID))             // Call function for each ID

	EmptyFileID() ID       // Return the StoreID for an 0 byte object
	IsEmptyFileID(ID) bool // Compares the ID to the stores empty ID

	SetRef(name string, id ID) error // Set a reference
	GetRef(name string) (ID, error)  // Get a reference
	DeleteRef(name string) error     // Delete a reference
	ListRefs() map[string]ID         // Get a reference
}

type hashStore struct {
	Hasher
	tempDir     string
	baseDir     string
	prefixDepth int
}

func (t *hashStore) Store() (WriteCloser, error) {
	file, err := ioutil.TempFile(t.tempDir, "blob")

	if err != nil {
		return nil, err
	}

	h := t.NewHash()
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
		// We can't use defer to clean up the TempFile, as
		// we must ensure it is deleted before we return success
		// to the calling channel. Should probably rewrite this

		extraLink := <-doneChan

		err := file.Sync()
		if err != nil {
			err = errors.New("failed to sync blob, " + err.Error())
			os.Remove(file.Name())
			writer.complete <- err
			return
		}

		err = file.Close()
		if err != nil {
			err = errors.New("failed to close blob, " + err.Error())
			os.Remove(file.Name())
			writer.complete <- err
			return
		}

		id, err := writer.Identity()
		if err != nil {
			err = errors.New("failed to rewtrieve hash of blob, " + err.Error())
			os.Remove(file.Name())
			writer.complete <- err
			return
		}

		name, path, err := t.idToPath(id)
		if err != nil {
			err = errors.New("Failed to translate id to path " + err.Error())
			os.Remove(file.Name())
			writer.complete <- err
			return
		}

		err = os.MkdirAll(path, 0755)
		if err != nil {
			err = errors.New("Failed to create blob directory " + err.Error())
			os.Remove(file.Name())
			writer.complete <- err
			return
		}

		_, err = os.Stat(name)
		if err == nil {
			os.Remove(file.Name())
		} else {
			err = os.Link(file.Name(), name)
			if err != nil {
				err = errors.New("Failed to link blob  " + err.Error())
				os.Remove(file.Name())
				writer.complete <- err
				return
			}
		}

		if extraLink != "" {
			srcInfo, _ := os.Stat(name)
			targetInfo, _ := os.Stat(extraLink)

			if !os.SameFile(srcInfo, targetInfo) {
				err = os.Link(name, extraLink)
				if err != nil {
					err = errors.New("Failed to link blob  " + err.Error())
					os.Remove(file.Name())
					writer.complete <- err
					return
				}
			}
		}

		os.Remove(file.Name())
		writer.complete <- err
	}()

	return writer, nil
}

func CopyToStore(d Storer, r io.ReadCloser) (id ID, err error) {
	w, err := d.Store()
	if err != nil {
		return nil, errors.New("during copy to store, " + err.Error())
	}

	if _, err = io.Copy(w, r); err != nil {
		return nil, errors.New("during copy to store, " + err.Error())
	}

	if r.Close() != nil {
		return nil, errors.New("during copy to store, " + err.Error())
	}

	if w.Close() != nil {
		return nil, errors.New("during copy to store, " + err.Error())
	}

	return w.Identity()
}

// New creates a blob store that uses  hex encoded hash strings of
// ingested blobs for IDs, using the provided hash function
func New(
	baseDir string, // Base directory of the persistant store
	tempDir string, // Temporary directory for ingesting files
	prefixDepth int, // How many chars to use for directory prefixes
	hf func() hash.Hash,
) Storer {
	store := &hashStore{
		Hasher:      NewHasher(hf),
		tempDir:     tempDir,
		baseDir:     baseDir,
		prefixDepth: prefixDepth,
	}

	return store
}

// Sha1Store creates a blob store that uses  hex encoded sha1 strings of
// ingested blobs for IDs
func Sha1Store(
	baseDir string, // Base directory of the persistant store
	tempDir string, // Temporary directory for ingesting files
	prefixDepth int, // How many chars to use for directory prefixes
) Storer {
	return New(
		tempDir,
		baseDir,
		prefixDepth,
		sha1.New)
}
