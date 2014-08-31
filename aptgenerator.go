package main

import (
	"io"
	"net/http"
	"os"

	"code.google.com/p/go.crypto/openpgp"
)

// Interface for any Apt repository generator
type AptGenerator interface {
	Regenerate() error // Regenerate the apt archive
	AddSession(session UploadSessioner) (respStatus int, respObj string, err error)
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

func (a *aptBlobArchiveGenerator) AddSession(session UploadSessioner) (respStatus int, respObj string, err error) {
	//Move the files into the pool
	for _, f := range session.Items() {
		dstdir := a.Repo.PoolFilePath(f.Filename)
		stat, err := os.Lstat(dstdir)

		if err != nil {
			if os.IsNotExist(err) {
				err = os.MkdirAll(dstdir, 0777)
				if err != nil {
					respStatus = http.StatusInternalServerError
					respObj = "File move failed, " + err.Error()
				}
			} else {
				respStatus = http.StatusInternalServerError
				respObj = "File move failed, "
			}
		} else {
			if !stat.IsDir() {
				respStatus = http.StatusInternalServerError
				respObj = "Destinatio path, " + dstdir + " is not a directory"
			}
		}

		//err = os.Rename(f.Filename, dstdir+f.Filename)
		// This should really be a re-linking
		reader, _ := os.Open(f.Filename)
		err = a.AddFile(dstdir+f.Filename, reader)
		if err != nil {
			respStatus = http.StatusInternalServerError
			respObj = "File move failed, " + err.Error()
		}
	}

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
