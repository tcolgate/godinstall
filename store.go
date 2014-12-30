package main

import (
	"crypto/sha1"
	"encoding/hex"
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

// An interface to a content addressable file store

// StoreID is a handle for an object within the store
type StoreID []byte

func (i StoreID) String() string {
	return hex.EncodeToString(i)
}

// StoreIDFromString parses a string and returns the StoreID
// that it would represent
func StoreIDFromString(str string) (StoreID, error) {
	b, err := hex.DecodeString(str)
	return StoreID(b), err
}

// StoreWalker is used to enumerate a store. Given a StoreID as an
// argument, it returns all the StoreIDs that the contents of the
// input store item may refer to
type StoreWalker func(StoreID) []StoreID

// Storer is an interface to a content addressable blob store. Files
// can be written to it, and then accesed and referred to based on an
// ID representing the content of the written item.
type Storer interface {
	Store() (StoreWriteCloser, error)     // Write something to the store
	Open(StoreID) (io.ReadCloser, error)  // Open a file by id
	Size(StoreID) (int64, error)          // Open a file by id
	Link(StoreID, ...string) error        // Link a file id to a given location
	UnLink(StoreID) error                 // UnLink a blob
	EmptyFileID() StoreID                 // Return the StoreID for an 0 byte object
	SetRef(name string, id StoreID) error // Set a reference
	GetRef(name string) (StoreID, error)  // Get a reference
	ListRefs() map[string]StoreID         // Get a reference
	ForEach(f func(StoreID))              // Call function for each ID
}

type sha1Store struct {
	tempDir     string
	baseDir     string
	prefixDepth int
}

func (t *sha1Store) storeIDToPathName(id StoreID) (string, string, error) {
	if len(id) != sha1.Size {
		log.Println("ID: ", id)
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

		name, path, err := t.storeIDToPathName(id)
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

func (t *sha1Store) Open(id StoreID) (reader io.ReadCloser, err error) {
	name, _, err := t.storeIDToPathName(id)
	if err != nil {
		err = errors.New("Failed to translate id to path " + err.Error())
		return nil, err
	}
	reader, err = os.Open(name)

	return reader, err
}

func (t *sha1Store) Size(id StoreID) (size int64, err error) {
	name, _, err := t.storeIDToPathName(id)
	if err != nil {
		err = errors.New("Failed to translate id to path " + err.Error())
		return 0, err
	}
	info, err := os.Stat(name)
	size = info.Size()

	return size, err
}

func (t *sha1Store) Link(id StoreID, targets ...string) (err error) {
	name, _, err := t.storeIDToPathName(id)
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

func (t *sha1Store) UnLink(id StoreID) (err error) {
	name, _, err := t.storeIDToPathName(id)
	if err != nil {
		err = errors.New("Failed to translate id to path " + err.Error())
		return err
	}
	err = os.Remove(name)

	return err
}

// Sha1Store creates a blob store that uses  hex encoded sha1 strings of
// ingested blobs for IDs
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

func (t *sha1Store) EmptyFileID() StoreID {
	hasher := sha1.New()
	id := hasher.Sum(nil)
	return id
}

func (t *sha1Store) SetRef(name string, id StoreID) error {
	refsPath := t.baseDir + "/refs/"
	refDir := refsPath

	prefix := strings.LastIndex(name, "/")
	if prefix > -1 {
		refDir = refDir + name[0:prefix+1]
		name = name[prefix+1:]
		if name == "" {
			return errors.New("reference name cannot end in /")
		}
	}

	refFile := refDir + name + ".ref"

	err := os.MkdirAll(refDir, 0777)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(refFile, []byte(id.String()), 0777)
	return err
}

func (t *sha1Store) GetRef(name string) (StoreID, error) {
	refsPath := t.baseDir + "/refs"
	refFile := refsPath + "/" + name + ".ref"

	f, err := os.Open(refFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	refStr, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}

	refid, err := StoreIDFromString(string(refStr))

	return refid, err
}

func (t *sha1Store) ListRefs() map[string]StoreID {
	refsPath := t.baseDir + "/refs"
	refs := make(map[string]StoreID)

	walker := func(path string, info os.FileInfo, err error) error {
		var reterr error
		if err != nil {
			return err
		}

		if info.IsDir() {
			return reterr
		}
		refname := strings.TrimSuffix(path[len(refsPath)+1:], ".ref")
		id, _ := t.GetRef(refname)
		refs[refname] = id
		return reterr
	}

	filepath.Walk(refsPath, walker)
	return refs
}

// StoreWriteCloser is used to arite a file to the file store.
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

func (t *sha1Store) ForEach(fn func(StoreID)) {
	walker := func(path string, info os.FileInfo, err error) error {
		var reterr error

		if info.IsDir() {
			return reterr
		}

		id, _ := StoreIDFromString(info.Name())
		if len(id) != 0 {
			fn(id)
		}
		return reterr
	}

	filepath.Walk(t.baseDir, walker)
	return
}
