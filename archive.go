package main

import (
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"compress/gzip"

	"code.google.com/p/go.crypto/openpgp"
)

// Archiver describes an interface for maintaining and generating
// the on disk repository
type Archiver interface {
	PublicDir() string
	Dists() map[string]StoreID
	GetDist(name string) (*Release, error)
	SetDist(name string, newrel StoreID) error
	AddSession(session UploadSessioner) (respStatus int, respObj string, err error)
	SignerID() *openpgp.Entity
	ArchiveStorer
}

// An Archiver that uses a version historied blob store
type archiveStoreArchive struct {
	PrivRing       openpgp.KeyRing // Private keyring cotaining singing key
	signerID       *openpgp.Entity // The key to sign release file with
	base           *string         // The base directory of the repository
	pruneRules     PruneRuleSet    // Rules to use for pruning the repo
	getTrimmer     func() Trimmer  // History Trimmer
	defPoolPattern string          // Default pool pattern
	ArchiveStorer                  // The blob store to use
}

// NewAptBlobArchive creates a new Archiver that uses a version
// historied content addressable store
func NewAptBlobArchive(
	privRing openpgp.KeyRing,
	signerID *openpgp.Entity,
	storeDir *string,
	tmpDir *string,
	publicDir *string,
	pruneRules PruneRuleSet,
	getTrimmer func() Trimmer,
	defPoolPattern string,
) Archiver {
	archivestore := NewArchiveBlobStore(*storeDir, *tmpDir)
	return &archiveStoreArchive{
		ArchiveStorer:  archivestore,
		PrivRing:       privRing,
		signerID:       signerID,
		base:           publicDir,
		pruneRules:     pruneRules,
		getTrimmer:     getTrimmer,
		defPoolPattern: defPoolPattern,
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

func (a *archiveStoreArchive) ReifyRelease(id StoreID) (err error) {
	release, err := a.GetRelease(id)
	if err != nil {
		return err
	}

	distBase := *a.base + "/dists/" + release.CodeName
	distAlias := *a.base + "/dists/" + release.SuiteName

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

		// Reify the compressed sources file
		/*
			sourcesBase := componentBase + "/source"

				err = a.Link(component.SourcesGz, sourcesBase+"/Sources.gz")
				if err != nil {
					return err
				}

				// Reify the uncompressed packages file
				gzreader, err := os.Open(sourcesBase + "/Sources.gz")
				defer gzreader.Close()
				if err != nil {
					return err
				}
				pkgs, err := os.Create(sourcesBase + "/Sources")
				defer pkgs.Close()
				if err != nil {
					return err
				}

				io.Copy(pkgs, gzreader)
		*/
	}

	err = a.Link(release.Release, distBase+"/Release")
	if err != nil {
		return err
	}

	if a.SignerID() != nil {
		err = a.Link(release.InRelease, distBase+"/InRelease")
		if err != nil {
			return err
		}
		err = a.Link(release.ReleaseGPG, distBase+"/Release.gpg")
		if err != nil {
			return err
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
		item, err := index.NextItem()
		if err != nil {
			if err != io.EOF {
				return err
			}
			break
		}
		for i := range item.Files {
			file := item.Files[i]
			path := a.PublicDir() + "/" + release.PoolFilePath(file.Name)
			filepath := path + file.Name
			err = a.Link(file.ID, filepath)
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

func (a *archiveStoreArchive) SignerID() *openpgp.Entity {
	return a.signerID
}

func (a *archiveStoreArchive) AddSession(session UploadSessioner) (respStatus int, respObj string, err error) {
	respStatus = http.StatusOK
	respObj = "Index committed"
	branchName := session.BranchName()

	items, err := a.ItemsFromChanges(session.Items())
	if err != nil {
		respStatus = http.StatusInternalServerError
		respObj = "Collating repository items failed, " + err.Error()
		return respStatus, respObj, err
	}

	heads := a.Dists()
	head, ok := heads[branchName]
	if !ok {
		head, err = a.GetReleaseRoot(Release{
			CodeName:    branchName,
			SuiteName:   "stable",
			PoolPattern: a.defPoolPattern,
		})
		if err != nil {
			respStatus = http.StatusInternalServerError
			respObj = "Creating distro root failed, " + err.Error()
			return respStatus, respObj, err
		}
	}

	newidx, actions, err := a.mergeItemsIntoRelease(head, items)
	if err != nil {
		respStatus = http.StatusInternalServerError
		respObj = "Creating new index failed, " + err.Error()
	}

	newhead, err := NewRelease(a, head, newidx, a.getTrimmer(), actions)
	if err != nil {
		respStatus = http.StatusInternalServerError
		respObj = "Creating updated commit failed, " + err.Error()
		return respStatus, respObj, err
	}

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
		case ActionSKIPPRUNE:
			{
				log.Println("Skipped due to prune policy: " + item.Description)
			}
		case ActionPRUNE:
			{
				log.Println("Pruned old item " + item.Description)
			}
		case ActionDELETE:
			{
				log.Println("Deleted " + item.Description)
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

	a.SetDist(branchName, newhead)
	log.Printf("Branch %v set to %v", branchName, StoreID(newhead).String())

	err = a.ReifyRelease(newhead)
	if err != nil {
		respStatus = http.StatusInternalServerError
		respObj = "Repopulating the archive directory failed," + err.Error()
		return respStatus, respObj, err
	}

	a.GarbageCollect()
	return
}

// Merge the content of index into the parent commit and return a new index
func (a archiveStoreArchive) mergeItemsIntoRelease(parentid StoreID, items []*ReleaseIndexItem) (result StoreID, actions []ReleaseLogAction, err error) {
	parent, err := a.GetRelease(parentid)
	actions = make([]ReleaseLogAction, 0)
	if err != nil {
		return nil, actions, errors.New("error getting parent commit, " + err.Error())
	}

	parentidx, err := a.OpenReleaseIndex(parent.IndexID)
	if err != nil {
		return nil, actions, errors.New("error getting parent commit index, " + err.Error())
	}
	defer parentidx.Close()

	mergedidx, err := a.AddReleaseIndex()
	if err != nil {
		return nil, actions, errors.New("error adding new index, " + err.Error())
	}

	left, err := parentidx.NextItem()

	right := items
	sort.Sort(ByReleaseIndexOrder(right))

	pruner := a.pruneRules.MakePruner()

	for {
		if err != nil {
			break
		}

		if len(right) > 0 {
			cmpItems := ReleaseIndexOrder(&left, right[0])
			if cmpItems < 0 { // New item not needed yet
				if !pruner(&left) {
					mergedidx.AddItem(&left)
				} else {
					actions = append(actions, ReleaseLogAction{
						Type:        ActionPRUNE,
						Description: left.Name + " " + left.Architecture + " " + left.Version.String(),
					})
				}
				left, err = parentidx.NextItem()
				continue
			} else if cmpItems == 0 { // New item identical to existing
				if !pruner(&left) {
					mergedidx.AddItem(&left)
					item := right[0]
					actions = append(actions, ReleaseLogAction{
						Type:        ActionSKIPPRESENT,
						Description: item.Name + " " + item.Architecture + " " + item.Version.String(),
					})
				} else {
					actions = append(actions, ReleaseLogAction{
						Type:        ActionSKIPPRUNE,
						Description: left.Name + " " + left.Architecture + " " + left.Version.String(),
					})
				}
				left, err = parentidx.NextItem()
				right = right[1:]
				continue
			} else {
				item := right[0]
				if !pruner(item) {
					mergedidx.AddItem(item)
					actions = append(actions, ReleaseLogAction{
						Type:        ActionADD,
						Description: item.Name + " " + item.Architecture + " " + item.Version.String(),
					})
				} else {
					actions = append(actions, ReleaseLogAction{
						Type:        ActionPRUNE,
						Description: item.Name + " " + item.Architecture + " " + item.Version.String(),
					})
				}
				right = right[1:]
				continue
			}
		} else {
			if !pruner(&left) {
				mergedidx.AddItem(&left)
			} else {
				actions = append(actions, ReleaseLogAction{
					Type:        ActionPRUNE,
					Description: left.Name + " " + left.Architecture + " " + left.Version.String(),
				})
			}
			left, err = parentidx.NextItem()
			continue
		}
	}

	// output any items that are left
	for _, item := range right {
		if !pruner(item) {
			mergedidx.AddItem(item)
			actions = append(actions, ReleaseLogAction{
				Type:        ActionADD,
				Description: item.Name + " " + item.Architecture + " " + item.Version.String(),
			})
		} else {
			actions = append(actions, ReleaseLogAction{
				Type:        ActionPRUNE,
				Description: item.Name + " " + item.Architecture + " " + item.Version.String(),
			})
		}
		right = right[1:]
		continue
	}

	id, err := mergedidx.Close()
	return StoreID(id), actions, err
}
