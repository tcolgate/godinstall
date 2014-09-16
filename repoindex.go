package main

import (
	"encoding/gob"
	"strconv"
	"strings"

	"github.com/stapelberg/godebiancontrol"
)

type RepoItemType int

const (
	UNKNOWN RepoItemType = 1 << iota
	BINARY  RepoItemType = 2
	SOURCE  RepoItemType = 3
)

type RepoItemFile struct {
	Name string
	ID   StoreID
}

type RepoItem struct {
	Type         RepoItemType
	Name         string
	Version      DebVersion
	Architecture string
	ID           StoreID
	Files        []*RepoItemFile
}

func RepoItemsFromChanges(changes *ChangesFile, store Storer) ([]*RepoItem, error) {
	var err error

	// Build repository items
	result := make([]*RepoItem, 0)
	for i, file := range changes.Files {
		switch {
		case strings.HasSuffix(i, ".deb"):
			var item RepoItem
			item.Type = BINARY

			pkg := NewDebPackage(file.data, nil)
			err := pkg.Parse()
			if err != nil {
				break
			}

			control, _ := pkg.Control()
			arch, ok := control["Architecture"]
			if !ok {
				arch = "all"
			}
			item.Architecture = arch

			// Store control file
			debStartFields := []string{"Package", "Version", "Filename", "Size"}
			debEndFields := []string{"MD5sum", "SHA1", "SHA256", "Description"}
			ctrlWriter, err := store.Store()
			if err != nil {
				return nil, err
			}

			control["Filename"] = file.Filename
			control["Size"] = strconv.FormatInt(file.Size, 10)
			control["MD5sum"] = file.Md5
			control["SHA1"] = file.Sha1
			control["SHA256"] = file.Sha256

			paragraphs := make([]godebiancontrol.Paragraph, 1)
			paragraphs[0] = control

			WriteDebianControl(ctrlWriter, paragraphs, debStartFields, debEndFields)
			ctrlWriter.Write([]byte("\n"))
			ctrlWriter.Close()
			item.ID, err = ctrlWriter.Identity()
			if err != nil {
				return nil, err
			}

			item.Version, _ = pkg.Version()

			fileSlice := make([]*RepoItemFile, 1)
			fileSlice[0] = &RepoItemFile{
				Name: file.Filename,
				ID:   file.StoreID,
			}
			item.Files = fileSlice

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
