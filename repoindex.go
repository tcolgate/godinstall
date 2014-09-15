package main

import (
	"encoding/gob"
	"strings"
)

type RepoItemType int

const (
	UNKNOWN RepoItemType = 1 << iota
	BINARY  RepoItemType = 2
	SOURCE  RepoItemType = 3
)

type RepoItem interface {
	Type() RepoItemType
	Name() string
	Version() DebVersion
	Architecture() string
	StoreID() StoreID
}

type RepoItemBase struct {
	RepoItem
	name         string
	version      DebVersion
	architecture string
	storeId      StoreID
}

func (r *RepoItemBase) Type() RepoItemType {
	return UNKNOWN
}

func (r *RepoItemBase) Name() string {
	return r.name
}

func (r *RepoItemBase) StoreID() StoreID {
	return r.storeId
}

func (r *RepoItemBase) Version() DebVersion {
	return r.version
}

func (r *RepoItemBase) Architecture() string {
	return r.architecture
}

type RepoItemBinary struct {
	RepoItemBase
	pkg DebPackageInfoer
}

func (r *RepoItemBinary) Type() RepoItemType {
	return BINARY
}

func (r *RepoItemBinary) Architecture() string {
	control, _ := r.pkg.Control()
	arch, ok := control["Architecture"]
	if !ok {
		arch = "all"
	}
	return arch
}

type RepoItemSources struct {
	RepoItemBase
}

func (r *RepoItemSources) Type() RepoItemType {
	return SOURCE
}

func (r *RepoItemSources) Architecture() string {
	return "source"
}

func RepoItemsFromChanges(changes *ChangesFile) ([]RepoItem, error) {
	var err error

	// Build repository items
	result := make([]RepoItem, 0)
	for i, file := range changes.Files {
		switch {
		case strings.HasSuffix(i, ".deb"):
			var item RepoItemBinary
			pkg := NewDebPackage(file.data, nil)
			err := pkg.Parse()
			if err != nil {
				break
			}
			item.pkg = pkg

			result = append(result, &item)

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

type thing struct {
}

func RetrieveRepoItem(s Storer, id StoreID) (RepoItem, error) {
	reader, err := s.Open(id)
	if err != nil {
		return nil, err
	}

	dec := gob.NewDecoder(reader)
	var T thing
	dec.Decode(&T)

	return T, nil
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
