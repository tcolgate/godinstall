package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"sort"
	"strings"
	"time"

	"compress/gzip"

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
	Suite       string
	Description string
	Version     string
	Date        time.Time
	Components  []Component
	InRelease   StoreID
	Release     StoreID
	ReleaseGPG  StoreID
	Actions     []ReleaseLogAction
	TrimAfter   int32
	ConfigID    StoreID

	store  ArchiveStorer
	config *ReleaseConfig
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

type compTempData struct {
	archs               map[string]*archTempData
	sourcesFileWriter   *WriteHasher
	sourcesGzFile       *WriteHasher
	sourcesGzStore      StoreWriteCloser
	sourcesGzFileWriter io.WriteCloser
	sourcesWriter       io.Writer
	sourcesSize         int64
	sourcesMD5          string
	sourcesSHA1         string
	sourcesSHA256       string
	sourcesGzSize       int64
	sourcesGzMD5        string
	sourcesGzSHA1       string
	sourcesGzSHA256     string
	SourcesGzID         StoreID
}

type relTempData map[string]*compTempData

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
	release.Suite = parent.Suite
	release.Description = parent.Description
	release.Version = parent.Version
	release.ConfigID = parent.ConfigID

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
		e, err := preIndex.NextEntry()
		if err != nil {
			break
		}
		for _, b := range e.BinaryItems {
			archName := b.Architecture
			archMap[archName] = true

			compName := b.Component

			comp, ok := relMap[compName]
			if !ok {
				relMap[compName] = &compTempData{}
				comp = relMap[compName]
				comp.sourcesFileWriter = MakeWriteHasher(ioutil.Discard)
				comp.sourcesGzStore, err = store.Store()
				if err != nil {
					return nil, err
				}
				comp.sourcesGzFile = MakeWriteHasher(comp.sourcesGzStore)
				comp.sourcesGzFileWriter = gzip.NewWriter(comp.sourcesGzFile)
				comp.sourcesWriter = io.MultiWriter(comp.sourcesFileWriter, comp.sourcesGzFileWriter)
				comp.archs = make(map[string]*archTempData, 0)
			}

			arch, ok := comp.archs[archName]
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
				comp.archs[archName] = arch
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
		e, err := index.NextEntry()
		if err != nil {
			break
		}

		s := e.SourceItem

		srcName := s.Name
		srcVersion := s.Version.String()
		poolpath := fmt.Sprintf("%s%s/%s/",
			release.PoolFilePath(srcName),
			srcName,
			srcVersion,
		)

		if len(s.ControlID) != 0 {
			srcCtrl, err := store.GetControlFile(s.ControlID)
			if err != nil {
				log.Println("Could not retrieve control data, " + err.Error())
				continue
			}
			srcCtrl.Data[0].SetValue("Directory", poolpath)
			pkgStr, _ := srcCtrl.Data[0].GetValue("Source")
			srcCtrl.Data[0].SetValue("Package", pkgStr)
			srcComp, ok := relMap[s.Component]
			if !ok {
				log.Println("No Componenet for source item")
				continue
			}

			FormatDpkgControlFile(srcComp.sourcesWriter, srcCtrl)
			srcComp.sourcesWriter.Write([]byte("\n"))
		}

		for _, b := range e.BinaryItems {
			archName := b.Architecture
			compName := b.Component

			comp, ok := relMap[compName]
			if !ok {
				log.Println("New file appeared in index!")
				continue
			}

			arch, ok := comp.archs[archName]
			if !ok {
				log.Println("New file appeared in index!")
				continue
			}

			filename := b.Files[0].Name
			path := poolpath + filename
			control, err := store.GetControlFile(b.ControlID)
			if err != nil {
				log.Println("Could not retrieve control data, " + err.Error())
				continue
			}
			control.Data[0].SetValue("Filename", path)

			FormatDpkgControlFile(arch.packagesWriter, control)
			arch.packagesWriter.Write([]byte("\n"))

			if archName == "all" {
				for _, otherArchName := range archNames {
					otherArch, _ := comp.archs[otherArchName]
					FormatDpkgControlFile(otherArch.packagesWriter, control)
					otherArch.packagesWriter.Write([]byte("\n"))
				}
			}
		}
	}

	release.Components = make([]Component, 0)

	compNames := make(sort.StringSlice, 0)
	for compName := range relMap {
		compNames = append(compNames, compName)
	}
	compNames.Sort()

	for _, compName := range compNames {
		c := relMap[compName]

		c.sourcesSize = c.sourcesFileWriter.Count()
		c.sourcesMD5 = hex.EncodeToString(c.sourcesFileWriter.MD5Sum())
		c.sourcesSHA1 = hex.EncodeToString(c.sourcesFileWriter.SHA1Sum())
		c.sourcesSHA256 = hex.EncodeToString(c.sourcesFileWriter.SHA256Sum())

		c.sourcesGzFileWriter.Close()
		c.sourcesGzStore.Close()
		c.sourcesGzMD5 = hex.EncodeToString(c.sourcesGzFile.MD5Sum())
		c.sourcesGzSHA1 = hex.EncodeToString(c.sourcesGzFile.SHA1Sum())
		c.sourcesGzSHA256 = hex.EncodeToString(c.sourcesGzFile.SHA256Sum())
		c.SourcesGzID, _ = c.sourcesGzStore.Identity()
		c.sourcesGzSize, _ = store.Size(c.SourcesGzID)

		archsMap := c.archs

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
			SourcesGz:     c.SourcesGzID,
		}
		release.Components = append(release.Components, comp)
	}

	releaseControl := ControlFile{}
	para := MakeControlParagraph()

	releaseStartFields := []string{"Origin", "Suite", "Codename"}
	releaseEndFields := []string{"SHA256"}
	para.SetValue("Origin", "GoDInstall")
	para.SetValue("Suite", release.Suite)
	para.SetValue("Codename", release.CodeName)

	archNames = append(archNames, "all")
	archNames.Sort()
	para.SetValue("Architectures", strings.Join(archNames, " "))
	para.SetValue("Components", strings.Join(compNames, " "))
	para.AddValue("MD5Sum", "")

	for i := range release.Components {
		comp := release.Components[i]
		c := relMap[comp.Name]
		srcsLine := fmt.Sprintf("%v %d %v/source/Sources",
			c.sourcesMD5,
			c.sourcesSize,
			comp.Name,
		)
		para.AddValue("MD5Sum", srcsLine)
		srcsGzLine := fmt.Sprintf("%v %d %v/source/Sources.gz",
			c.sourcesMD5,
			c.sourcesSize,
			comp.Name,
		)
		para.AddValue("MD5Sum", srcsGzLine)

		for j := range release.Components[i].Architectures {
			arch := comp.Architectures[j]
			archFiles := relMap[comp.Name].archs[arch.Name]

			pkgsLine := fmt.Sprintf("%v %d %v/binary-%v/Packages",
				archFiles.packagesMD5,
				archFiles.packagesSize,
				comp.Name,
				arch.Name,
			)
			para.AddValue("MD5Sum", pkgsLine)
			pkgsGzLine := fmt.Sprintf("%v %d %v/binary-%v/Packages.gz",
				archFiles.packagesGzMD5,
				archFiles.packagesGzSize,
				comp.Name,
				arch.Name,
			)
			para.AddValue("MD5Sum", pkgsGzLine)
		}
	}

	releaseControl.Data = append(releaseControl.Data, &para)

	// This is a little convoluted. We'll ultimately write to this, but it may be
	// writing to unsigned and signed releases, or just the unsigned, depending
	// on user options
	var releaseWriter io.Writer
	unsignedReleaseFile, _ := store.Store()

	var signedReleaseFile StoreWriteCloser
	var signedReleaseWriter io.WriteCloser
	signerKey, err := release.SignerKey()
	if err != nil {
		return nil, err
	}

	if signerKey != nil {
		signedReleaseFile, err = store.Store()
		signedReleaseWriter, err = clearsign.Encode(signedReleaseFile, signerKey.PrivateKey, nil)
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

	if signerKey != nil {
		signedReleaseWriter.Close()
		signedReleaseFile.Close()
		release.InRelease, _ = signedReleaseFile.Identity()

		// Generate detached GPG signature
		releaseReader, _ := store.Open(release.Release)
		defer releaseReader.Close()
		releaseGpgFile, _ := store.Store()
		err = openpgp.ArmoredDetachSign(releaseGpgFile, signerKey, releaseReader, nil)
		releaseGpgFile.Close()
		release.ReleaseGPG, _ = releaseGpgFile.Identity()
	}

	// Trim the release history if requested
	if parent.Config().AutoTrim {
		trimmer := parent.Config().MakeTrimmer()
		err = release.TrimHistory(store, trimmer)
		if err != nil {
			return nil, err
		}
	}

	release.Date = time.Now()

	releaseid, err := store.AddRelease(&release)

	return releaseid, nil
}

// PoolFilePath provides the full path to the location
// in the debian apt pool for a given file, expanded using
// the pool pattern
func (r *Release) PoolFilePath(filename string) (poolpath string) {
	poolpath = "pool/" + r.CodeName + "/"

	matches := r.Config().PoolPattern.FindSubmatch([]byte(filename))
	if len(matches) > 0 {
		poolpath = poolpath + string(matches[0]) + "/"
	}

	return
}

// Config returns the ReleaseConfig  for the release
func (r *Release) Config() *ReleaseConfig {
	if r.config == nil {
		var err error
		cfg, err := r.store.GetReleaseConfig(r.ConfigID)
		log.Println(err)
		r.config = &cfg
	}

	return r.config
}

func (r *Release) SignerKey() (*openpgp.Entity, error) {
	id := r.Config().SigningKeyID
	rdr, err := r.store.Open(id)
	if err != nil {
		return nil, errors.New("failed to retrieve signing key, " + err.Error())
	}

	kr, err := openpgp.ReadArmoredKeyRing(rdr)
	if err != nil {
		return nil, errors.New("failed to retrieve signing key, " + err.Error())
	}
	if len(kr) != 0 {
		return nil, fmt.Errorf("failed to retrieve signing key, wrong number of items in keyring, %v", len(kr))
	}

	return kr[0], nil
}

func (r *Release) PubRing() (openpgp.EntityList, error) {
	return nil, nil
}
