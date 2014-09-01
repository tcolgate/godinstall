package main

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/stapelberg/godebiancontrol"

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
	sourcesFile, err := os.Create(*a.Repo.RepoBase + "/Sources")
	sourcesStartFields := []string{"Package"}
	sourcesEndFields := []string{"Description"}

	packageFile, err := os.Create(*a.Repo.RepoBase + "/Packages")
	packagesStartFields := []string{"Package"}
	packagesEndFields := []string{"Description"}

	f := func(path string, info os.FileInfo, err error) error {
		var reterr error

		switch {
		case info.IsDir():
			return reterr
		case strings.HasSuffix(path, ".deb"):
			reader, _ := os.Open(path)
			pkg := NewDebPackage(reader, nil)
			controlData, _ := pkg.Control()
			paragraphs := make([]godebiancontrol.Paragraph, 1)
			paragraphs[0] = controlData
			WriteDebianControl(packageFile, paragraphs, packagesStartFields, packagesEndFields)
			packageFile.Write([]byte("\n"))
		case strings.HasSuffix(path, ".dsc"):
			reader, _ := os.Open(path)
			paragraphs, _ := godebiancontrol.Parse(reader)
			paragraphs[0]["Package"] = paragraphs[0]["Source"]
			delete(paragraphs[0], "Source")

			WriteDebianControl(sourcesFile, paragraphs, sourcesStartFields, sourcesEndFields)
			sourcesFile.Write([]byte("\n"))
		}

		return reterr
	}
	filepath.Walk(*a.Repo.RepoBase, f)

	sourcesFile.Close()
	packageFile.Close()

	return
}

func (a *aptBlobArchiveGenerator) AddSession(session UploadSessioner) (respStatus int, respObj string, err error) {
	respStatus = http.StatusOK

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
