// Package store is a content addressable file store
// TODO(tcm): Abstract out file operations to allow alternate backing stores
package store

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var ErrInvalidStoreID = errors.New("invalid StoreID")

func (t *hashStore) idToPath(id ID) (string, string, error) {
	if len(id) == 0 {
		return "", "", ErrInvalidStoreID
	}

	idStr := id.String()
	prefix := idStr[0:t.prefixDepth]

	filePath := t.baseDir + "/"
	for c := range prefix {
		filePath = filePath + string(prefix[c]) + "/"
	}

	fileName := filePath + idStr

	return fileName, filePath, nil
}

func (t *hashStore) Open(id ID) (reader io.ReadCloser, err error) {
	name, _, err := t.idToPath(id)
	if err != nil {
		err = errors.New("Failed to translate id to path " + err.Error())
		return nil, err
	}
	reader, err = os.Open(name)

	return reader, err
}

func (t *hashStore) Size(id ID) (size int64, err error) {
	name, _, err := t.idToPath(id)
	if err != nil {
		err = errors.New("Failed to translate id to path " + err.Error())
		return 0, err
	}
	info, err := os.Stat(name)
	size = info.Size()

	return size, err
}

func (t *hashStore) Link(id ID, targets ...string) (err error) {
	name, _, err := t.idToPath(id)
	if err != nil {
		err = errors.New("Failed to translate id to path " + err.Error())
		return err
	}

	for t := range targets {
		target := targets[t]
		prefix := strings.LastIndex(target, "/")
		if prefix > -1 {
			linkDir := target[0 : prefix+1]
			linkName := target[prefix+1:]
			if linkName == "" {
				return errors.New("reference name cannot end in /")
			}
			err = os.MkdirAll(linkDir, 0777)
			if err != nil {
				if os.IsExist(err) {
					break
				} else {
					return err
				}
			}
		}

		linkerr := os.Link(name, targets[t])
		if linkerr != nil {
			if os.IsExist(linkerr) {
				var f1, f2 os.FileInfo
				f1, err = os.Stat(name)
				f2, err = os.Stat(targets[t])
				if os.SameFile(f1, f2) {
					break
				}
				return errors.New("File associated with differet hashes")
			}
			return err
		}
	}
	return err
}

func (t *hashStore) UnLink(id ID) (err error) {
	name, _, err := t.idToPath(id)
	if err != nil {
		err = errors.New("Failed to translate id to path " + err.Error())
		return err
	}
	err = os.Remove(name)

	return err
}

func (t *hashStore) ForEach(fn func(ID)) {
	walker := func(path string, info os.FileInfo, err error) error {
		var reterr error

		if info.IsDir() {
			return reterr
		}

		id, _ := IDFromString(info.Name())
		if len(id) != 0 {
			fn(id)
		}
		return reterr
	}

	filepath.Walk(t.baseDir, walker)
	return
}
