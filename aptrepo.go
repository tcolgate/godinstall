package main

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
)

// AptRepo is an interface for desribing the disk layout
// of a repository
type AptRepo interface {
	Base() string
	PoolFilePath(string) string
	FindReleaseBase() (string, error)
}

// Simple AptRepo implementation
type aptRepo struct {
	RepoBase    *string        // The base directory of the repository
	PoolPattern *regexp.Regexp // A regex for deciding which pool directory to store a file in
}

// Return the raw path to the base directory, used for directly
// serving content
func (a *aptRepo) Base() string {
	return *a.RepoBase
}

// Given a file name, provide the full path to the location
// in the debian apt pool
func (a *aptRepo) PoolFilePath(filename string) (poolpath string) {
	poolpath = *a.RepoBase + "/pool"

	matches := a.PoolPattern.FindSubmatch([]byte(filename))
	if len(matches) > 0 {
		poolpath = poolpath + "/" + string(matches[0]) + "/"
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
