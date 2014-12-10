package main

import "bytes"

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

// A ReleaseIndexItem is either deb, or a dsc describing
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

// ByReleaseIndexOrder implements sort.Interface for []ReleaseIndexItem.
// Packages are sorted by:
//  - Alphabetical package name
//  - Alphabetical architecture
//  - Reverse Version
type ByReleaseIndexOrder []*ReleaseIndexItem

func (a ByReleaseIndexOrder) Len() int      { return len(a) }
func (a ByReleaseIndexOrder) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByReleaseIndexOrder) Less(i, j int) bool {
	res := ReleaseIndexOrder(a[i], a[j])
	if res < 0 {
		return true
	}
	return false
}

// ReleaseIndexOrder implements  the order we want items to appear in the index
func ReleaseIndexOrder(a, b *ReleaseIndexItem) int {
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
