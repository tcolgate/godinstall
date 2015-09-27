// Package store is a content addressable file store
// TODO(tcm): Abstract out file operations to allow alternate backing stores
package store

import (
	"crypto/sha1"
	"errors"
	"hash"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
)

// Walker is used to enumerate a store. Given a StoreID as an
// argument, it returns all the StoreIDs that the contents of the
// input store item may refer to
type Walker func(ID) []ID

// Storer is an interface to a content addressable blob store. Files
// can be written to it, and then accesed and referred to based on an
// ID representing the content of the written item.
type Storer interface {
	Store() (WriteCloser, error)           // Write something to the store
	CopyToStore(io.ReadCloser) (ID, error) // Write something to the store
	Open(ID) (io.ReadCloser, error)        // Open a file by id
	Size(ID) (int64, error)                // Open a file by id
	Link(ID, ...string) error              // Link a file id to a given location
	UnLink(ID) error                       // UnLink a blob
	EmptyFileID() ID                       // Return the StoreID for an 0 byte object
	SetRef(name string, id ID) error       // Set a reference
	GetRef(name string) (ID, error)        // Get a reference
	DeleteRef(name string) error           // Delete a reference
	ListRefs() map[string]ID               // Get a reference
	ForEach(f func(ID))                    // Call function for each ID
}

type hashStore struct {
	newHash     func() hash.Hash
	tempDir     string
	baseDir     string
	prefixDepth int
}

func (t *hashStore) IDToPath(id ID) (string, string, error) {
	if len(id) != sha1.Size {
		log.Printf("Invalid store ID(%v)", id)
		debug.PrintStack()
		return "", "", errors.New("invalid StoreID " + string(id))
	}
	idStr := id.String()
	prefix := idStr[0:t.prefixDepth]

	filePath := t.baseDir + "/"
	for c := range prefix {
		filePath = filePath + string(prefix[c]) + "/"
	}

	fileName := filePath + idStr

	return fileName, filePath, nil
}

func (t *hashStore) Store() (WriteCloser, error) {
	file, err := ioutil.TempFile(t.tempDir, "blob")

	if err != nil {
		return nil, err
	}

	h := t.newHash()
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

		name, path, err := t.IDToPath(id)
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

func (t *hashStore) CopyToStore(r io.ReadCloser) (id ID, err error) {
	w, err := t.Store()
	if err != nil {
		return nil, errors.New("during copy to store, " + err.Error())
	}
	_, err = io.Copy(w, r)
	if err != nil {
		return nil, errors.New("during copy to store, " + err.Error())
	}
	err = r.Close()
	if err != nil {
		return nil, errors.New("during copy to store, " + err.Error())
	}
	err = w.Close()
	if err != nil {
		return nil, errors.New("during copy to store, " + err.Error())
	}

	return w.Identity()
}

func (t *hashStore) Open(id ID) (reader io.ReadCloser, err error) {
	name, _, err := t.IDToPath(id)
	if err != nil {
		err = errors.New("Failed to translate id to path " + err.Error())
		return nil, err
	}
	reader, err = os.Open(name)

	return reader, err
}

func (t *hashStore) Size(id ID) (size int64, err error) {
	name, _, err := t.IDToPath(id)
	if err != nil {
		err = errors.New("Failed to translate id to path " + err.Error())
		return 0, err
	}
	info, err := os.Stat(name)
	size = info.Size()

	return size, err
}

func (t *hashStore) Link(id ID, targets ...string) (err error) {
	name, _, err := t.IDToPath(id)
	if err != nil {
		err = errors.New("Failed to translate id to path " + err.Error())
		return err
	}

	for t := range targets {
		target := targets[t]
		prefix := strings.LastIndex(target, "/")
		if prefix > -1 {
			linkDir := target[0 : prefix+1]
			linkName := target[prefix+1:]
			if linkName == "" {
				return errors.New("reference name cannot end in /")
			}
			err = os.MkdirAll(linkDir, 0777)
			if err != nil {
				if os.IsExist(err) {
					break
				} else {
					return err
				}
			}
		}

		linkerr := os.Link(name, targets[t])
		if linkerr != nil {
			if os.IsExist(linkerr) {
				var f1, f2 os.FileInfo
				f1, err = os.Stat(name)
				f2, err = os.Stat(targets[t])
				if os.SameFile(f1, f2) {
					break
				}
				return errors.New("File associated with differet hashes")
			}
			return err
		}
	}
	return err
}

func (t *hashStore) UnLink(id ID) (err error) {
	name, _, err := t.IDToPath(id)
	if err != nil {
		err = errors.New("Failed to translate id to path " + err.Error())
		return err
	}
	err = os.Remove(name)

	return err
}

func (t *hashStore) EmptyFileID() ID {
	hasher := t.newHash()
	id := hasher.Sum(nil)
	return id
}

func (t *hashStore) ForEach(fn func(ID)) {
	walker := func(path string, info os.FileInfo, err error) error {
		var reterr error

		if info.IsDir() {
			return reterr
		}

		id, _ := IDFromString(info.Name())
		if len(id) != 0 {
			fn(id)
		}
		return reterr
	}

	filepath.Walk(t.baseDir, walker)
	return
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
		tempDir:     tempDir,
		baseDir:     baseDir,
		prefixDepth: prefixDepth,
		newHash:     hf,
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
