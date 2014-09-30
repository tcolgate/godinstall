package main

import (
	"encoding/hex"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"compress/gzip"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"

	"code.google.com/p/go.crypto/openpgp"
	"code.google.com/p/go.crypto/openpgp/clearsign"
)

// Interface for any Apt repository generator
type AptGenerator interface {
	GenerateCommit(CommitID, IndexID, []RepoAction) (CommitID, error) // Regenerate the apt archive
	ReifyCommit(CommitID) error                                       // Reify the commit into  the archive
	AddSession(session UploadSessioner) (respStatus int, respObj string, err error)
}

// An AptGenerator that uses a version historied blob store
type aptBlobArchiveGenerator struct {
	Repo       *aptRepo        // The repo to update
	PrivRing   openpgp.KeyRing // Private keyring cotaining singing key
	SignerId   *openpgp.Entity // The key to sign release file with
	store      RepoStorer      // The blob store to use
	purgeRules PurgeRuleSet    // Rules to use for purging the repo
}

// Create a new AptGenerator that uses a version historied blob store
func NewAptBlobArchiveGenerator(
	repo *aptRepo,
	privRing openpgp.KeyRing,
	signerId *openpgp.Entity,
	store RepoStorer,
	purgeRules PurgeRuleSet,
) AptGenerator {
	return &aptBlobArchiveGenerator{
		repo,
		privRing,
		signerId,
		store,
		purgeRules,
	}
}

type writeCounter struct {
	backing io.Writer
	Count   int64
}

func (w *writeCounter) Write(p []byte) (n int, err error) {
	n, err = w.backing.Write(p)
	w.Count += int64(n)
	return
}

func MakeWriteCounter(w io.Writer) *writeCounter {
	return &writeCounter{
		backing: w,
		Count:   0,
	}
}

func (a *aptBlobArchiveGenerator) GenerateCommit(parentid CommitID, indexid IndexID, actions []RepoAction) (commitid CommitID, err error) {
	commit := &RepoCommit{}
	commit.Index = indexid
	commit.Parent = parentid
	commit.Actions = actions
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

	packagesFile := MakeWriteCounter(ioutil.Discard)
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

			filename, ok := control[0].GetValue("Filename")
			if !ok {
				log.Println("control file does not contain filename")
				continue
			}
			poolpath := a.Repo.PoolFilePath(filename)
			path := poolpath[len(a.Repo.Base())+1:] + filename

			control[0].SetValue("Filename", path)
			FormatControlFile(packagesWriter, control)
			packagesWriter.Write([]byte("\n"))
			/*
				case SOURCE:
					control, _ := RetrieveSourceControlFile(a.store, item.ControlID)
					control[0]["Package"] = control[0]["Source"]
					delete(control[0], "Source")
					FormatControlFile(sourcesWriter, control)
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

	packagesSize := packagesFile.Count
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

	release := make(ControlFile, 1)
	para := MakeControlParagraph()
	release[0] = &para

	releaseStartFields := []string{"Origin", "Suite", "Codename"}
	releaseEndFields := []string{"SHA256"}
	release[0].SetValue("Origin", "godinstall")
	release[0].SetValue("Suite", "stable")
	release[0].SetValue("Components", "main")
	release[0].SetValue("Architectures", "amd64 all")
	release[0].AddValue("MD5Sum", "")
	release[0].AddValue("MD5Sum", packagesMD5+" "+strconv.FormatInt(packagesSize, 10)+" Packages")
	release[0].AddValue("MD5Sum", packagesGzMD5+" "+strconv.FormatInt(packagesGzSize, 10)+" Packages.gz")
	release[0].AddValue("SHA1", "")
	release[0].AddValue("SHA1", packagesSHA1+" "+strconv.FormatInt(packagesSize, 10)+" Packages")
	release[0].AddValue("SHA1", packagesGzSHA1+" "+strconv.FormatInt(packagesGzSize, 10)+" Packages.gz")
	release[0].AddValue("SHA256", "")
	release[0].AddValue("SHA256", packagesSHA256+" "+strconv.FormatInt(packagesSize, 10)+" Packages")
	release[0].AddValue("SHA256", packagesGzSHA256+" "+strconv.FormatInt(packagesGzSize, 10)+" Packages.gz")

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

	clearTime := time.Now()
	clearRepo := func() {
		os.Remove(a.Repo.Base() + "/Packages")
		os.Remove(a.Repo.Base() + "/Packages.gz")
		os.Remove(a.Repo.Base() + "/Release")
		os.Remove(a.Repo.Base() + "/InRelease")
		os.RemoveAll(a.Repo.Base() + "/pool")
	}
	clearDuration := time.Since(clearTime)
	log.Println("Cleared old archive in ", clearDuration)

	clearRepo()
	defer func() {
		if err != nil {
			clearRepo()
		}
	}()

	log.Println("Reifing archive from commit ", StoreID(id).String())
	reifyTime := time.Now()
	fileCount := 0
	for {
		item, err := index.NextItem()
		if err != nil {
			if err != io.EOF {
				return err
			}
			break
		}
		for i := range item.Files {
			fileCount += 1
			file := item.Files[i]
			path := a.Repo.PoolFilePath(file.Name)
			filepath := path + file.Name
			err = a.store.Link(file.ID, filepath)
			if err != nil {
				return err
			}
		}
	}

	err = a.store.Link(commit.PackagesGz, a.Repo.Base()+"/Packages.gz")
	if err != nil {
		return err
	}

	// Reify the uncompressed packages file
	gzreader, err := os.Open(a.Repo.Base() + "/Packages.gz")
	defer gzreader.Close()
	if err != nil {
		return err
	}
	pkgs, err := os.Create(a.Repo.Base() + "/Packages")
	defer pkgs.Close()
	if err != nil {
		return err
	}
	err = a.store.Link(commit.PackagesGz, a.Repo.Base()+"/Packages.gz")
	if err != nil {
		return err
	}
	gunzipper, err := gzip.NewReader(gzreader)
	defer gunzipper.Close()
	if err != nil {
		return err
	}
	io.Copy(pkgs, gunzipper)

	//err = a.store.Link(commit.SourcesGz, a.Repo.Base()+"/Sources.gz")
	err = a.store.Link(commit.Release, a.Repo.Base()+"/Release")
	if err != nil {
		return err
	}

	if a.SignerId != nil {
		err = a.store.Link(commit.InRelease, a.Repo.Base()+"/InRelease")
		if err != nil {
			return err
		}
	}

	reifyDuration := time.Since(reifyTime)
	log.Printf("Reified %v files in %v ", fileCount, reifyDuration)

	return
}

func (a *aptBlobArchiveGenerator) AddSession(session UploadSessioner) (respStatus int, respObj string, err error) {
	//defer a.store.GarbageCollect()

	respStatus = http.StatusOK
	respObj = "Index committed"

	items, err := a.store.ItemsFromChanges(session.Items())
	if err != nil {
		respStatus = http.StatusInternalServerError
		respObj = "Collating repository items failed, " + err.Error()
		return respStatus, respObj, err
	}

	branchName := "master"
	head, err := a.store.GetHead(branchName)
	if err != nil {
		if os.IsNotExist(err) {
			emptyidx, err := a.store.EmptyIndex()
			if err != nil {
				respStatus = http.StatusInternalServerError
				respObj = "Creating empty index failed, " + err.Error()
				return respStatus, respObj, err
			}
			root := a.store.EmptyFileID()
			head, err = a.GenerateCommit(CommitID(root), emptyidx, []RepoAction{})
			if err != nil {
				respStatus = http.StatusInternalServerError
				respObj = "Creating empty commit failed, " + err.Error()
				return respStatus, respObj, err
			}
			a.store.SetHead(branchName, head)
			log.Println("Initialised new branch: " + branchName)
		} else {
			respStatus = http.StatusInternalServerError
			respObj = "Opening repo head failed, " + err.Error()
			return respStatus, respObj, err
		}
	}

	newidx, actions, err := a.store.MergeItemsIntoCommit(head, items, a.purgeRules)
	if err != nil {
		respStatus = http.StatusInternalServerError
		respObj = "Creating new index failed, " + err.Error()
	}

	newhead, err := a.GenerateCommit(head, newidx, actions)
	if err != nil {
		respStatus = http.StatusInternalServerError
		respObj = "Creating updated commit failed, " + err.Error()
		return respStatus, respObj, err
	}
	a.store.SetHead(branchName, newhead)

	for _, item := range actions {
		switch item.Type {
		case ActionADD:
			{
				log.Println("Added " + item.Description)
			}
		case ActionSKIPPRESENT:
			{
				log.Println("Item already present: " + item.Description)
			}
		case ActionPURGE:
			{
				log.Println("PUrged old item " + item.Description)
			}
		case ActionDELETE:
			{
				log.Println("Deleted " + item.Description)
			}
		default:
			{
				log.Println(item.Description)
			}
		}
	}
	log.Printf("Branch %v set to %v", branchName, StoreID(newhead).String())

	err = a.ReifyCommit(newhead)
	if err != nil {
		respStatus = http.StatusInternalServerError
		respObj = "Repopulating the archive directory failed," + err.Error()
		return respStatus, respObj, err
	}

	return
}
