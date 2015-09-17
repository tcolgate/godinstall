package hasher

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"hash"
	"io"
)

// Hasher counts writes to a io.writer, used to assess the size of a file
// without needing to store it
type Hasher struct {
	backing  io.Writer
	count    int64
	md5er    hash.Hash
	sha1er   hash.Hash
	sha256er hash.Hash
}

// Write implmeents the io.Writer interface for the hasher
func (w *Hasher) Write(p []byte) (n int, err error) {
	n, err = w.backing.Write(p)
	w.md5er.Write(p)
	w.sha1er.Write(p)
	w.sha256er.Write(p)
	w.count += int64(n)
	return
}

// Count returns the number of hytes in total that have
// been written to the writer
func (w *Hasher) Count() int64 {
	return w.count
}

// MD5Sum of the input so far
func (w *Hasher) MD5Sum() []byte {
	return w.md5er.Sum(nil)
}

// SHA1Sum of the input so far
func (w *Hasher) SHA1Sum() []byte {
	return w.sha1er.Sum(nil)
}

// SHA256Sum of the input so far
func (w *Hasher) SHA256Sum() []byte {
	return w.sha256er.Sum(nil)
}

// New creates an io.Writer to calculate the sha1,sha256 and
// md5 sums and measure the size of a data written to the passed writer
func New(w io.Writer) *Hasher {
	return &Hasher{
		backing:  w,
		count:    0,
		md5er:    md5.New(),
		sha1er:   sha1.New(),
		sha256er: sha256.New(),
	}
}
