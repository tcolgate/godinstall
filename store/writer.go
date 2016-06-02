package store

import (
	"errors"
	"hash"
	"io"
	"os"
)

// WriteCloser is used to arite a file to the file store.
// Write data, and call Close() when done, Identity then retries the items ID
type WriteCloser interface {
	io.WriteCloser
	Identity() (ID, error) // Return the ID of the item in the store, once closed.
}

type hashedStoreWriter struct {
	writer   io.Writer
	file     *os.File
	hasher   hash.Hash
	closed   bool
	done     chan struct{}
	complete chan error
}

func (w *hashedStoreWriter) Write(p []byte) (n int, err error) {
	if w.closed {
		return 0, errors.New("attempt to write to closed file")
	}

	return w.writer.Write(p)
}

func (w *hashedStoreWriter) Close() (err error) {
	if w.closed {
		return errors.New("attempt to close closed file")
	}

	w.closed = true

	err = w.file.Sync()
	if err != nil {
		err = errors.New("failed to sync blob, " + err.Error())
		os.Remove(w.file.Name())
		return
	}

	err = w.file.Close()
	if err != nil {
		err = errors.New("failed to close blob, " + err.Error())
		os.Remove(w.file.Name())
		return
	}

	id, err := w.Identity()
	if err != nil {
		err = errors.New("failed to rewtrieve hash of blob, " + err.Error())
		os.Remove(w.file.Name())
		return
	}

	name, path, err := t.idToPath(id)
	if err != nil {
		err = errors.New("Failed to translate id to path " + err.Error())
		os.Remove(w.file.Name())
		return
	}

	err = os.MkdirAll(path, 0755)
	if err != nil {
		err = errors.New("Failed to create blob directory " + err.Error())
		os.Remove(w.file.Name())
		return
	}

	_, err = os.Stat(name)
	if err != nil {
		err = os.Link(w.file.Name(), name)
		if err != nil {
			err = errors.New("Failed to link blob  " + err.Error())
			os.Remove(w.file.Name())
			return
		}
	}

	os.Remove(w.file.Name())
	return
}

func (w *hashedStoreWriter) Identity() (id ID, err error) {
	if !w.closed {
		return nil, errors.New("Identitty called before file storage was compete")
	}

	return ID(w.hasher.Sum(nil)), nil
}
