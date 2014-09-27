package main

import (
	"bytes"
	"encoding/gob"
	"errors"
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
	AddBinaryControlFile(data ControlFile) (StoreID, error)
	GetBinaryControlFile(id StoreID) (ControlFile, error)
	AddIndex() (h repoIndexWriterHandle, err error)
	EmptyIndex() (IndexID, error)
	OpenIndex(id IndexID) (h repoIndexReaderHandle, err error)
	ItemsFromChanges(files map[string]*ChangesItem) ([]*RepoItem, error)
	MergeItemsIntoCommit(parentid CommitID, items []*RepoItem) (result IndexID, err error)
	GarbageCollect()
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

func (r repoBlobStore) gcWalkIndex(used *SafeMap, id IndexID) {
	used.Set(StoreID(id).String(), true)
	index, _ := r.OpenIndex(id)
	for {
		item, err := index.NextItem()
		if err != nil {
			break
		}
		ctrlid := item.ControlID
		used.Set(ctrlid.String(), true)
		for _, f := range item.Files {
			used.Set(f.ID.String(), true)
		}
	}
	index.Close()
}

func (r repoBlobStore) gcWalkCommit(used *SafeMap, id CommitID) {
	used.Set(StoreID(id).String(), true)
	commit, _ := r.GetCommit(id)
	used.Set(commit.InRelease.String(), true)
	used.Set(commit.Release.String(), true)
	used.Set(commit.Packages.String(), true)
	used.Set(commit.PackagesGz.String(), true)
	used.Set(commit.Sources.String(), true)
	used.Set(commit.SourcesGz.String(), true)

	r.gcWalkIndex(used, commit.Index)

	if StoreID(commit.Parent).String() != r.EmptyFileID().String() {
		r.gcWalkCommit(used, commit.Parent)
	}
}

func (r repoBlobStore) GarbageCollect() {
	used := NewSafeMap()
	refs := r.ListRefs()

	for _, id := range refs {
		r.gcWalkCommit(used, CommitID(id))
	}

	f := func(id StoreID) {
		if !used.Check(id.String()) {
			log.Println("Removing unused blob ", id.String())
			r.UnLink(id)
		}
	}

	r.ForEach(f)

	return
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
	defer pkgReader.Close()

	pkg := NewDebPackage(pkgReader, nil)
	err = pkg.Parse()
	if err != nil {
		return nil, err
	}

	control, _ := pkg.Control()
	arch, ok := control.GetValue("Architecture")
	if !ok {
		arch = "all"
	}
	item.Architecture = arch

	control.SetValue("Filename", file.Filename)
	control.SetValue("Size", strconv.FormatInt(file.Size, 10))
	control.SetValue("MD5sum", file.Md5)
	control.SetValue("SHA1", file.Sha1)
	control.SetValue("SHA256", file.Sha256)

	paragraphs := make(ControlFile, 1)
	paragraphs[0] = control

	item.ControlID, err = r.AddBinaryControlFile(paragraphs)
	if err != nil {
		return nil, err
	}

	item.Name, _ = pkg.Name()
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
type consistantControlFile []struct {
	Keys   []string
	Values [][]string
}

func (r repoBlobStore) GetDebianControlFile(id StoreID) (ControlFile, error) {
	reader, err := r.Open(id)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	dec := gob.NewDecoder(reader)
	var item consistantControlFile
	err = dec.Decode(&item)
	if err != nil {
		return nil, err
	}

	result := make(ControlFile, len(item))
	for i := range item {
		para := MakeControlParagraph()
		result[i] = &para
		for j := range item[i].Keys {
			strVals := item[i].Values[j]
			for k := range strVals {
				result[i].AddValue(item[i].Keys[j], strVals[k])
			}
		}
	}

	return result, nil
}

func (r repoBlobStore) AddDebianControlFile(item ControlFile) (StoreID, error) {
	data := make(consistantControlFile, len(item))

	for i := range item {
		data[i].Keys = make([]string, len(*item[i]))
		data[i].Values = make([][]string, len(*item[i]))

		j := 0
		for s := range *item[i] {
			data[i].Keys[j] = s
			j++
		}
		sort.Strings(data[i].Keys)

		for j = range data[i].Keys {
			key := data[i].Keys[j]
			valptrs, _ := item[i].GetValues(key)
			vals := make([]string, len(valptrs))
			for k := range valptrs {
				vals[k] = *valptrs[k]
			}
			data[i].Values[j] = vals
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

func (r repoBlobStore) GetBinaryControlFile(id StoreID) (ControlFile, error) {
	return r.GetDebianControlFile(id)
}

func (r repoBlobStore) AddBinaryControlFile(data ControlFile) (StoreID, error) {
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

// Define the order we want items to appear in the index
func IndexOrder(a, b *RepoItem) int {
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
	handle  io.ReadCloser
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

func (r *repoIndexReaderHandle) Close() error {
	err := r.handle.Close()
	return err
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
	defer reader.Close()

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
	parent, err := r.GetCommit(parentid)
	if err != nil {
		return nil, errors.New("error getting parent commit, " + err.Error())
	}

	parentidx, err := r.OpenIndex(parent.Index)
	if err != nil {
		return nil, errors.New("error getting parent commit index, " + err.Error())
	}
	defer parentidx.Close()

	mergedidx, err := r.AddIndex()
	if err != nil {
		return nil, errors.New("error adding new index, " + err.Error())
	}

	left, err := parentidx.NextItem()

	right := items
	sort.Sort(ByIndexOrder(right))

	for {
		if err != nil {
			break
		}

		if len(right) > 0 {
			cmpItems := IndexOrder(&left, right[0])
			if cmpItems < 0 { // New item not needed yet
				mergedidx.AddItem(&left)
				left, err = parentidx.NextItem()
				continue
			} else if cmpItems == 0 { // New item identical to existing
				mergedidx.AddItem(&left)
				left, err = parentidx.NextItem()
				right = right[1:]
				continue
			} else {
				mergedidx.AddItem(right[0])
				right = right[1:]
				continue
			}
		} else {
			mergedidx.AddItem(&left)
			left, err = parentidx.NextItem()
			continue
		}
	}

	// output any items that are left
	for i := range right {
		mergedidx.AddItem(right[i])
	}

	id, err := mergedidx.Close()
	return IndexID(id), err
}
