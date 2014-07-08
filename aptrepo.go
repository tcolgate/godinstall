package main

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
)

type AptRepo interface {
	PoolFilePath(string) string
	FindReleaseBase() (string, error)
}

type aptRepo struct {
	RepoBase    *string
	PoolBase    *string
	PoolPattern *regexp.Regexp
}

// Given a file name, provide the full path to the location
// in the debian apt pool
func (a *aptRepo) PoolFilePath(filename string) (poolpath string) {
	poolpath = *a.PoolBase

	matches := a.PoolPattern.FindSubmatch([]byte(filename))
	if len(matches) > 0 {
		poolpath = poolpath + string(matches[0]) + "/"
	}

	return
}

// Find the locatio to write a Releases file to, this assumes
// an apt archive is already present
func (a *aptRepo) FindReleaseBase() (string, error) {
	releasePath := ""

	visit := func(path string, f os.FileInfo, errIn error) (err error) {
		switch {
		case f.Name() == "Contents-all":
			releasePath = filepath.Dir(path)
			err = errors.New("Found file")
		case f.Name() == "pool":
			err = filepath.SkipDir
		}
		return err
	}

	filepath.Walk(*a.RepoBase, visit)

	if releasePath == "" {
		return releasePath, errors.New("Can't locate release base dir")
	}

	return releasePath, nil
}
