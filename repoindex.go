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

type RepoItemBase struct {
	RepoItem
}

func (r *RepoItemBase) Type() RepoItemType {
	return UNKNOWN
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

	result := make([]RepoItem, 0)
	return result, nil
}
