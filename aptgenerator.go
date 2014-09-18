package main

import (
	"encoding/hex"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/stapelberg/godebiancontrol"

	"compress/gzip"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"

	"code.google.com/p/go.crypto/openpgp"
	"code.google.com/p/go.crypto/openpgp/clearsign"
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

	id, err := a.blobStore.GetRef("master")
	newindex, err := OpenRepoIndex(id, a.blobStore)
	if err != nil {
		return
	}

	for {
		item, err := newindex.NextItem()
		if err != nil {
			log.Println(err)
			break
		}

		log.Println(item)
	}

	sourcesStartFields := []string{"Package"}
	sourcesEndFields := []string{"Description"}

	sourcesFile, err := os.Create(*a.Repo.RepoBase + "/Sources")
	sourcesMD5er := md5.New()
	sourcesSHA1er := sha1.New()
	sourcesSHA256er := sha256.New()
	sourcesHashedWriter := io.MultiWriter(
		sourcesFile,
		sourcesMD5er,
		sourcesSHA1er,
		sourcesSHA256er)

	sourcesGzFile, err := os.Create(*a.Repo.RepoBase + "/Sources.gz")
	sourcesGzMD5er := md5.New()
	sourcesGzSHA1er := sha1.New()
	sourcesGzSHA256er := sha256.New()
	sourcesGzHashedWriter := io.MultiWriter(
		sourcesGzFile,
		sourcesGzMD5er,
		sourcesGzSHA1er,
		sourcesGzSHA256er)
	sourcesGzWriter := gzip.NewWriter(sourcesGzHashedWriter)

	sourcesWriter := io.MultiWriter(sourcesHashedWriter, sourcesGzWriter)

	packagesStartFields := []string{"Package", "Version", "Filename", "Size"}
	packagesEndFields := []string{"MD5sum", "SHA1", "SHA256", "Description"}

	packagesFile, err := os.Create(*a.Repo.RepoBase + "/Packages")
	packagesMD5er := md5.New()
	packagesSHA1er := sha1.New()
	packagesSHA256er := sha256.New()
	packagesHashedWriter := io.MultiWriter(
		packagesFile,
		packagesMD5er,
		packagesSHA1er,
		packagesSHA256er)

	packagesGzFile, err := os.Create(*a.Repo.RepoBase + "/Packages.gz")
	packagesGzMD5er := md5.New()
	packagesGzSHA1er := sha1.New()
	packagesGzSHA256er := sha256.New()
	packagesGzHashedWriter := io.MultiWriter(
		packagesGzFile,
		packagesGzMD5er,
		packagesGzSHA1er,
		packagesGzSHA256er)
	packagesGzWriter := gzip.NewWriter(packagesGzHashedWriter)

	packagesWriter := io.MultiWriter(packagesHashedWriter, packagesGzWriter)

	f := func(path string, info os.FileInfo, err error) error {
		var reterr error

		switch {
		case info.IsDir():
			return reterr
		case strings.HasSuffix(path, ".deb"):
			reader, _ := os.Open(path)
			pkg := NewDebPackage(reader, nil)
			controlData, _ := pkg.Control()
			controlData["Filename"] = path[len(*a.Repo.RepoBase)+1:]
			controlData["Size"] = strconv.FormatInt(info.Size(), 10)
			md5, _ := pkg.Md5()
			controlData["MD5sum"] = hex.EncodeToString(md5)
			sha1, _ := pkg.Sha1()
			controlData["SHA1"] = hex.EncodeToString(sha1)
			sha256, _ := pkg.Sha256()
			controlData["SHA256"] = hex.EncodeToString(sha256)
			paragraphs := make([]godebiancontrol.Paragraph, 1)
			paragraphs[0] = controlData
			WriteDebianControl(packagesWriter, paragraphs, packagesStartFields, packagesEndFields)
			packagesWriter.Write([]byte("\n"))
		case strings.HasSuffix(path, ".dsc"):
			reader, _ := os.Open(path)
			paragraphs, _ := godebiancontrol.Parse(reader)
			paragraphs[0]["Package"] = paragraphs[0]["Source"]
			delete(paragraphs[0], "Source")

			WriteDebianControl(sourcesWriter, paragraphs, sourcesStartFields, sourcesEndFields)
			sourcesWriter.Write([]byte("\n"))
		}

		return reterr
	}
	filepath.Walk(a.Repo.PoolFilePath(""), f)

	sourcesFile.Close()
	//sourcesMD5 := hex.EncodeToString(sourcesMD5er.Sum(nil))
	//sourcesSHA1 := hex.EncodeToString(sourcesSHA1er.Sum(nil))
	sourcesSHA256 := hex.EncodeToString(sourcesSHA256er.Sum(nil))

	sourcesGzWriter.Close()
	//sourcesGzMD5 := hex.EncodeToString(sourcesGzMD5er.Sum(nil))
	//sourcesGzSHA1 := hex.EncodeToString(sourcesGzSHA1er.Sum(nil))
	sourcesGzSHA256 := hex.EncodeToString(sourcesGzSHA256er.Sum(nil))

	packagesFile.Close()
	//packagesMd5 := hex.EncodeToString(packagesMD5er.Sum(nil))
	//packagesSha1 := hex.EncodeToString(packagesSHA1er.Sum(nil))
	packagesSHA256 := hex.EncodeToString(packagesSHA256er.Sum(nil))

	packagesGzWriter.Close()
	//packagesGzMD5 := hex.EncodeToString(packagesGzMD5er.Sum(nil))
	//packagesGzSHA1 := hex.EncodeToString(packagesGzSHA1er.Sum(nil))
	packagesGzSHA256 := hex.EncodeToString(packagesGzSHA256er.Sum(nil))

	sourcesInfo, _ := os.Stat(*a.Repo.RepoBase + "/Sources")
	sourcesGzInfo, _ := os.Stat(*a.Repo.RepoBase + "/Sources.gz")
	packagesInfo, _ := os.Stat(*a.Repo.RepoBase + "/Packages")
	packagesGzInfo, _ := os.Stat(*a.Repo.RepoBase + "/Packages.gz")

	release := make([]godebiancontrol.Paragraph, 1)
	release[0] = make(godebiancontrol.Paragraph)

	releaseStartFields := []string{"Origin", "Suite", "Codename"}
	releaseEndFields := []string{"SHA256"}
	release[0]["Origin"] = "godinstall"
	release[0]["Suite"] = "stable"
	release[0]["Components"] = "main"
	release[0]["Architectures"] = "amd64 all"
	SHA256Str :=
		packagesSHA256 + " " + strconv.FormatInt(packagesInfo.Size(), 10) + " Packages\n" +
			" " + packagesGzSHA256 + " " + strconv.FormatInt(packagesGzInfo.Size(), 10) + " Packages.gz\n" +
			" " + sourcesSHA256 + " " + strconv.FormatInt(sourcesInfo.Size(), 10) + " Sources\n" +
			" " + sourcesGzSHA256 + " " + strconv.FormatInt(sourcesGzInfo.Size(), 10) + " Sources.gz\n"

	release[0]["SHA256"] = SHA256Str

	unsignedReleaseFile, err := os.Create(*a.Repo.RepoBase + "/Release")

	var releaseWriter io.Writer
	var signedReleaseWriter io.WriteCloser

	if a.SignerId != nil {
		signedReleaseFile, err := os.Create(*a.Repo.RepoBase + "/InRelease")
		signedReleaseWriter, err = clearsign.Encode(signedReleaseFile, a.SignerId.PrivateKey, nil)
		if err != nil {
			return errors.New("Error InRelease clear-signer, " + err.Error())
		}
		releaseWriter = io.MultiWriter(unsignedReleaseFile, signedReleaseWriter)
	} else {
		releaseWriter = unsignedReleaseFile
	}

	WriteDebianControl(releaseWriter, release, releaseStartFields, releaseEndFields)

	unsignedReleaseFile.Close()
	if a.SignerId != nil {
		signedReleaseWriter.Close()
	}

	return
}

func (a *aptBlobArchiveGenerator) AddSession(session UploadSessioner) (respStatus int, respObj string, err error) {
	respStatus = http.StatusOK

	items, err := RepoItemsFromChanges(session.Items(), a.blobStore)
	if err != nil {
		respStatus = http.StatusInternalServerError
		respObj = "Collating repository items failed, " + err.Error()
		return respStatus, respObj, err
	}

	index, _ := NewRepoIndex(a.blobStore)
	for i := range items {
		index.AddRepoItem(items[i])
	}
	_, err = index.Close()

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
