package store

import (
	"errors"
	"hash"
	"io"
)

// WriteCloser is used to arite a file to the file store.
// Write data, and call Close() when done, Identity then retries the items ID
type WriteCloser interface {
	io.WriteCloser
	Identity() (ID, error) // Return the ID of the item in the store, once closed.
}

type hashedStoreWriter struct {
	writer   io.Writer
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
	w.done <- struct{}{}

	return <-w.complete
}

func (w *hashedStoreWriter) Identity() (id ID, err error) {
	if !w.closed {
		return nil, errors.New("Identitty called before file storage was compete")
	}

	return ID(w.hasher.Sum(nil)), nil
}
