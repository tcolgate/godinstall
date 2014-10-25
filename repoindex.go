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

// RepoStorer defines an interface for interacting with an on disk
// versioned apt repository (probably needs splitting up to seperate
// bits of functionality
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
	MergeItemsIntoCommit(parentid CommitID, items []*RepoItem, pruneRules PruneRuleSet) (result IndexID, actions []RepoAction, err error)
	GarbageCollect()
	DisableGarbageCollector()
	EnableGarbageCollector()
	Storer
}

type gcReq struct {
	done chan struct{}
}

type repoBlobStore struct {
	Storer
	enableGCChan  chan gcReq
	disableGCChan chan gcReq
	runGCChan     chan gcReq
}

// NewRepoBlobStore This creates a new repository, based ona a content
// addressable fs
func NewRepoBlobStore(storeDir string, tmpDir string) RepoStorer {
	result := &repoBlobStore{
		Sha1Store(storeDir, tmpDir, 3),
		make(chan gcReq),
		make(chan gcReq),
		make(chan gcReq),
	}

	go result.garbageCollector()

	return result
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
	used.Set(commit.ReleaseGPG.String(), true)
	used.Set(commit.PackagesGz.String(), true)
	used.Set(commit.Sources.String(), true)
	used.Set(commit.SourcesGz.String(), true)

	if !used.Check(StoreID(commit.Index).String()) {
		r.gcWalkIndex(used, commit.Index)
	}

	if StoreID(commit.Parent).String() != r.EmptyFileID().String() {
		if !used.Check(StoreID(commit.Parent).String()) {
			r.gcWalkCommit(used, commit.Parent)
		}
	}
}

func (r repoBlobStore) runGC() {
	log.Println("Beginning GC")

	stime := time.Now()
	gcFiles := 0
	gcBytes := int64(0)
	defer func() {
		gcDuration := time.Since(stime)
		log.Printf("GC %v files (%v bytes) in %v", gcFiles, gcBytes, gcDuration)
	}()

	used := NewSafeMap()
	refs := r.ListRefs()

	for _, id := range refs {
		r.gcWalkCommit(used, CommitID(id))
	}

	f := func(id StoreID) {
		if !used.Check(id.String()) {
			gcFiles++
			size, _ := r.Size(id)
			gcBytes += size
			log.Println("Removing unused blob ", id.String())
			r.UnLink(id)
		}
	}
	r.ForEach(f)
}

func (r repoBlobStore) garbageCollector() {
	runGC := false
	lockCount := 0

	for {
		var req gcReq
		select {
		case req = <-r.enableGCChan:
			lockCount -= 1
			if lockCount < 0 {
				panic("GC Lock count has gone negactive")
			}
		case req = <-r.disableGCChan:
			lockCount += 1
		case req = <-r.runGCChan:
			runGC = true
		}

		if runGC && lockCount == 0 {
			r.runGC()
			runGC = false
		}

		close(req.done)
	}
}

func (r repoBlobStore) GarbageCollect() {
	c := make(chan struct{})
	r.runGCChan <- gcReq{c}
	<-c
}

func (r repoBlobStore) DisableGarbageCollector() {
	log.Println("Disable GC")
	c := make(chan struct{})
	r.disableGCChan <- gcReq{c}
	<-c
}

func (r repoBlobStore) EnableGarbageCollector() {
	log.Println("Enable GC")
	c := make(chan struct{})
	r.enableGCChan <- gcReq{c}
	<-c
}

func (r repoBlobStore) GetHead(name string) (CommitID, error) {
	id, err := r.GetRef("heads/" + name)
	return CommitID(id), err
}

func (r repoBlobStore) SetHead(name string, id CommitID) error {
	return r.SetRef("heads/"+name, StoreID(id))
}

// RepoItemType is used to differentiate source and binary repository items
type RepoItemType int

// An uninitialised repo item
// A binary item (a deb)
// a source item (dsc, and related files)
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
	var result []*RepoItem
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
	}
	return false
}

// IndexOrder implements  the order we want items to appear in the index
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

// IndexID is used to reference an index description in the
// blob store
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

// RepoActionType is used to document the list of actions
// take by a given merge
type RepoActionType int

//	ActionUNKNOWN     - An uninitilized action item
//	ActionADD         - An item was added
//	ActionDELETE      - An item was explicitly deleted
//	ActionPRUNE       - An item was pruned by the pruning rules
//	ActionSKIPPRESENT - An item was skipped, as it alerady existed
//	ActionSKIPPRUNE   - An item was was skipped, dur to purge rules
const (
	ActionUNKNOWN     RepoActionType = 1 << iota
	ActionADD         RepoActionType = 2
	ActionDELETE      RepoActionType = 3
	ActionPRUNE       RepoActionType = 4
	ActionSKIPPRESENT RepoActionType = 5
	ActionSKIPPRUNE   RepoActionType = 6
	ActionTRIM        RepoActionType = 7
)

// RepoAction desribes an action taken during a merge or update
type RepoAction struct {
	Type        RepoActionType
	Description string
}

// CommitID is used to reference a specific point in the repository's history
type CommitID StoreID

// RepoCommit represents an actual complete repository state
type RepoCommit struct {
	Parent      CommitID     // The previous commit we updated
	Date        time.Time    // Time of the update
	Index       IndexID      // The item index in the blob store
	PoolPattern string       // The pool pattern we used to reify the index files
	PackagesGz  StoreID      // StoreID for reified compressed binary index
	Sources     StoreID      // StoreID for reified source index
	SourcesGz   StoreID      // StoreID for reified compressed source index
	Release     StoreID      // StoreID for reified release file
	ReleaseGPG  StoreID      // StoreID for reified release.gpg file
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
func (r repoBlobStore) MergeItemsIntoCommit(parentid CommitID, items []*RepoItem, pruneRules PruneRuleSet) (result IndexID, actions []RepoAction, err error) {
	parent, err := r.GetCommit(parentid)
	actions = make([]RepoAction, 0)
	if err != nil {
		return nil, actions, errors.New("error getting parent commit, " + err.Error())
	}

	parentidx, err := r.OpenIndex(parent.Index)
	if err != nil {
		return nil, actions, errors.New("error getting parent commit index, " + err.Error())
	}
	defer parentidx.Close()

	mergedidx, err := r.AddIndex()
	if err != nil {
		return nil, actions, errors.New("error adding new index, " + err.Error())
	}

	left, err := parentidx.NextItem()

	right := items
	sort.Sort(ByIndexOrder(right))

	pruner := pruneRules.MakePruner()

	for {
		if err != nil {
			break
		}

		if len(right) > 0 {
			cmpItems := IndexOrder(&left, right[0])
			if cmpItems < 0 { // New item not needed yet
				if !pruner(&left) {
					mergedidx.AddItem(&left)
				} else {
					actions = append(actions, RepoAction{
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
					actions = append(actions, RepoAction{
						Type:        ActionSKIPPRESENT,
						Description: item.Name + " " + item.Architecture + " " + item.Version.String(),
					})
				} else {
					actions = append(actions, RepoAction{
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
					actions = append(actions, RepoAction{
						Type:        ActionADD,
						Description: item.Name + " " + item.Architecture + " " + item.Version.String(),
					})
				} else {
					actions = append(actions, RepoAction{
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
				actions = append(actions, RepoAction{
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
			actions = append(actions, RepoAction{
				Type:        ActionADD,
				Description: item.Name + " " + item.Architecture + " " + item.Version.String(),
			})
		} else {
			actions = append(actions, RepoAction{
				Type:        ActionPRUNE,
				Description: item.Name + " " + item.Architecture + " " + item.Version.String(),
			})
		}
		right = right[1:]
		continue
	}

	id, err := mergedidx.Close()
	return IndexID(id), actions, err
}

// Trimmer is an type for describing functions that can be used to
// trim the repository history
type Trimmer func(*RepoCommit) bool

// MakeTimeTrimmer creates a trimmer function that reduces the repository
// history to a given window of time
func (r repoBlobStore) MakeTimeTrimmer(time.Duration) Trimmer {
	return func(commit *RepoCommit) (trim bool) {

		return false
	}
}

// MakeLengthTrimmer creates a trimmer function that reduces the repository
// history to a given number of commits
func (r repoBlobStore) MakeLengthTrimmer(commitcount int) Trimmer {
	count := commitcount
	return func(commit *RepoCommit) (trim bool) {
		if count >= 0 {
			count--
			return false
		}
		return true
	}
}

// Trim works it's way through the commit history, and rebuilds a new version
// of the repository history, truncated at the commit selected by the trimmer
func (r repoBlobStore) Trim(head CommitID, t Trimmer) (newhead CommitID, err error) {
	var history []*RepoCommit
	newhead = head
	curr := head

	for {
		if StoreID(curr).String() == r.EmptyFileID().String() {
			// We reached an empty commit before we decided to trim
			// so just return the untrimmed origin CommitID
			return head, nil
		}

		c, err := r.GetCommit(curr)
		if err != nil {
			return head, err
		}

		if t(c) {
			break
		}
		history = append(history, c)
	}

	newhead = CommitID(r.EmptyFileID())
	history[len(history)-1].Actions = []RepoAction{
		RepoAction{
			Type:        ActionTRIM,
			Description: "Repository history trimmed",
		},
	}

	for i := len(history) - 1; i >= 0; i-- {
		newcommit := history[i]
		newcommit.Parent = newhead
		newhead, err = r.AddCommit(newcommit)
		if err != nil {
			return head, err
		}
	}

	return
}
