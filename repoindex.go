package main

type RepoItemType int

const (
	UNKNOWN RepoItemType = 1 << iota
	BINARY  RepoItemType = 2
	SOURCE  RepoItemType = 3
)

type RepoItem interface {
	Type() RepoItem
}

type RepoItemBinary struct {
	RepoItem
}

func (r *RepoItemBinary) Type() RepoItemType {
	return BINARY
}

type RepoItemSources struct {
	RepoItem
}

func (r *RepoItemSources) Type() RepoItemType {
	return SOURCE
}

func RepoItemsFromChanges(changes *ChangesFile) ([]RepoItem, error) {

	result := make([]RepoItem, 0)
	return result, nil
}
