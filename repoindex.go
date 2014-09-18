package main

import (
	"encoding/gob"
	"log"
	"strconv"
	"strings"

	"github.com/stapelberg/godebiancontrol"
)

type ControlData []godebiancontrol.Paragraph
type RepoItemType int

const (
	UNKNOWN RepoItemType = 1 << iota
	BINARY  RepoItemType = 2
	SOURCE  RepoItemType = 3
)

// A repo item is either deb, or a dsc, describing
// a set of files for a source archive
type RepoItem struct {
	Type         RepoItemType // The type of file
	Name         string
	Version      DebVersion
	Architecture string
	ID           StoreID
	Files        []RepoItemFile
}

type RepoItemFile struct {
	Name string
	ID   StoreID
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

	item.ID, err = StoreBinaryControlFile(store, paragraphs)
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

type repoIndexHandle struct {
	handle  StoreWriteCloser
	encoder *gob.Encoder
}

func NewRepoIndex(store Storer) (h repoIndexHandle, err error) {
	h.handle, err = store.Store()
	if err != nil {
		return
	}

	h.encoder = gob.NewEncoder(h.handle)
	return
}

func (r *repoIndexHandle) AddRepoItem(item *RepoItem) (err error) {
	err = r.encoder.Encode(item)
	return
}

func (r *repoIndexHandle) CloseRepoIndex() (id StoreID, err error) {
	err = r.handle.Close()
	if err != nil {
		return
	}

	id, err = r.handle.Identity()
	return
}
