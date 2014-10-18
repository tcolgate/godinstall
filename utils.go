package main

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"hash"
	"io"
)

// WriteHasher counts writes to a io.writer, used to assess the size of a file
// without needing to store it
type WriteHasher struct {
	backing  io.Writer
	count    int64
	md5er    hash.Hash
	sha1er   hash.Hash
	sha256er hash.Hash
}

func (w *WriteHasher) Write(p []byte) (n int, err error) {
	n, err = w.backing.Write(p)
	w.md5er.Write(p)
	w.sha1er.Write(p)
	w.sha256er.Write(p)
	w.count += int64(n)
	return
}

func (w *WriteHasher) Count() int64 {
	return w.count
}
func (w *WriteHasher) MD5Sum() []byte {
	return w.md5er.Sum(nil)
}
func (w *WriteHasher) SHA1Sum() []byte {
	return w.sha1er.Sum(nil)
}
func (w *WriteHasher) SHA256Sum() []byte {
	return w.sha256er.Sum(nil)
}

// MakeWriteHasher creates an io.Writer to calculate the sha1,sha256 and
// md5 sums and measure the size of a data written to the passed writer
func MakeWriteHasher(w io.Writer) *WriteHasher {
	return &WriteHasher{
		backing:  w,
		count:    0,
		md5er:    md5.New(),
		sha1er:   sha1.New(),
		sha256er: sha256.New(),
	}
}
