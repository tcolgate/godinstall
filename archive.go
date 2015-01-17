package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"compress/gzip"
)

// Archiver describes an interface for maintaining and generating
// the on disk repository
type Archiver interface {
	PublicDir() string
	Dists() map[string]StoreID
	GetDist(name string) (*Release, error)
	SetDist(name string, newrel StoreID) error
	ReifyRelease(id StoreID) (err error)
	DeleteDist(name string) error
	AddUpload(session *UploadSession) error
	ArchiveStorer
}

// An Archiver that uses a version historied blob store
type archiveStoreArchive struct {
	base          *string // The base directory of the repository
	ArchiveStorer         // The blob store to use
}

// NewAptBlobArchive creates a new Archiver that uses a version
// historied content addressable store
func NewAptBlobArchive(
	storeDir *string,
	tmpDir *string,
	publicDir *string,
	defConfig ReleaseConfig,
) Archiver {
	archivestore := NewArchiveBlobStore(*storeDir, *tmpDir, defConfig)

	return &archiveStoreArchive{
		ArchiveStorer: archivestore,
		base:          publicDir,
	}
}

func (a *archiveStoreArchive) Dists() map[string]StoreID {
	tags := a.ReleaseTags()
	dists := make(map[string]StoreID, 0)
	for tag := range tags {
		if !strings.HasPrefix(tag, "heads/") {
			continue
		}
		tagSuffix := strings.TrimPrefix(tag, "heads/")
		if strings.Index(tagSuffix, "/") != -1 {
			continue
		}
		dists[tagSuffix] = tags[tag]
	}
	return dists
}

func (a *archiveStoreArchive) GetDist(name string) (*Release, error) {
	if strings.Index(name, "/") != -1 {
		return nil, errors.New("Distribution name cannot include /")
	}

	releaseID, err := a.GetReleaseTag("heads/" + name)
	if err != nil {
		return nil, err
	}
	release, err := a.GetRelease(releaseID)
	if err != nil {
		return nil, err
	}

	return release, nil
}

func (a *archiveStoreArchive) SetDist(name string, newrel StoreID) error {
	if strings.Index(name, "/") != -1 {
		return errors.New("Distribution name cannot include /")
	}
	return a.SetReleaseTag("heads/"+name, newrel)
}

func (a *archiveStoreArchive) DeleteDist(name string) error {
	if strings.Index(name, "/") != -1 {
		return errors.New("Distribution name cannot include /")
	}

	err := a.DeleteReleaseTag("heads/" + name)
	if err != nil {
		return fmt.Errorf("Distribution note deleted, %v", err.Error())
	}

	return os.RemoveAll(*a.base + "/dists/" + name)
}

func (a *archiveStoreArchive) ReifyRelease(id StoreID) (err error) {
	release, err := a.GetRelease(id)
	if err != nil {
		return err
	}

	distBase := *a.base + "/dists/" + release.CodeName
	distAlias := *a.base + "/dists/" + release.Suite

	clearTime := time.Now()
	clearDist := func() {
		os.Remove(distAlias)
		os.RemoveAll(distBase)
	}
	clearDist()
	clearDuration := time.Since(clearTime)
	log.Println("Cleared old distribution in ", clearDuration)

	defer func() {
		if err != nil {
			clearDist()
		}
	}()

	reifyTime := time.Now()
	fileCount := 0
	log.Printf("Reifying release %v", release.CodeName)

	for _, component := range release.Components {
		log.Printf("Reifying component %v", component.Name)

		componentBase := distBase + "/" + component.Name

		// Reify the compressed sources file
		sourcesBase := componentBase + "/source"

		err = a.Link(component.SourcesGz, sourcesBase+"/Sources.gz")
		if err != nil {
			return err
		}

		// Reify the uncompressed sources file
		gzreader, err := os.Open(sourcesBase + "/Sources.gz")
		defer gzreader.Close()
		if err != nil {
			return err
		}
		srcs, err := os.Create(sourcesBase + "/Sources")
		defer srcs.Close()
		if err != nil {
			return err
		}
		gunzipper, err := gzip.NewReader(gzreader)
		defer gunzipper.Close()
		if err != nil {
			return err
		}

		io.Copy(srcs, gunzipper)

		for _, arch := range component.Architectures {
			archBase := componentBase + "/binary-" + arch.Name

			// Reify the compressed packages file
			err = a.Link(arch.PackagesGz, archBase+"/Packages.gz")
			if err != nil {
				return err
			}

			// Reify the uncompressed packages file
			gzreader, err := os.Open(archBase + "/Packages.gz")
			defer gzreader.Close()
			if err != nil {
				return err
			}
			pkgs, err := os.Create(archBase + "/Packages")
			defer pkgs.Close()
			if err != nil {
				return err
			}
			gunzipper, err := gzip.NewReader(gzreader)
			defer gunzipper.Close()
			if err != nil {
				return err
			}
			io.Copy(pkgs, gunzipper)
		}

	}

	err = a.Link(release.Release, distBase+"/Release")
	if err != nil {
		return err
	}

	if len(release.InRelease) != 0 {
		err = a.Link(release.InRelease, distBase+"/InRelease")
		if err != nil {
			return err
		}
		err = a.Link(release.ReleaseGPG, distBase+"/Release.gpg")
		if err != nil {
			return err
		}

		key, _ := release.SignerKey()
		if key != nil {
			keyfile, err := os.Create(distBase + "/pubkey.gpg")
			defer keyfile.Close()
			if err == nil {
				key.Serialize(keyfile)
			}
		}
	}

	err = a.updatePool(release)
	if err != nil {
		return err
	}

	reifyDuration := time.Since(reifyTime)
	log.Printf("Reified %v files in %v ", fileCount, reifyDuration)

	return
}

func (a *archiveStoreArchive) updatePool(release *Release) error {
	index, err := a.OpenReleaseIndex(release.IndexID)
	defer index.Close()
	if err != nil {
		return err
	}

	poolBase := a.PublicDir() + "/pool/" + release.CodeName
	log.Printf("Clearing pool %v ", poolBase)
	os.RemoveAll(poolBase)

	for {
		e, err := index.NextEntry()
		if err != nil {
			if err != io.EOF {
				return err
			}
			break
		}

		srcName := e.SourceItem.Name
		srcVersion := e.SourceItem.Version.String()
		poolpath := fmt.Sprintf("%s/%s%s/%s/",
			a.PublicDir(),
			release.PoolFilePath(srcName),
			srcName,
			srcVersion,
		)

		err = a.Link(e.ChangesID, poolpath+"changes")
		if err != nil {
			return err
		}

		for _, s := range e.SourceItem.Files {
			filename := s.Name
			path := poolpath + filename
			err = a.Link(s.StoreID, path)
			if err != nil {
				return err
			}
		}

		for _, b := range e.BinaryItems {
			filename := b.Files[0].Name
			path := poolpath + filename
			err = a.Link(b.Files[0].StoreID, path)
			if err != nil {
				return err
			}
		}
	}
	log.Printf("Pool rebuild complete")
	return nil
}

// Return the raw path to the base directory, used for directly
// serving content
func (a *archiveStoreArchive) PublicDir() string {
	return *a.base
}

func (a *archiveStoreArchive) AddUpload(session *UploadSession) error {

	entry, err := NewReleaseIndexEntry(session)
	if err != nil {
		return fmt.Errorf("Collating repository items failed, %v", err)
	}

	branchName := session.ReleaseName
	heads := a.Dists()
	head, ok := heads[branchName]
	if !ok {
		defCfgID, err := a.GetDefaultReleaseConfigID()
		if err != nil {
			return fmt.Errorf("Creating distro root failed getting default config ID for release, %v", err)
		}
		head, err = a.GetReleaseRoot(Release{
			CodeName: branchName,
			Suite:    "stable",
			ConfigID: defCfgID,
		})
		if err != nil {
			return fmt.Errorf("Creating distro root failed, %v", err)
		}
	}

	newidx, actions, err := a.mergeEntryIntoRelease(head, entry)
	if err != nil {
		return fmt.Errorf("Creating new index failed, %v", err)
	}

	realchange := false
	for _, item := range actions {
		switch item.Type {
		case ActionADD:
			{
				log.Println("Added " + item.Description)
				realchange = true
			}
		case ActionSKIPPRESENT:
			{
				log.Println("Item already present: " + item.Description)
			}
		case ActionSKIPPRUNE:
			{
				log.Println("Skipped due to prune policy: " + item.Description)
			}
		case ActionPRUNE:
			{
				log.Println("Pruned old item " + item.Description)
				realchange = true
			}
		case ActionDELETE:
			{
				log.Println("Deleted " + item.Description)
				realchange = true
			}
		case ActionTRIM:
			{
				log.Println("Trimmed " + item.Description)
			}
		default:
			{
				log.Println(item.Description)
			}
		}
	}

	if !realchange {
		log.Println("No changes to index to cmmit")
		return nil
	}

	newhead, err := NewRelease(a, head, newidx, actions)
	if err != nil {
		return fmt.Errorf("Creating updated commit failed, %v", err)
	}

	if err = a.SetDist(branchName, newhead); err != nil {
		return fmt.Errorf("Setting dist ref failed, %v", err)
	}
	log.Printf("Branch %v set to %v", branchName, StoreID(newhead).String())

	if err = a.ReifyRelease(newhead); err != nil {
		return fmt.Errorf("Repopulating the archive directory failed,, %v", err)
	}

	a.GarbageCollect()
	return nil
}
