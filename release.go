package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tcolgate/godinstall/deb"
	"github.com/tcolgate/godinstall/hasher"
	"github.com/tcolgate/godinstall/store"

	"compress/gzip"

	"code.google.com/p/go.crypto/openpgp"
	"code.google.com/p/go.crypto/openpgp/clearsign"
)

// Architecture collates all the information for an architecture
// within a component of a release.
type Architecture struct {
	Name       string
	PackagesGz store.ID
}

// Component collates all the information for a component
// within a release
type Component struct {
	Name          string
	Architectures []Architecture
	SourcesGz     store.ID
}

// Release collects all the information for a release
type Release struct {
	ParentID    store.ID
	IndexID     store.ID
	CodeName    string
	Suite       string
	Description string
	Version     string
	Date        time.Time
	Components  []Component
	InRelease   store.ID
	Release     store.ID
	ReleaseGPG  store.ID
	Actions     []ReleaseLogAction
	TrimAfter   int32
	ConfigID    store.ID

	store  ArchiveStorer
	id     store.ID
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
	ActionUNKNOWN      ReleaseLogActionType = 1 << iota
	ActionADD          ReleaseLogActionType = 2
	ActionDELETE       ReleaseLogActionType = 3
	ActionPRUNE        ReleaseLogActionType = 4
	ActionSKIPPRESENT  ReleaseLogActionType = 5
	ActionSKIPPRUNE    ReleaseLogActionType = 6
	ActionTRIM         ReleaseLogActionType = 7
	ActionCONFIGCHANGE ReleaseLogActionType = 8
)

// ReleaseLogAction desribes an action taken during a merge or update
type ReleaseLogAction struct {
	Type        ReleaseLogActionType
	Description string
}

type archTempData struct {
	packagesFileWriter   *hasher.Hasher
	packagesGzFile       *hasher.Hasher
	packagesGzStore      store.StoreWriteCloser
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
	PackagesGzID         store.ID
}

type compTempData struct {
	archs               map[string]*archTempData
	sourcesFileWriter   *hasher.Hasher
	sourcesGzFile       *hasher.Hasher
	sourcesGzStore      store.StoreWriteCloser
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
	SourcesGzID         store.ID
}

type relTempData map[string]*compTempData

// Parent returns the Release this Reelase was built from
func (r *Release) Parent() (*Release, error) {
	return r.store.GetRelease(r.id)
}

// NewChild return a new Release
func (r *Release) NewChild() *Release {
	c := *r
	c.ParentID = r.id
	c.Date = time.Now()

	ver, err := strconv.ParseUint(r.Version, 10, 64)
	if err == nil {
		ver++
		c.Version = strconv.FormatUint(ver, 10)
	}

	return &c
}

func (r *Release) updateReleaseSigFiles() bool {
	p, err := r.Parent()
	if err != nil {
		log.Printf("signature files update failed, %v", err)
		return false
	}

	prelid := p.Release
	relid := r.Release

	pkey, err := p.SignerKey()
	if err != nil {
		log.Printf("signature files update failed, %v", err)
		return false
	}
	key, err := r.SignerKey()
	if err != nil {
		log.Printf("signature files update failed, %v", err)
		return false
	}

	if key != pkey || relid.String() != prelid.String() {
		if key == nil {
			r.ReleaseGPG = store.ID("")
			r.InRelease = store.ID("")
			return true
		}

		rd, err := r.store.Open(r.Release)
		if err != nil {
			log.Printf("InRelease clear-signer, %v", err)
			return false
		}
		defer rd.Close()
		win, err := r.store.Store()
		if err != nil {
			log.Printf("InRelease clear-signer, %v", err)
			return false
		}
		wplain, err := clearsign.Encode(win, key.PrivateKey, nil)
		if err != nil {
			log.Printf("InRelease clear-signer, %v", err)
			return false
		}
		_, err = io.Copy(wplain, rd)
		if err != nil {
			log.Printf("InRelease clear-signer, %v", err)
			return false
		}
		wplain.Close()
		win.Close()

		rd, _ = r.store.Open(r.Release)
		defer rd.Close()

		wgpg, _ := r.store.Store()
		if err != nil {
			log.Printf("Release.gpg detacked signer, %v", err)
			return false
		}
		err = openpgp.ArmoredDetachSign(wgpg, key, rd, nil)
		wgpg.Close()

		r.InRelease, _ = win.Identity()
		r.ReleaseGPG, _ = wgpg.Identity()
		return true
	}

	return false
}

// updateReleaseSigFiles regenerates the Release, Packages and Sources files.
// this needs breaking up a bit
func (r *Release) updateReleasefiles() {
	// We want to merge all packages of arch all into each binary-$arch
	// so we walk the package index in a first pass to find all the archs
	// we are dealing with, we'll setup the temporary data to track the
	// packages files while we are at it.
	relMap := make(relTempData, 0)
	archMap := make(map[string]bool, 0)
	preIndex, err := r.store.OpenReleaseIndex(r.IndexID)
	if err != nil {
		log.Printf("failed to update releases, %v", err)
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
				comp.sourcesFileWriter = hasher.New(ioutil.Discard)
				comp.sourcesGzStore, err = r.store.Store()
				if err != nil {
					log.Printf("failed to update releases, %v", err)
					return
				}
				comp.sourcesGzFile = hasher.New(comp.sourcesGzStore)
				comp.sourcesGzFileWriter = gzip.NewWriter(comp.sourcesGzFile)
				comp.sourcesWriter = io.MultiWriter(comp.sourcesFileWriter, comp.sourcesGzFileWriter)
				comp.archs = make(map[string]*archTempData, 0)
			}

			arch, ok := comp.archs[archName]
			if !ok {
				// This is a new arch in this component
				arch = new(archTempData)

				arch.packagesFileWriter = hasher.New(ioutil.Discard)
				arch.packagesGzStore, err = r.store.Store()
				if err != nil {
					log.Printf("failed to update releases, %v", err)
				}
				arch.packagesGzFile = hasher.New(arch.packagesGzStore)
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
	index, err := r.store.OpenReleaseIndex(r.IndexID)
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
			r.PoolFilePath(srcName),
			srcName,
			srcVersion,
		)

		if len(s.ControlID) != 0 {
			srcCtrl, err := r.store.GetControlFile(s.ControlID)
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

			deb.FormatDpkgControlFile(srcComp.sourcesWriter, srcCtrl)
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
			control, err := r.store.GetControlFile(b.ControlID)
			if err != nil {
				log.Println("Could not retrieve control data, " + err.Error())
				continue
			}
			control.Data[0].SetValue("Filename", path)

			deb.FormatDpkgControlFile(arch.packagesWriter, control)
			arch.packagesWriter.Write([]byte("\n"))

			if archName == "all" {
				for _, otherArchName := range archNames {
					otherArch, _ := comp.archs[otherArchName]
					deb.FormatDpkgControlFile(otherArch.packagesWriter, control)
					otherArch.packagesWriter.Write([]byte("\n"))
				}
			}
		}
	}

	r.Components = make([]Component, 0)

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
		c.sourcesGzSize, _ = r.store.Size(c.SourcesGzID)

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
			archFiles.packagesGzSize, _ = r.store.Size(archFiles.PackagesGzID)

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
		r.Components = append(r.Components, comp)
	}

	releaseControl := deb.ControlFile{}
	para := deb.MakeControlParagraph()

	releaseStartFields := []string{"Origin", "Suite", "Codename"}
	releaseEndFields := []string{"SHA256"}
	para.SetValue("Origin", "GoDInstall")
	para.SetValue("Suite", r.Suite)
	para.SetValue("Codename", r.CodeName)

	archNames = append(archNames, "all")
	archNames.Sort()
	para.SetValue("Architectures", strings.Join(archNames, " "))
	para.SetValue("Components", strings.Join(compNames, " "))
	para.AddValue("MD5Sum", "")

	for i := range r.Components {
		comp := r.Components[i]
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

		for j := range r.Components[i].Architectures {
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

	releaseWriter, _ := r.store.Store()
	deb.WriteControl(releaseWriter, releaseControl, releaseStartFields, releaseEndFields)
	releaseWriter.Close()
	r.Release, _ = releaseWriter.Identity()

	r.updateReleaseSigFiles()
}

// NewRelease creates a new release object in the specified store, based on the
// parent and built using the passed in index, and associated set of
// actions
func NewRelease(store Archiver, parentid store.ID, indexid store.ID, actions []ReleaseLogAction) (id store.ID, err error) {
	parent, err := store.GetRelease(parentid)
	if err != nil {
		return nil, err
	}
	release := parent.NewChild()
	release.IndexID = indexid
	release.Actions = actions

	release.updateReleasefiles()

	// Trim the release history if requested
	if release.Config().AutoTrim {
		trimmer := release.Config().MakeTrimmer()
		err = release.TrimHistory(store, trimmer)
		if err != nil {
			return nil, err
		}
	}

	releaseid, err := store.AddRelease(release)

	return releaseid, nil
}

// PoolFilePath provides the full path to the location
// in the debian apt pool for a given file, expanded using
// the pool pattern
func (r *Release) PoolFilePath(filename string) (poolpath string) {
	poolpath = "pool/" + r.CodeName + "/"

	re := r.Config().PoolRegexp()

	matches := re.FindSubmatch([]byte(filename))
	if len(matches) > 0 {
		poolpath = poolpath + string(matches[0]) + "/"
	}

	return
}

// Config returns the ReleaseConfig  for the release
func (r *Release) Config() *ReleaseConfig {
	if r.config == nil {
		cfg, _ := r.store.GetReleaseConfig(r.ConfigID)
		r.config = &cfg
	}

	return r.config
}

// SignerKey returns the key that will be used to sign
// this release
func (r *Release) SignerKey() (*openpgp.Entity, error) {
	id := r.Config().SigningKeyID
	if len(id) == 0 {
		return nil, nil
	}

	rdr, err := r.store.Open(id)
	if err != nil {
		return nil, errors.New("failed to retrieve signing key, " + err.Error())
	}
	defer rdr.Close()

	kr, err := openpgp.ReadArmoredKeyRing(rdr)
	if err != nil {
		return nil, errors.New("failed to retrieve signing key, " + err.Error())
	}
	if len(kr) != 1 {
		return nil, fmt.Errorf("failed to retrieve signing key, wrong number of items in keyring, %v", len(kr))
	}

	return kr[0], nil
}

// PubRing returns the set of public keys of the people
// permitted to upload packages
func (r *Release) PubRing() (openpgp.EntityList, error) {
	var kr openpgp.EntityList
	ids := r.Config().PublicKeyIDs
	if len(ids) == 0 {
		return nil, nil
	}

	for _, id := range ids {
		rdr, err := r.store.Open(id)
		if err != nil {
			return nil, errors.New("failed to retrieve signing key, " + err.Error())
		}
		defer rdr.Close()

		subkr, err := openpgp.ReadArmoredKeyRing(rdr)
		if err != nil {
			return nil, errors.New("failed to retrieve signing key, " + err.Error())
		}
		if len(subkr) != 1 {
			return nil, fmt.Errorf("failed to retrieve signing key, wrong number of items in keyring, %v", len(kr))
		}

		kr = append(kr, subkr[0])
	}

	return kr, nil
}
