package main

import (
	"encoding/gob"
	"io"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"
)

type RepoStorer interface {
	GetHead(string) (CommitID, error)
	SetHead(string, CommitID) error
	AddDeb(file *ChangesItem) (*RepoItem, error)
	GetCommit(id CommitID) (*RepoCommit, error)
	AddCommit(data *RepoCommit) (CommitID, error)
	AddBinaryControlFile(data ControlData) (StoreID, error)
	GetBinaryControlFile(id StoreID) (ControlData, error)
	AddIndex() (h repoIndexWriterHandle, err error)
	EmptyIndex() (IndexID, error)
	OpenIndex(id IndexID) (h repoIndexReaderHandle, err error)
	ItemsFromChanges(files map[string]*ChangesItem) ([]*RepoItem, error)
	MergeItemsIntoCommit(parentid CommitID, items []*RepoItem) (result IndexID, err error)
	Storer
}

type repoBlobStore struct {
	Storer
}

func NewRepoBlobStore(storeDir string, tmpDir string) RepoStorer {
	return &repoBlobStore{
		Sha1Store(storeDir, tmpDir, 3),
	}
}

func (r repoBlobStore) GetHead(name string) (CommitID, error) {
	id, err := r.GetRef("heads/" + name)
	return CommitID(id), err
}

func (r repoBlobStore) SetHead(name string, id CommitID) error {
	return r.SetRef("heads/"+name, StoreID(id))
}

type RepoItemType int

const (
	UNKNOWN RepoItemType = 1 << iota
	BINARY  RepoItemType = 2
	SOURCE  RepoItemType = 3
)

// A RepoItem is either deb, or a dsc describing
// a set of files for a source archive
type RepoItem struct {
	Type         RepoItemType // The type of file
	Name         string
	Version      DebVersion
	Architecture string
	ControlID    StoreID        // StoreID for teh control data
	Files        []RepoItemFile // This list of files that make up this item
}

// RepoItemFile repesent one file that makes up part of an
// item in the repository. A Binary item will only have one
// file (the deb package), but a Source item may have many
type RepoItemFile struct {
	Name string  // File name as it will appear in the repo
	ID   StoreID // Store ID for the actual file
}

func (r repoBlobStore) AddDeb(file *ChangesItem) (*RepoItem, error) {
	var item RepoItem
	item.Type = BINARY

	pkgReader, err := r.Open(file.StoreID)
	if err != nil {
		return nil, err
	}

	pkg := NewDebPackage(pkgReader, nil)
	err = pkg.Parse()
	if err != nil {
		return nil, err
	}

	control, _ := pkg.Control()
	arch, ok := control["Architecture"]
	if !ok {
		arch = "all"
	}
	item.Architecture = arch

	control["Filename"] = file.Filename
	control["Size"] = strconv.FormatInt(file.Size, 10)
	control["MD5sum"] = file.Md5
	control["SHA1"] = file.Sha1
	control["SHA256"] = file.Sha256

	paragraphs := make(ControlData, 1)
	paragraphs[0] = control

	item.ControlID, err = r.AddBinaryControlFile(paragraphs)
	if err != nil {
		return nil, err
	}

	item.Version, _ = pkg.Version()

	fileSlice := make([]RepoItemFile, 1)
	fileSlice[0] = RepoItemFile{
		Name: file.Filename,
		ID:   file.StoreID,
	}
	item.Files = fileSlice

	return &item, nil
}

func (r repoBlobStore) ItemsFromChanges(files map[string]*ChangesItem) ([]*RepoItem, error) {
	var err error

	// Build repository items
	result := make([]*RepoItem, 0)
	for i, file := range files {
		switch {
		case strings.HasSuffix(i, ".deb"):
			item, err := r.AddDeb(file)
			if err != nil {
				log.Println(err)
				return nil, err
			}
			result = append(result, item)

			//case strings.HasSuffix(i, ".dsc"):
			//	var item RepoItemBinary
			//	result = append(result, &item)
		}
	}

	if err != nil {
		return nil, err
	}

	return result, nil
}

// We can't serialize a map, as the key order is not
// guaranteed, which will result in inconsistant
// hashes for the same data.
type consistantControlData []struct {
	Keys   []string
	Values []string
}

func (r repoBlobStore) GetDebianControlFile(id StoreID) (ControlData, error) {
	reader, err := r.Open(id)
	if err != nil {
		return nil, err
	}

	dec := gob.NewDecoder(reader)
	var item consistantControlData
	err = dec.Decode(&item)
	if err != nil {
		return nil, err
	}

	result := make(ControlData, len(item))
	for i := range item {
		result[i] = make(map[string]string, 0)
		for j := range item[i].Keys {
			result[i][item[i].Keys[j]] = item[i].Values[j]
		}
	}

	return result, nil
}

func (r repoBlobStore) AddDebianControlFile(item ControlData) (StoreID, error) {
	data := make(consistantControlData, len(item))

	for i := range item {
		data[i].Keys = make([]string, len(item[i]))
		data[i].Values = make([]string, len(item[i]))

		j := 0
		for s := range item[i] {
			data[i].Keys[j] = s
			j++
		}
		sort.Strings(data[i].Keys)

		for j = range data[i].Keys {
			key := data[i].Keys[j]
			data[i].Values[j] = item[i][key]
		}
	}

	writer, err := r.Store()
	if err != nil {
		return nil, err
	}
	enc := gob.NewEncoder(writer)

	err = enc.Encode(data)
	if err != nil {
		return nil, err
	}

	writer.Close()
	id, err := writer.Identity()
	if err != nil {
		return nil, err
	}

	return id, nil
}

func (r repoBlobStore) GetBinaryControlFile(id StoreID) (ControlData, error) {
	return r.GetDebianControlFile(id)
}

func (r repoBlobStore) AddBinaryControlFile(data ControlData) (StoreID, error) {
	return r.AddDebianControlFile(data)
}

// ByIndexOrder implements sort.Interface for []RepoItem.
// Packages are sorted by:
//  - Alphabetical package name
//  - Alphabetical architecture
//  - Reverse Version

type ByIndexOrder []*RepoItem

func (a ByIndexOrder) Len() int      { return len(a) }
func (a ByIndexOrder) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByIndexOrder) Less(i, j int) bool {
	res := IndexOrder(a[i], a[j])
	if res < 0 {
		return true
	} else {
		return false
	}
}

func IndexOrder(a, b *RepoItem) int {
	switch {
	case a.Name < b.Name:
		return -1
	case a.Name == b.Name &&
		a.Architecture < b.Architecture:
		return -1
	case a.Name == b.Name &&
		a.Architecture == b.Architecture &&
		DebVersionCompare(a.Version, b.Version) < 0:
		return -1
	case a.Name == b.Name &&
		a.Architecture == b.Architecture &&
		DebVersionCompare(a.Version, b.Version) == 0:
		return 0
	default:
		return 1
	}
}

// Used for tracking the state of reads from an Index
type repoIndexWriterHandle struct {
	handle  StoreWriteCloser
	encoder *gob.Encoder
}

type IndexID StoreID

// RepoIndex represent a complete list of packages that will make
// up a full release.
func (r repoBlobStore) AddIndex() (h repoIndexWriterHandle, err error) {
	h.handle, err = r.Store()
	if err != nil {
		return
	}

	h.encoder = gob.NewEncoder(h.handle)
	return
}

func (r *repoIndexWriterHandle) AddItem(item *RepoItem) (err error) {
	err = r.encoder.Encode(item)
	return
}

func (r *repoIndexWriterHandle) Close() (IndexID, error) {
	err := r.handle.Close()
	if err != nil {
		return nil, err
	}

	id, err := r.handle.Identity()
	return IndexID(id), err
}

func (r repoBlobStore) EmptyIndex() (id IndexID, err error) {
	idx, err := r.AddIndex()
	return idx.Close()
}

type repoIndexReaderHandle struct {
	handle  io.Reader
	decoder *gob.Decoder
}

func (r repoBlobStore) OpenIndex(id IndexID) (h repoIndexReaderHandle, err error) {
	h.handle, err = r.Open(StoreID(id))
	if err != nil {
		return
	}

	h.decoder = gob.NewDecoder(h.handle)
	return
}

func (r repoIndexReaderHandle) NextItem() (item RepoItem, err error) {
	err = r.decoder.Decode(&item)
	return
}

type RepoActionType int

const (
	ActionUNKNOWN RepoActionType = 1 << iota
	ActionADD     RepoActionType = 2
	ActionDELETE  RepoActionType = 3
	ActionPURGE   RepoActionType = 4
)

// This lists the actions that were taken during a commit
type RepoAction struct {
	Type        RepoActionType
	Description string
}

type CommitID StoreID

// RepoCommit represents an actual complete repository state
type RepoCommit struct {
	Parent      CommitID     // The previous commit we updated
	Date        time.Time    // Time of the update
	Index       IndexID      // The item index in the blob store
	PoolPattern string       // The pool pattern we used to reify the index files
	Packages    StoreID      // StoreID for reified binary index
	PackagesGz  StoreID      // StoreID for reified compressed binary index
	Sources     StoreID      // StoreID for reified source index
	SourcesGz   StoreID      // StoreID for reified compressed source index
	Release     StoreID      // StoreID for reified release file
	InRelease   StoreID      // StoreID for reified signed release file
	Actions     []RepoAction // List of all actions that were performed for this commit
}

func (r repoBlobStore) GetCommit(id CommitID) (*RepoCommit, error) {
	var commit RepoCommit
	reader, err := r.Open(StoreID(id))
	if err != nil {
		return nil, err
	}

	dec := gob.NewDecoder(reader)
	dec.Decode(&commit)

	return &commit, nil
}

func (r repoBlobStore) AddCommit(data *RepoCommit) (CommitID, error) {
	writer, err := r.Store()
	if err != nil {
		return nil, err
	}
	enc := gob.NewEncoder(writer)

	enc.Encode(data)
	writer.Close()
	id, err := writer.Identity()
	if err != nil {
		return nil, err
	}

	return CommitID(id), nil
}

// Merge the content of index into the parent commit and return a new index
func (r repoBlobStore) MergeItemsIntoCommit(parentid CommitID, items []*RepoItem) (result IndexID, err error) {
	sort.Sort(ByIndexOrder(items))
	parent, _ := r.GetCommit(parentid)
	parentidx, _ := r.OpenIndex(parent.Index)
	mergedidx, _ := r.AddIndex()

	for {
		item, err := parentidx.NextItem()
		if err != nil {
			break
		}
		if len(items) > 0 {
			if IndexOrder(&item, items[0]) < 0 { // New item not in index
				mergedidx.AddItem(&item)
				break
			} else if IndexOrder(&item, items[0]) == 0 { // New item identical to existing
				// should do more checks here
				mergedidx.AddItem(items[0])
				items = items[1:]
				continue
			} else {
				mergedidx.AddItem(items[0])
				items = items[1:]
			}
		}
	}

	// output any items that are left
	for i := range items {
		mergedidx.AddItem(items[i])
	}

	id, err := mergedidx.Close()
	return IndexID(id), err
}
