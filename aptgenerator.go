package main

import (
	"encoding/hex"
	"errors"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"

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
	GenerateCommit(*RepoCommit) (StoreID, error) // Regenerate the apt archive
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

func (a *aptBlobArchiveGenerator) GenerateCommit(commit *RepoCommit) (commitid StoreID, err error) {
	/*
		sourcesFile, err := a.blobStore.Store()
		sourcesMD5er := md5.New()
		sourcesSHA1er := sha1.New()
		sourcesSHA256er := sha256.New()
		sourcesHashedWriter := io.MultiWriter(
			sourcesFile,
			sourcesMD5er,
			sourcesSHA1er,
			sourcesSHA256er)

		sourcesGzFile, err := a.blobStore.Store()
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

	packagesFile, err := a.blobStore.Store()
	packagesMD5er := md5.New()
	packagesSHA1er := sha1.New()
	packagesSHA256er := sha256.New()
	packagesHashedWriter := io.MultiWriter(
		packagesFile,
		packagesMD5er,
		packagesSHA1er,
		packagesSHA256er)

	packagesGzFile, err := a.blobStore.Store()
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

	newindex, err := OpenRepoIndex(commit.Index, a.blobStore)
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

		switch item.Type {
		case BINARY:
			control, _ := RetrieveBinaryControlFile(a.blobStore, item.ControlID)
			poolpath := a.Repo.PoolFilePath(control[0]["Filename"])
			path := poolpath[len(a.Repo.Base())+1:] + control[0]["Filename"]

			control[0]["Filename"] = path
			FormatControlData(packagesWriter, control)
			packagesWriter.Write([]byte("\n"))
			/*
				case SOURCE:
					control, _ := RetrieveSourceControlFile(a.blobStore, item.ControlID)
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
	packagesSize, _ := a.blobStore.Size(commit.Packages)
	packagesMD5 := hex.EncodeToString(packagesMD5er.Sum(nil))
	packagesSHA1 := hex.EncodeToString(packagesSHA1er.Sum(nil))
	packagesSHA256 := hex.EncodeToString(packagesSHA256er.Sum(nil))

	packagesGzWriter.Close()
	packagesGzFile.Close()
	commit.PackagesGz, _ = packagesGzFile.Identity()
	packagesGzSize, _ := a.blobStore.Size(commit.PackagesGz)
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
	unsignedReleaseFile, _ := a.blobStore.Store()

	var signedReleaseFile StoreWriteCloser
	var signedReleaseWriter io.WriteCloser

	if a.SignerId != nil {
		signedReleaseFile, err = a.blobStore.Store()
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

	log.Println(*commit)

	commitid, _ = StoreRepoCommit(a.blobStore, *commit)
	return
}

func (a *aptBlobArchiveGenerator) AddSession(session UploadSessioner) (respStatus int, respObj string, err error) {
	respStatus = http.StatusOK
	respObj = "Index committed"

	items, err := RepoItemsFromChanges(session.Items(), a.blobStore)
	if err != nil {
		respStatus = http.StatusInternalServerError
		respObj = "Collating repository items failed, " + err.Error()
		return respStatus, respObj, err
	}

	sort.Sort(ByIndexOrder(items))

	index, _ := NewRepoIndex(a.blobStore)
	for i := range items {
		index.AddRepoItem(items[i])
	}
	indexId, err := index.Close()
	if err != nil {
		respStatus = http.StatusInternalServerError
		respObj = "File move failed, " + err.Error()
	}

	var commit RepoCommit
	commit.Index = indexId
	_, err = a.GenerateCommit(&commit)
	if err != nil {
		respStatus = http.StatusInternalServerError
		respObj = "File move failed, " + err.Error()
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
