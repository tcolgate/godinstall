package store

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

func (t *hashStore) SetRef(name string, id ID) error {
	refsPath := t.baseDir + "/refs/"
	refDir := refsPath

	refLog, err := os.OpenFile(t.baseDir+"/reflog", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return errors.New("Could not open reflog, " + err.Error())
	}
	defer refLog.Close()

	prefix := strings.LastIndex(name, "/")
	if prefix > -1 {
		refDir = refDir + name[0:prefix+1]
		name = name[prefix+1:]
		if name == "" {
			return errors.New("reference name cannot end in /")
		}
	}

	refFile := refDir + name + ".ref"

	newRef := false

	oldRef, err := ioutil.ReadFile(refFile)
	if err != nil {
		if !os.IsNotExist(err) {
			return errors.New("Could not read old reference, " + err.Error())
		}
		newRef = true
	} else {
		newRef = false
	}

	err = os.MkdirAll(refDir, 0777)
	if err != nil {
		return err
	}

	if newRef {
		_, err = refLog.WriteString(fmt.Sprintf("Create:%s:%s\n", refFile, id.String()))
	} else {
		_, err = refLog.WriteString(fmt.Sprintf("Update:%s:%s(%s)\n", refFile, id.String(), oldRef))
	}
	if err != nil {
		return errors.New("Failed writing reflog, " + err.Error())
	}

	err = refLog.Sync()
	if err != nil {
		return errors.New("Failed syncing reflog, " + err.Error())
	}

	err = ioutil.WriteFile(refFile, []byte(id.String()), 0777)
	return err
}

func (t *hashStore) GetRef(name string) (ID, error) {
	refsPath := t.baseDir + "/refs"
	refFile := refsPath + "/" + name + ".ref"

	f, err := os.Open(refFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	refStr, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}

	refid, err := IDFromString(string(refStr))

	return refid, err
}

func (t *hashStore) DeleteRef(name string) error {
	refsPath := t.baseDir + "/refs"
	refFile := refsPath + "/" + name + ".ref"

	return os.Remove(refFile)
}

func (t *hashStore) ListRefs() map[string]ID {
	refsPath := t.baseDir + "/refs"
	refs := make(map[string]ID)

	walker := func(path string, info os.FileInfo, err error) error {
		var reterr error
		if err != nil {
			return err
		}

		if info.IsDir() {
			return reterr
		}
		refname := strings.TrimSuffix(path[len(refsPath)+1:], ".ref")
		id, _ := t.GetRef(refname)
		refs[refname] = id
		return reterr
	}

	filepath.Walk(refsPath, walker)
	return refs
}
