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
	"syscall"
)

// An interface to a content addressable file store

type StoreID []byte

func (i StoreID) String() string {
	return hex.EncodeToString(i)
}

func StoreIDFromString(str string) (StoreID, error) {
	b, err := hex.DecodeString(str)
	return StoreID(b), err
}

type Storer interface {
	Store() (StoreWriteCloser, error)     // Write something to the store
	Open(StoreID) (io.ReadCloser, error)  // Open a file by id
	Size(StoreID) (int64, error)          // Open a file by id
	Link(StoreID, ...string) error        // Link a file id to a given location
	GarbageCollect()                      // Remove all files with no external links
	EmptyFileID() StoreID                 // Return the StoreID for an 0 byte object
	SetRef(name string, id StoreID) error // Set a reference
	GetRef(name string) (StoreID, error)  // Get a reference
}

type sha1Store struct {
	tempDir     string
	baseDir     string
	prefixDepth int
}

func (t *sha1Store) storeIdToPathName(id StoreID) (string, string, error) {
	if len(id) != sha1.Size {
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
		// we must ensure it is delete before we return success
		// to the calling channel. Should probably rewrite this

		extraLink := <-doneChan
		id, err := writer.Identity()
		if err != nil {
			err = errors.New("failed to rewtrieve hash of blob, " + err.Error())
			os.Remove(file.Name())
			writer.complete <- err
			return
		}

		name, path, err := t.storeIdToPathName(id)
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
	name, _, err := t.storeIdToPathName(id)
	if err != nil {
		err = errors.New("Failed to translate id to path " + err.Error())
		return nil, err
	}
	reader, err = os.Open(name)

	return reader, err
}

func (t *sha1Store) Size(id StoreID) (size int64, err error) {
	name, _, err := t.storeIdToPathName(id)
	if err != nil {
		err = errors.New("Failed to translate id to path " + err.Error())
		return 0, err
	}
	info, err := os.Stat(name)
	size = info.Size()

	return size, err
}

func (t *sha1Store) Link(id StoreID, targets ...string) (err error) {
	name, _, err := t.storeIdToPathName(id)
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
			} else {
				return err
			}
		}
	}
	return err
}

func (t *sha1Store) GarbageCollect() {
	f := func(path string, info os.FileInfo, err error) error {
		var reterr error

		if info.IsDir() {
			return reterr
		}

		// Have no idea how I'd do this on other OSs
		stat := info.Sys().(*syscall.Stat_t)
		if stat != nil {
			nlink := int64(stat.Nlink)
			if nlink == 1 {
				reterr = os.Remove(path)
			}
		} else {
			log.Println("Could not get UNIX stat info for " + path)
		}
		return reterr
	}

	filepath.Walk(t.baseDir, f)
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

	refFile := refDir + name

	err := os.MkdirAll(refDir, 0777)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(refFile, []byte(id.String()), 0777)
	return err
}

func (t *sha1Store) GetRef(name string) (StoreID, error) {
	refsPath := t.baseDir + "/refs"
	refFile := refsPath + "/" + name

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
