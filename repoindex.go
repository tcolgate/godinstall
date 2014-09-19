package main

import (
	"encoding/gob"
	"io"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/stapelberg/godebiancontrol"
)

type ControlData []godebiancontrol.Paragraph

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

func RepoIndexDeb(file *ChangesItem, store Storer) (*RepoItem, error) {
	var item RepoItem
	item.Type = BINARY

	pkgReader, err := store.Open(file.StoreID)
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

	item.ControlID, err = StoreBinaryControlFile(store, paragraphs)
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

func RepoItemsFromChanges(files map[string]*ChangesItem, store Storer) ([]*RepoItem, error) {
	var err error

	// Build repository items
	result := make([]*RepoItem, 0)
	for i, file := range files {
		switch {
		case strings.HasSuffix(i, ".deb"):
			item, err := RepoIndexDeb(file, store)
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

func RetrieveRepoItem(s Storer, id StoreID) (*RepoItem, error) {
	reader, err := s.Open(id)
	if err != nil {
		return nil, err
	}

	dec := gob.NewDecoder(reader)
	var item RepoItem
	dec.Decode(&item)

	return &item, nil
}

func StoreRepoItem(s Storer, item RepoItem) (StoreID, error) {
	writer, err := s.Store()
	if err != nil {
		return nil, err
	}
	enc := gob.NewEncoder(writer)

	enc.Encode(item)
	writer.Close()
	id, err := writer.Identity()
	if err != nil {
		return nil, err
	}

	return id, nil
}

func RetrieveBinaryControlFile(s Storer, id StoreID) (ControlData, error) {
	reader, err := s.Open(id)
	if err != nil {
		return nil, err
	}

	dec := gob.NewDecoder(reader)
	var item ControlData
	dec.Decode(&item)

	return item, nil
}

func StoreBinaryControlFile(s Storer, data ControlData) (StoreID, error) {
	writer, err := s.Store()
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

	return id, nil
}

// Used for tracking the state of reads from an Index
type repoIndexWriterHandle struct {
	handle  StoreWriteCloser
	encoder *gob.Encoder
}

// RepoIndex represent a complete list of packages that will make
// up a full release.
func NewRepoIndex(store Storer) (h repoIndexWriterHandle, err error) {
	h.handle, err = store.Store()
	if err != nil {
		return
	}

	h.encoder = gob.NewEncoder(h.handle)
	return
}

func (r *repoIndexWriterHandle) AddRepoItem(item *RepoItem) (err error) {
	err = r.encoder.Encode(item)
	return
}

func (r *repoIndexWriterHandle) Close() (id StoreID, err error) {
	err = r.handle.Close()
	if err != nil {
		return
	}

	id, err = r.handle.Identity()
	return
}

type repoIndexReaderHandle struct {
	handle  io.Reader
	decoder *gob.Decoder
}

func OpenRepoIndex(id StoreID, store Storer) (h repoIndexReaderHandle, err error) {
	h.handle, err = store.Open(id)
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

// RepoCommit represents an actual complete repository state
type RepoCommit struct {
	Parent      StoreID      // The previous commit we updated
	Date        time.Time    // Time of the update
	Index       StoreID      // The item index in the blob store
	PoolPattern string       // The pool pattern we used to reify the index files
	Packages    StoreID      // StoreID for reified binary index
	PackagesGz  StoreID      // StoreID for reified compressed binary index
	Sources     StoreID      // StoreID for reified source index
	SourcesGz   StoreID      // StoreID for reified compressed source index
	Release     StoreID      // StoreID for reified release file
	InRelease   StoreID      // StoreID for reified signed release file
	Actions     []RepoAction // List of all actions that were performed for this commit
}

func RetrieveRepoCommit(s Storer, id StoreID) (*RepoCommit, error) {
	var commit RepoCommit
	reader, err := s.Open(id)
	if err != nil {
		return nil, err
	}

	dec := gob.NewDecoder(reader)
	dec.Decode(&commit)

	return &commit, nil
}

func StoreRepoCommit(s Storer, data RepoCommit) (StoreID, error) {
	writer, err := s.Store()
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

	return id, nil
}
