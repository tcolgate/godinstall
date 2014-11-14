package main

import (
	"compress/gzip"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"regexp"
	"sort"
	"strings"
	"time"

	"code.google.com/p/go.crypto/openpgp"
	"code.google.com/p/go.crypto/openpgp/clearsign"
)

// Architecture collates all the information for an architecture
// within a component of a release.
type Architecture struct {
	Name       string
	PackagesGz StoreID
}

// Component collates all the information for a component
// within a release
type Component struct {
	Name          string
	Architectures []Architecture
	SourcesGz     StoreID
}

// Release collects all the information for a release
type Release struct {
	ParentID    StoreID
	IndexID     StoreID
	CodeName    string
	SuiteName   string
	Description string
	Version     string
	Date        time.Time
	Components  []Component
	InRelease   StoreID
	Release     StoreID
	ReleaseGPG  StoreID
	Actions     []ReleaseLogAction
	PoolPattern string
	poolPattern *regexp.Regexp
}

// ReleaseLogActionType is used to document the list of actions
// take by a given merge
type ReleaseLogActionType int

//	ActionUNKNOWN     - An uninitilized action item
//	ActionADD         - An item was added
//	ActionDELETE      - An item was explicitly deleted
//	ActionPRUNE       - An item was pruned by the pruning rules
//	ActionSKIPPRESENT - An item was skipped, as it alerady existed
//	ActionSKIPPRUNE   - An item was was skipped, dur to purge rules
const (
	ActionUNKNOWN     ReleaseLogActionType = 1 << iota
	ActionADD         ReleaseLogActionType = 2
	ActionDELETE      ReleaseLogActionType = 3
	ActionPRUNE       ReleaseLogActionType = 4
	ActionSKIPPRESENT ReleaseLogActionType = 5
	ActionSKIPPRUNE   ReleaseLogActionType = 6
	ActionTRIM        ReleaseLogActionType = 7
)

// ReleaseLogAction desribes an action taken during a merge or update
type ReleaseLogAction struct {
	Type        ReleaseLogActionType
	Description string
}

type archTempData struct {
	packagesFileWriter   *WriteHasher
	packagesGzFile       *WriteHasher
	packagesGzStore      StoreWriteCloser
	packagesGzFileWriter io.WriteCloser
	packagesWriter       io.Writer
	packagesSize         int64
	packagesMD5          string
	packagesSHA1         string
	packagesSHA256       string
	packagesGzSize       int64
	packagesGzMD5        string
	packagesGzSHA1       string
	packagesGzSHA256     string
	PackagesGzID         StoreID
}

type compTempData map[string]*archTempData

type relTempData map[string]compTempData

// NewRelease creates a new release object in the specified store, based on the
// parent and built using the passed in index, and associated set of
// actions
func NewRelease(store Archiver, parentid StoreID, indexid StoreID, actions []ReleaseLogAction) (id StoreID, err error) {
	release := Release{}
	release.ParentID = parentid
	release.IndexID = indexid
	release.Actions = actions
	release.Date = time.Now()

	parent, err := store.GetRelease(parentid)
	if err != nil {
		return nil, err
	}

	release.CodeName = parent.CodeName
	release.SuiteName = parent.SuiteName
	release.Description = parent.Description
	release.Version = parent.Version
	release.PoolPattern = parent.PoolPattern

	// For the moment we'll only have the main component

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

	// We want to merge all packages of arch all into each binary-$arch
	// so we walk the package index in a first pass to find all the archs
	// we are dealing with, we'll setup the temporary data to track the
	// packages files while we are at it.
	relMap := make(relTempData, 0)
	archMap := make(map[string]bool, 0)
	preIndex, err := store.OpenReleaseIndex(release.IndexID)
	if err != nil {
		return
	}

	for {
		item, err := preIndex.NextItem()
		if err != nil {
			break
		}

		switch item.Type {
		case BINARY:
			control, err := store.GetBinaryControlFile(item.ControlID)
			if err != nil {
				log.Println("Error retrieving control file for ", item, err)
				continue
			}

			archName, ok := control[0].GetValue("Architecture")
			if ok {
				archMap[archName] = true
			}

			compName := "main"
			_, ok = control[0].GetValue("Section")
			if !ok {
				log.Println("control file does not contain section, default component to main")
			} else {
				// Should try and guess a component from the section ala reprepro
				compName = "main"
			}

			comp, ok := relMap[compName]
			if !ok {
				relMap[compName] = make(compTempData, 0)
				comp = relMap[compName]
			}

			arch, ok := comp[archName]
			if !ok {
				// This is a new arch in this component
				arch = new(archTempData)

				arch.packagesFileWriter = MakeWriteHasher(ioutil.Discard)
				arch.packagesGzStore, err = store.Store()
				if err != nil {
					return nil, err
				}
				arch.packagesGzFile = MakeWriteHasher(arch.packagesGzStore)
				arch.packagesGzFileWriter = gzip.NewWriter(arch.packagesGzFile)
				arch.packagesWriter = io.MultiWriter(arch.packagesFileWriter, arch.packagesGzFileWriter)
				comp[archName] = arch
			}
		}
	}
	preIndex.Close()

	archNames := make(sort.StringSlice, 0)
	for archName := range archMap {
		if archName != "all" {
			archNames = append(archNames, archName)
		}
	}
	archNames.Sort()

	//Now we'll walk the index again to collate the package lists
	index, err := store.OpenReleaseIndex(release.IndexID)
	if err != nil {
		return
	}
	defer index.Close()

	for {
		item, err := index.NextItem()
		if err != nil {
			break
		}

		switch item.Type {
		case BINARY:
			control, err := store.GetBinaryControlFile(item.ControlID)
			if err != nil {
				log.Println("Error retrieving control file for ", item, err)
				continue
			}

			archName, ok := control[0].GetValue("Architecture")
			if !ok {
				log.Println("control file does not contain architecture, default to all")
				archName = "all"
			}

			compName := "main"
			_, ok = control[0].GetValue("Section")
			if !ok {
				log.Println("control file does not contain section, default component to main")
			} else {
				// Should try and guess a component from the section ala reprepro
				compName = "main"
			}

			comp, ok := relMap[compName]
			if !ok {
				log.Println("New file appeared in index!")
				continue
			}

			arch, ok := comp[archName]
			if !ok {
				log.Println("New file appeared in index!")
				continue
			}

			filename, ok := control[0].GetValue("Filename")
			if !ok {
				log.Println("control file does not contain filename")
				continue
			}
			poolpath := release.PoolFilePath(filename)
			path := poolpath + filename
			log.Println(path)
			control[0].SetValue("Filename", path)

			FormatControlFile(arch.packagesWriter, control)
			arch.packagesWriter.Write([]byte("\n"))

			if archName == "all" {
				for _, otherArchName := range archNames {
					otherArch, _ := comp[otherArchName]
					FormatControlFile(otherArch.packagesWriter, control)
					otherArch.packagesWriter.Write([]byte("\n"))
				}
			}
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

	release.Components = make([]Component, 0)

	compNames := make(sort.StringSlice, 0)
	for compName := range relMap {
		compNames = append(compNames, compName)
	}
	compNames.Sort()

	for _, compName := range compNames {
		archsMap := relMap[compName]

		var archs []Architecture
		archNames := make(sort.StringSlice, 0)
		for archName := range archsMap {
			archNames = append(archNames, archName)
		}
		archNames.Sort()

		for _, archName := range archNames {
			archFiles := archsMap[archName]
			archFiles.packagesSize = archFiles.packagesFileWriter.Count()
			archFiles.packagesMD5 = hex.EncodeToString(archFiles.packagesFileWriter.MD5Sum())
			archFiles.packagesSHA1 = hex.EncodeToString(archFiles.packagesFileWriter.SHA1Sum())
			archFiles.packagesSHA256 = hex.EncodeToString(archFiles.packagesFileWriter.SHA256Sum())

			archFiles.packagesGzFileWriter.Close()
			archFiles.packagesGzStore.Close()
			archFiles.packagesGzMD5 = hex.EncodeToString(archFiles.packagesGzFile.MD5Sum())
			archFiles.packagesGzSHA1 = hex.EncodeToString(archFiles.packagesGzFile.SHA1Sum())
			archFiles.packagesGzSHA256 = hex.EncodeToString(archFiles.packagesGzFile.SHA256Sum())
			archFiles.PackagesGzID, _ = archFiles.packagesGzStore.Identity()
			archFiles.packagesGzSize, _ = store.Size(archFiles.PackagesGzID)

			arch := Architecture{
				Name:       archName,
				PackagesGz: archFiles.PackagesGzID,
			}
			archs = append(archs, arch)
		}

		comp := Component{
			Name:          compName,
			Architectures: archs,
		}
		release.Components = append(release.Components, comp)
	}

	/*
		sourcesInfo, _ := os.Stat(*a.Repo.RepoBase + "/Sources")
		sourcesGzInfo, _ := os.Stat(*a.Repo.RepoBase + "/Sources.gz")
	*/

	releaseControl := make(ControlFile, 1)
	para := MakeControlParagraph()
	releaseControl[0] = &para

	releaseStartFields := []string{"Origin", "Suite", "Codename"}
	releaseEndFields := []string{"SHA256"}
	releaseControl[0].SetValue("Origin", "GoDInstall")
	releaseControl[0].SetValue("Suite", release.SuiteName)
	releaseControl[0].SetValue("Codename", release.CodeName)

	archNames = append(archNames, "all")
	archNames.Sort()
	releaseControl[0].SetValue("Architectures", strings.Join(archNames, " "))

	releaseControl[0].SetValue("Components", strings.Join(compNames, " "))

	releaseControl[0].AddValue("MD5Sum", "")

	for i := range release.Components {
		comp := release.Components[i]
		for j := range release.Components[i].Architectures {
			arch := comp.Architectures[j]
			archFiles := relMap[comp.Name][arch.Name]

			pkgsLine := fmt.Sprintf("%v %d %v/binary-%v/Packages",
				archFiles.packagesMD5,
				archFiles.packagesSize,
				comp.Name,
				arch.Name,
			)
			releaseControl[0].AddValue("MD5Sum", pkgsLine)
			pkgsGzLine := fmt.Sprintf("%v %d %v/binary-%v/Packages.gz",
				archFiles.packagesGzMD5,
				archFiles.packagesGzSize,
				comp.Name,
				arch.Name,
			)
			releaseControl[0].AddValue("MD5Sum", pkgsGzLine)
		}
	}

	// This is a little convoluted. We'll ultimately write to this, but it may be
	// writing to unsigned and signed releases, or just the unsigned, depending
	// on user options
	var releaseWriter io.Writer
	unsignedReleaseFile, _ := store.Store()

	var signedReleaseFile StoreWriteCloser
	var signedReleaseWriter io.WriteCloser

	if store.SignerID() != nil {
		signedReleaseFile, err = store.Store()
		signedReleaseWriter, err = clearsign.Encode(signedReleaseFile, store.SignerID().PrivateKey, nil)
		if err != nil {
			return nil, errors.New("Error InRelease clear-signer, " + err.Error())
		}
		releaseWriter = io.MultiWriter(unsignedReleaseFile, signedReleaseWriter)
	} else {
		releaseWriter = unsignedReleaseFile
	}

	WriteDebianControl(releaseWriter, releaseControl, releaseStartFields, releaseEndFields)

	unsignedReleaseFile.Close()
	release.Release, _ = unsignedReleaseFile.Identity()

	if store.SignerID() != nil {
		signedReleaseWriter.Close()
		signedReleaseFile.Close()
		release.InRelease, _ = signedReleaseFile.Identity()

		// Generate detached GPG signature
		releaseReader, _ := store.Open(release.Release)
		defer releaseReader.Close()
		releaseGpgFile, _ := store.Store()
		err = openpgp.ArmoredDetachSign(releaseGpgFile, store.SignerID(), releaseReader, nil)
		releaseGpgFile.Close()
		release.ReleaseGPG, _ = releaseGpgFile.Identity()
	}

	release.Date = time.Now()

	releaseid, err := store.AddRelease(&release)
	log.Println("grrr: ", err)

	return releaseid, nil
}

// Trimmer is an type for describing functions that can be used to
// trim the repository history
type Trimmer func(*Release) bool

// MakeTimeTrimmer creates a trimmer function that reduces the repository
// history to a given window of time
func MakeTimeTrimmer(time.Duration) Trimmer {
	return func(commit *Release) (trim bool) {
		return false
	}
}

// MakeLengthTrimmer creates a trimmer function that reduces the repository
// history to a given number of commits
func MakeLengthTrimmer(commitcount int) Trimmer {
	count := commitcount
	return func(commit *Release) (trim bool) {
		if count >= 0 {
			count--
			return false
		}
		return true
	}
}

// Trim works it's way through the commit history, and rebuilds a new version
// of the repository history, truncated at the commit selected by the trimmer
/*
func Trim(head StoreID, t Trimmer) (newhead StoreID, err error) {
	var history []*Release
	newhead = head
	curr := head

	for {
		if StoreID(curr).String() == r.EmptyFileID().String() {
			// We reached an empty commit before we decided to trim
			// so just return the untrimmed origin StoreID
			return head, nil
		}

		c, err := r.GetRelease(curr)
		if err != nil {
			return head, err
		}

		if t(c) {
			break
		}
		history = append(history, c)
	}

	newhead = StoreID(r.EmptyFileID())
	history[len(history)-1].Actions = []ReleaseLogAction{
		ReleaseLogAction{
			Type:        ActionTRIM,
			Description: "Repository history trimmed",
		},
	}

	for i := len(history) - 1; i >= 0; i-- {
		newcommit := history[i]
		newcommit.Parent = newhead
		newhead, err = r.AddRelease(newcommit)
		if err != nil {
			return head, err
		}
	}

	return
}
*/

// PoolFilePath provides the full path to the location
// in the debian apt pool for a given file, expanded using
// the pool pattern
func (r *Release) PoolFilePath(filename string) (poolpath string) {
	poolpath = "pool/" + r.CodeName + "/"

	if r.poolPattern == nil {
		var err error
		r.poolPattern, err = regexp.CompilePOSIX("^(" + r.PoolPattern + ")")
		if err != nil {
			log.Println(err.Error())
			return
		}
	}

	matches := r.poolPattern.FindSubmatch([]byte(filename))
	if len(matches) > 0 {
		poolpath = poolpath + string(matches[0]) + "/"
	}

	return
}
