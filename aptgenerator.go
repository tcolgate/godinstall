package main

import (
	"encoding/hex"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

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
	GenerateCommit(IndexID) (CommitID, error) // Regenerate the apt archive
	ReifyCommit(CommitID) error               // Reify the commit into  the archive
	AddSession(session UploadSessioner) (respStatus int, respObj string, err error)
}

// An AptGenerator that uses a version historied blob store
type aptBlobArchiveGenerator struct {
	Repo     *aptRepo        // The repo to update
	PrivRing openpgp.KeyRing // Private keyring cotaining singing key
	SignerId *openpgp.Entity // The key to sign release file with
	store    RepoStorer      // The blob store to use
}

// Create a new AptGenerator that uses a version historied blob store
func NewAptBlobArchiveGenerator(
	repo *aptRepo,
	privRing openpgp.KeyRing,
	signerId *openpgp.Entity,
	store RepoStorer,
) AptGenerator {
	return &aptBlobArchiveGenerator{
		repo,
		privRing,
		signerId,
		store,
	}
}

func (a *aptBlobArchiveGenerator) GenerateCommit(indexid IndexID) (commitid CommitID, err error) {
	commit := &RepoCommit{}
	commit.Index = indexid
	/*
		sourcesFile, err := a.store.Store()
		sourcesMD5er := md5.New()
		sourcesSHA1er := sha1.New()
		sourcesSHA256er := sha256.New()
		sourcesHashedWriter := io.MultiWriter(
			sourcesFile,
			sourcesMD5er,
			sourcesSHA1er,
			sourcesSHA256er)

		sourcesGzFile, err := a.store.Store()
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
	*/

	packagesFile, err := a.store.Store()
	packagesMD5er := md5.New()
	packagesSHA1er := sha1.New()
	packagesSHA256er := sha256.New()
	packagesHashedWriter := io.MultiWriter(
		packagesFile,
		packagesMD5er,
		packagesSHA1er,
		packagesSHA256er)

	packagesGzFile, err := a.store.Store()
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

	newindex, err := a.store.OpenIndex(commit.Index)
	if err != nil {
		return
	}
	defer newindex.Close()

	for {
		item, err := newindex.NextItem()
		if err != nil {
			break
		}

		switch item.Type {
		case BINARY:
			control, err := a.store.GetBinaryControlFile(item.ControlID)
			if err != nil {
				log.Println("Error retrieving control file for ", item, err)
				continue
			}

			poolpath := a.Repo.PoolFilePath(control[0]["Filename"])
			path := poolpath[len(a.Repo.Base())+1:] + control[0]["Filename"]

			control[0]["Filename"] = path
			FormatControlData(packagesWriter, control)
			packagesWriter.Write([]byte("\n"))
			/*
				case SOURCE:
					control, _ := RetrieveSourceControlFile(a.store, item.ControlID)
					control[0]["Package"] = control[0]["Source"]
					delete(control[0], "Source")
					FormatControlData(sourcesWriter, control)
					sourcesWriter.Write([]byte("\n"))
			*/
		}
	}

	/*
		sourcesFile.Close()
		//sourcesMD5 := hex.EncodeToString(sourcesMD5er.Sum(nil))
		//sourcesSHA1 := hex.EncodeToString(sourcesSHA1er.Sum(nil))
		sourcesSHA256 := hex.EncodeToString(sourcesSHA256er.Sum(nil))

		sourcesGzWriter.Close()
		//sourcesGzMD5 := hex.EncodeToString(sourcesGzMD5er.Sum(nil))
		//sourcesGzSHA1 := hex.EncodeToString(sourcesGzSHA1er.Sum(nil))
		sourcesGzSHA256 := hex.EncodeToString(sourcesGzSHA256er.Sum(nil))
	*/

	packagesFile.Close()
	commit.Packages, _ = packagesFile.Identity()
	packagesSize, _ := a.store.Size(commit.Packages)
	packagesMD5 := hex.EncodeToString(packagesMD5er.Sum(nil))
	packagesSHA1 := hex.EncodeToString(packagesSHA1er.Sum(nil))
	packagesSHA256 := hex.EncodeToString(packagesSHA256er.Sum(nil))

	packagesGzWriter.Close()
	packagesGzFile.Close()
	commit.PackagesGz, _ = packagesGzFile.Identity()
	packagesGzSize, _ := a.store.Size(commit.PackagesGz)
	packagesGzMD5 := hex.EncodeToString(packagesGzMD5er.Sum(nil))
	packagesGzSHA1 := hex.EncodeToString(packagesGzSHA1er.Sum(nil))
	packagesGzSHA256 := hex.EncodeToString(packagesGzSHA256er.Sum(nil))

	/*
		sourcesInfo, _ := os.Stat(*a.Repo.RepoBase + "/Sources")
		sourcesGzInfo, _ := os.Stat(*a.Repo.RepoBase + "/Sources.gz")
	*/

	release := make([]godebiancontrol.Paragraph, 1)
	release[0] = make(godebiancontrol.Paragraph)

	releaseStartFields := []string{"Origin", "Suite", "Codename"}
	releaseEndFields := []string{"SHA256"}
	release[0]["Origin"] = "godinstall"
	release[0]["Suite"] = "stable"
	release[0]["Components"] = "main"
	release[0]["Architectures"] = "amd64 all"
	MD5Str := "\n" +
		" " + packagesMD5 + " " + strconv.FormatInt(packagesSize, 10) + " Packages\n" +
		" " + packagesGzMD5 + " " + strconv.FormatInt(packagesGzSize, 10) + " Packages.gz\n"
	SHA1Str := "\n" +
		" " + packagesSHA1 + " " + strconv.FormatInt(packagesSize, 10) + " Packages\n" +
		" " + packagesGzSHA1 + " " + strconv.FormatInt(packagesGzSize, 10) + " Packages.gz\n"
	SHA256Str := "\n" +
		" " + packagesSHA256 + " " + strconv.FormatInt(packagesSize, 10) + " Packages\n" +
		" " + packagesGzSHA256 + " " + strconv.FormatInt(packagesGzSize, 10) + " Packages.gz\n"
	//			" " + sourcesSHA256 + " " + strconv.FormatInt(sourcesInfo.Size(), 10) + " Sources\n" +
	//			" " + sourcesGzSHA256 + " " + strconv.FormatInt(sourcesGzInfo.Size(), 10) + " Sources.gz\n"

	release[0]["MD5Sum"] = MD5Str
	release[0]["SHA1"] = SHA1Str
	release[0]["SHA256"] = SHA256Str

	// This is a little convoluted. We'll ultimately write to this, but it may be
	// writing to unsigned and signed releases, or just the unsigned, depending
	// on user options
	var releaseWriter io.Writer
	unsignedReleaseFile, _ := a.store.Store()

	var signedReleaseFile StoreWriteCloser
	var signedReleaseWriter io.WriteCloser

	if a.SignerId != nil {
		signedReleaseFile, err = a.store.Store()
		signedReleaseWriter, err = clearsign.Encode(signedReleaseFile, a.SignerId.PrivateKey, nil)
		if err != nil {
			return nil, errors.New("Error InRelease clear-signer, " + err.Error())
		}
		releaseWriter = io.MultiWriter(unsignedReleaseFile, signedReleaseWriter)
	} else {
		releaseWriter = unsignedReleaseFile
	}

	WriteDebianControl(releaseWriter, release, releaseStartFields, releaseEndFields)

	unsignedReleaseFile.Close()
	commit.Release, _ = unsignedReleaseFile.Identity()

	if a.SignerId != nil {
		signedReleaseWriter.Close()
		signedReleaseFile.Close()
		commit.InRelease, _ = signedReleaseFile.Identity()
	}

	commit.Date = time.Now()

	commitid, _ = a.store.AddCommit(commit)
	return
}

func (a *aptBlobArchiveGenerator) ReifyCommit(id CommitID) (err error) {
	commit, err := a.store.GetCommit(id)
	indexId := commit.Index
	index, err := a.store.OpenIndex(indexId)
	defer index.Close()

	clearRepo := func() {
		os.Remove(a.Repo.Base() + "/Packages")
		os.Remove(a.Repo.Base() + "/Packages.gz")
		os.Remove(a.Repo.Base() + "/Release")
		os.Remove(a.Repo.Base() + "/InRelease")
		os.RemoveAll(a.Repo.Base() + "/pool")
	}

	clearRepo()
	defer func() {
		if err != nil {
			clearRepo()
		}
	}()

	for {
		item, err := index.NextItem()
		if err != nil {
			if err != io.EOF {
				return err
			}
			break
		}
		for i := range item.Files {
			file := item.Files[i]
			path := a.Repo.PoolFilePath(file.Name)
			filepath := path + file.Name
			err = a.store.Link(file.ID, filepath)
			if err != nil {
				return err
			}
		}
	}

	err = a.store.Link(commit.Packages, a.Repo.Base()+"/Packages")
	err = a.store.Link(commit.PackagesGz, a.Repo.Base()+"/Packages.gz")
	//err = a.store.Link(commit.Sources, a.Repo.Base()+"/Sources")
	//err = a.store.Link(commit.SourcesGz, a.Repo.Base()+"/Sources.gz")
	err = a.store.Link(commit.Release, a.Repo.Base()+"/Release")
	if a.SignerId != nil {
		err = a.store.Link(commit.InRelease, a.Repo.Base()+"/InRelease")
	}

	return
}

func (a *aptBlobArchiveGenerator) AddSession(session UploadSessioner) (respStatus int, respObj string, err error) {
	respStatus = http.StatusOK
	respObj = "Index committed"

	items, err := a.store.ItemsFromChanges(session.Items())
	if err != nil {
		respStatus = http.StatusInternalServerError
		respObj = "Collating repository items failed, " + err.Error()
		return respStatus, respObj, err
	}

	head, err := a.store.GetHead("master")
	if err != nil {
		if os.IsNotExist(err) {
			emptyidx, err := a.store.EmptyIndex()
			if err != nil {
				respStatus = http.StatusInternalServerError
				respObj = "Creating empty index failed, " + err.Error()
				return respStatus, respObj, err
			}
			head, err = a.GenerateCommit(emptyidx)
			if err != nil {
				respStatus = http.StatusInternalServerError
				respObj = "Creating empty commit failed, " + err.Error()
				return respStatus, respObj, err
			}
			a.store.SetHead("master", head)
		} else {
			respStatus = http.StatusInternalServerError
			respObj = "Opening repo head failed, " + err.Error()
			return respStatus, respObj, err
		}
	}

	newidx, err := a.store.MergeItemsIntoCommit(head, items)
	if err != nil {
		respStatus = http.StatusInternalServerError
		respObj = "Creating new index failed, " + err.Error()
	}

	newhead, err := a.GenerateCommit(newidx)
	if err != nil {
		respStatus = http.StatusInternalServerError
		respObj = "Creating updated commit failed, " + err.Error()
		return respStatus, respObj, err
	}
	a.store.SetHead("master", newhead)

	err = a.ReifyCommit(newhead)
	if err != nil {
		respStatus = http.StatusInternalServerError
		respObj = "Repopulating the archive directory failed," + err.Error()
		return respStatus, respObj, err
	}

	return
}
