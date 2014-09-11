package main

import "strings"

type RepoItemType int

const (
	UNKNOWN RepoItemType = 1 << iota
	BINARY  RepoItemType = 2
	SOURCE  RepoItemType = 3
)

type RepoItem interface {
	Type() RepoItem
	Name() string
	Version() DebVersion
	Architecture() string
}

type RepoItemBase struct {
	RepoItem
	name         string
	version      DebVersion
	architecture string
}

func (r *RepoItemBase) Type() RepoItemType {
	return UNKNOWN
}

func (r *RepoItemBase) Name() string {
	return r.name
}

func (r *RepoItemBase) Version() DebVersion {
	return r.version
}

func (r *RepoItemBase) Architecture() string {
	return r.architecture
}

type RepoItemBinary struct {
	RepoItemBase
}

func (r *RepoItemBinary) Type() RepoItemType {
	return BINARY
}

type RepoItemSources struct {
	RepoItemBase
}

func (r *RepoItemSources) Type() RepoItemType {
	return SOURCE
}

func RepoItemsFromChanges(changes *ChangesFile) ([]RepoItem, error) {
	var err error

	// Do some checks
	for i := range changes.Files {
		switch {
		case strings.HasSuffix(i, ".deb"):
		case strings.HasSuffix(i, ".dsc"):
		}
	}

	if err != nil {
		return nil, err
	}

	// Build repository items
	result := make([]RepoItem, 0)
	for i := range changes.Files {
		switch {
		case strings.HasSuffix(i, ".deb"):
		case strings.HasSuffix(i, ".dsc"):
		}
	}

	return result, nil
}
