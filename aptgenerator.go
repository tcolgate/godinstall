package main

import (
	"io"

	"code.google.com/p/go.crypto/openpgp"
)

// Interface for any Apt repository generator
type AptGenerator interface {
	Regenerate() error                      // Regenerate the apt archive
	AddFile(name string, r io.Reader) error // Add the content of the reader with the given filename
}

// An AptGenerator that uses a version historied blob store
type aptBlobArchiveGenerator struct {
	Repo      *aptRepo        // The repo to update
	PrivRing  openpgp.KeyRing // Private keyring cotaining singing key
	SignerId  *openpgp.Entity // The key to sign release file with
	blobStore Storer          // The blob store to use
}

// Create a new AptGenerator that uses apt-ftparchive
func NewAptBlobArchiveGenerator(
	repo *aptRepo,
	privRing openpgp.KeyRing,
	signerId *openpgp.Entity,
	blobStore Storer,
) AptGenerator {
	return &aptBlobArchiveGenerator{
		repo,
		privRing,
		signerId,
		blobStore,
	}
}

func (a *aptBlobArchiveGenerator) Regenerate() (err error) {
	return
}

func (a *aptBlobArchiveGenerator) AddFile(filename string, data io.Reader) (err error) {
	store, err := a.blobStore.Store()
	if err != nil {
		return err
	}

	io.Copy(store, data)
	err = store.CloseAndLink(filename)
	return
}
