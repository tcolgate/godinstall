package main

import (
	"code.google.com/p/go.crypto/openpgp"
)

// Interface for any Apt repository generator
type AptGenerator interface {
	Regenerate() error // Regenerate the apt archive
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

func (*aptBlobArchiveGenerator) Regenerate() (err error) {
	return
}
