package main

import "bytes"

// ReleaseIndexEntry represents a set of packages and is
// maps closely to the changes files used to upload.
type ReleaseIndexEntry struct {
	Name        string
	Version     DebVersion
	SourceItem  ReleaseIndexItem
	BinaryItems []ReleaseIndexItem
	ChangesID   StoreID // StoreID for the changes data
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

// ReleaseIndexOrder implements  the order we want items to appear in the index
func ReleaseIndexEntryOrder(a, b *ReleaseIndexEntry) int {
	nameCmp := bytes.Compare([]byte(a.Name), []byte(b.Name))
	if nameCmp != 0 {
		return nameCmp
	}

	// We'll use reverse order for the version, to make pruning
	// a touch easier
	debCmp := DebVersionCompare(b.Version, a.Version)

	return debCmp
}

// ReleaseIndexItemType is used to differentiate source and binary repository items
type ReleaseIndexItemType int

// An uninitialised repo item
// A binary item (a deb)
// a source item (dsc, and related files)
const (
	UNKNOWN ReleaseIndexItemType = 1 << iota
	BINARY  ReleaseIndexItemType = 2
	SOURCE  ReleaseIndexItemType = 3
)

// ReleaseIndexItem is either deb, or a dsc describing
// a set of files for a source archive
type ReleaseIndexItem struct {
	Type         ReleaseIndexItemType // The type of file
	Name         string
	Version      DebVersion
	Component    string
	Architecture string
	ControlID    StoreID                // StoreID for teh control data
	Files        []ReleaseIndexItemFile // This list of files that make up this item
}

// ReleaseIndexItemFile repesent one file that makes up part of an
// item in the repository. A Binary item will only have one
// file (the deb package), but a Source item may have many
type ReleaseIndexItemFile struct {
	Name string  // File name as it will appear in the repo
	ID   StoreID // Store ID for the actual file
}

// ByReleaseIndexItemOrder implements sort.Interface for []ReleaseIndexItem.
// Packages are sorted by:
//  - Alphabetical package name
//  - Alphabetical architecture
//  - Reverse Version
type ByReleaseIndexItemOrder []*ReleaseIndexItem

func (a ByReleaseIndexItemOrder) Len() int      { return len(a) }
func (a ByReleaseIndexItemOrder) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByReleaseIndexItemOrder) Less(i, j int) bool {
	res := ReleaseIndexItemOrder(a[i], a[j])
	if res < 0 {
		return true
	}
	return false
}

// ReleaseIndexOrder implements  the order we want items to appear in the index
func ReleaseIndexItemOrder(a, b *ReleaseIndexItem) int {
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
	debCmp := DebVersionCompare(b.Version, a.Version)

	return debCmp
}

// ReleaseIndexWriter is used to build an index to a store
// one item at a time
type ReleaseIndexWriter interface {
	AddItem(item *ReleaseIndexItem) (err error)
	Close() (StoreID, error)
}

// ReleaseIndexReader is used to read an index from a store
// one item at a time
type ReleaseIndexReader interface {
	NextItem() (item ReleaseIndexItem, err error)
	Close() error
}
