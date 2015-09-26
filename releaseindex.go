package main

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/tcolgate/godinstall/deb"
	"github.com/tcolgate/godinstall/store"
)

// ReleaseIndexEntryItemFile repesent one file that makes up part of an
// item in the repository. A Binary item will only have one
// file (the deb package), but a Source item may have many
type ReleaseIndexEntryItemFile struct {
	Name    string // File name as it will appear in the repo
	StoreID store.ID
	Size    int64

	Md5    []byte
	Sha1   []byte
	Sha256 []byte

	SignedBy []string
}

// ReleaseIndexEntryItem represents one item, binary or source, making up
// part of a ReleaseIndexEntry
type ReleaseIndexEntryItem struct {
	Name         string
	Version      deb.Version
	Architecture string
	Component    string
	ControlID    store.ID                    // store.ID for the control data
	Files        []ReleaseIndexEntryItemFile // This list of files that make up this item
}

// ReleaseIndexEntry represents a set of packages and is
// maps closely to the changes files used to upload.
type ReleaseIndexEntry struct {
	SourceItem  ReleaseIndexEntryItem
	BinaryItems []ReleaseIndexEntryItem
	ChangesID   store.ID // store.ID for the changes data
}

// NewReleaseIndexEntry  turns an UploadSession (a collection of hash verified
// files), into an entry suitable for adding to a release index, by indeitfying
// binary items, and grouping source files into a source item.
func NewReleaseIndexEntry(u *UploadSession) (*ReleaseIndexEntry, error) {
	srcItem := ReleaseIndexEntryItem{
		Name:         u.changes.Source,
		Version:      u.changes.SourceVersion,
		Architecture: "source",
		Component:    "main",
	}

	srcFiles := []ReleaseIndexEntryItemFile{}
	binItems := []ReleaseIndexEntryItem{}

	binVersion := u.changes.BinaryVersion
	for _, f := range u.Expecting {
		rief := ReleaseIndexEntryItemFile{
			Name:     f.Name,
			StoreID:  f.storeID,
			Size:     f.Size,
			SignedBy: f.SignedBy,
		}
		switch {
		case strings.HasSuffix(f.Name, ".deb"):
			un, err := f.pkg.Name()
			if err != nil {
				return nil, fmt.Errorf(
					"reading pckage %s failed, %s",
					f.Name,
					err.Error(),
				)
			}
			uv, _ := f.pkg.Version()
			ua, _ := f.pkg.Architecture()
			uc := "main"
			ui := f.controlID

			if uv != binVersion {
				return nil, fmt.Errorf(
					"Uploaded file version for %s (%s) does not match changes file version %s",
					f.Name,
					uv.String(),
					binVersion.String(),
				)
			}

			binItems = append(binItems, ReleaseIndexEntryItem{
				Name:         un,
				Version:      uv,
				Architecture: ua,
				Component:    uc,
				ControlID:    ui,
				Files:        []ReleaseIndexEntryItemFile{rief},
			})
		case strings.HasSuffix(f.Name, ".dsc"):
			srcFiles = append(srcFiles, rief)
			srcItem.ControlID = f.controlID
		default:
			srcFiles = append(srcFiles, rief)
		}
	}

	srcItem.Files = srcFiles

	return &ReleaseIndexEntry{
		SourceItem:  srcItem,
		BinaryItems: binItems,
		ChangesID:   u.changesID,
	}, nil
}

// ByReleaseIndexEntryOrder implements sort.Interface for []ReleaseIndexEntry.
// Packages are sorted by:
//  - Alphabetical package name
//  - Reverse Version
type ByReleaseIndexEntryOrder []*ReleaseIndexEntry

func (a ByReleaseIndexEntryOrder) Len() int      { return len(a) }
func (a ByReleaseIndexEntryOrder) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByReleaseIndexEntryOrder) Less(i, j int) bool {
	res := ReleaseIndexEntryOrder(a[i], a[j])
	if res < 0 {
		return true
	}
	return false
}

// ReleaseIndexEntryOrder implements  the order we want items to appear in the index
func ReleaseIndexEntryOrder(a, b *ReleaseIndexEntry) int {
	nameCmp := bytes.Compare([]byte(a.SourceItem.Name), []byte(b.SourceItem.Name))
	if nameCmp != 0 {
		return nameCmp
	}

	// We'll use reverse order for the version, to make pruning
	// a touch easier
	debCmp := deb.VersionCompare(b.SourceItem.Version, a.SourceItem.Version)

	return debCmp
}

// ByReleaseIndexEntryItemOrder implements sort.Interface for []ReleaseIndexItemEntry.
// Packages are sorted by:
//  - Alphabetical package name
//  - Alphabetical architecture
//  - Reverse Version
type ByReleaseIndexEntryItemOrder []*ReleaseIndexEntryItem

func (a ByReleaseIndexEntryItemOrder) Len() int      { return len(a) }
func (a ByReleaseIndexEntryItemOrder) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByReleaseIndexEntryItemOrder) Less(i, j int) bool {
	res := ReleaseIndexEntryItemOrder(a[i], a[j])
	if res < 0 {
		return true
	}
	return false
}

// ReleaseIndexEntryItemOrder implements an order for individual items within
// a ReleaseIndexEntry
func ReleaseIndexEntryItemOrder(a, b *ReleaseIndexEntryItem) int {
	nameCmp := bytes.Compare([]byte(a.Name), []byte(b.Name))
	if nameCmp != 0 {
		return nameCmp
	}

	archCmp := bytes.Compare([]byte(a.Architecture), []byte(b.Architecture))
	if archCmp != 0 {
		return archCmp
	}

	// We'll use reverse order for the version, to make pruning
	// a touch easier
	debCmp := deb.VersionCompare(b.Version, a.Version)

	return debCmp
}

// ReleaseIndexWriter is used to build an index to a store
// one item at a time
type ReleaseIndexWriter interface {
	AddEntry(item *ReleaseIndexEntry) (err error)
	Close() (store.ID, error)
}

// ReleaseIndexReader is used to read an index from a store
// one item at a time
type ReleaseIndexReader interface {
	NextEntry() (item ReleaseIndexEntry, err error)
	Close() error
}

// Merge the content of index into the parent commit and return a new index
func (a archiveStoreArchive) mergeEntryIntoRelease(parentid store.ID, entry *ReleaseIndexEntry) (result store.ID, actions []ReleaseLogAction, err error) {
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

	left, err := parentidx.NextEntry()

	right := []*ReleaseIndexEntry{entry}
	sort.Sort(ByReleaseIndexEntryOrder(right))

	pruner := parent.Config().MakePruner()

	for {
		if err != nil {
			break
		}

		if len(right) > 0 {
			cmpItems := ReleaseIndexEntryOrder(&left, right[0])
			if cmpItems < 0 { // New item not needed yet
				if !pruner(&left) {
					mergedidx.AddEntry(&left)
				} else {
					actions = append(actions, ReleaseLogAction{
						Type:        ActionPRUNE,
						Description: left.SourceItem.Name + " " + left.SourceItem.Version.String(),
					})
				}
				left, err = parentidx.NextEntry()
				continue
			} else if cmpItems == 0 { // New item identical to existing
				if !pruner(&left) {
					mergedidx.AddEntry(&left)
					item := right[0]
					actions = append(actions, ReleaseLogAction{
						Type:        ActionSKIPPRESENT,
						Description: item.SourceItem.Name + " " + item.SourceItem.Version.String(),
					})
				} else {
					actions = append(actions, ReleaseLogAction{
						Type:        ActionSKIPPRUNE,
						Description: left.SourceItem.Name + " " + left.SourceItem.Version.String(),
					})
				}
				left, err = parentidx.NextEntry()
				right = right[1:]
				continue
			} else {
				item := right[0]
				if !pruner(item) {
					mergedidx.AddEntry(item)
					actions = append(actions, ReleaseLogAction{
						Type:        ActionADD,
						Description: item.SourceItem.Name + " " + item.SourceItem.Version.String(),
					})
				} else {
					actions = append(actions, ReleaseLogAction{
						Type:        ActionPRUNE,
						Description: item.SourceItem.Name + " " + item.SourceItem.Version.String(),
					})
				}
				right = right[1:]
				continue
			}
		} else {
			if !pruner(&left) {
				mergedidx.AddEntry(&left)
			} else {
				actions = append(actions, ReleaseLogAction{
					Type:        ActionPRUNE,
					Description: left.SourceItem.Name + " " + left.SourceItem.Version.String(),
				})
			}
			left, err = parentidx.NextEntry()
			continue
		}
	}

	// output any items that are left
	for _, item := range right {
		if !pruner(item) {
			mergedidx.AddEntry(item)
			actions = append(actions, ReleaseLogAction{
				Type:        ActionADD,
				Description: item.SourceItem.Name + " " + item.SourceItem.Version.String(),
			})
		} else {
			actions = append(actions, ReleaseLogAction{
				Type:        ActionPRUNE,
				Description: item.SourceItem.Name + " " + item.SourceItem.Version.String(),
			})
		}
		right = right[1:]
		continue
	}

	id, err := mergedidx.Close()
	return store.ID(id), actions, err
}
