package main

import (
	//"code.google.com/p/go.crypto/openpgp"
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"code.google.com/p/go.crypto/openpgp"
	"code.google.com/p/go.crypto/openpgp/clearsign"
)

// ChangesFile represents a debian changes file. A changes file
// describes a set of files for upload to a repositry
type ChangesFile struct {
	Original     StoreID // The original uploaded changes file
	Date         time.Time
	SourceItem   ChangesItem
	BinaryItems  []ChangesItem
	Distribution string
	Maintainer   string
	Urgency      string
	ChangedBy    string
	Description  string
	Closes       string
	SignedBy     string

	// These fields are only needed during upload and should no be stored
	// in the index. They contain information which is redundant once
	// the upload is shown to be correct
	loneDeb       bool            // Is this really a lone deb file upload?
	signed        bool            // Whether this changes file signed
	validated     bool            // Whether the signature is valid
	signedBy      *openpgp.Entity // The pgp entity that signed the file
	binaryNames   []string
	sourceName    string
	architectures []string
}

// ReleaseIndexItemType is used to differentiate source and binary repository items
type ChangesItemType int

// An uninitialised repo item
// A binary item (a deb)
// a source item (dsc, and related files)
const (
	ChangesUnknownItem ChangesItemType = 1 << iota
	ChangesBinaryItem  ChangesItemType = 2
	ChangesSourceItem  ChangesItemType = 3
)

// ChangesItem represents an individual item referred to by a changes file, with
// the expected size and related hashes
type ChangesItem struct {
	Type         ChangesItemType // The type of file
	Name         string
	Version      DebVersion
	Suite        string
	Component    string
	Architecture string
	ControlID    StoreID           // StoreID for the control data
	Files        []ChangesItemFile // This list of files that make up this item
}

// ReleaseIndexItemFile repesent one file that makes up part of an
// item in the repository. A Binary item will only have one
// file (the deb package), but a Source item may have many
type ChangesItemFile struct {
	Name             string // File name as it will appear in the repo
	StoreID          StoreID
	Size             int64
	Md5              []byte
	Sha1             []byte
	Sha256           []byte
	SignedBy         []string
	UploadHookResult HookOutput

	uploaded bool
	data     io.Reader
}

// ParseDebianChanges parses a debian chnages file into a ChangesFile object
// and verify any signature against keys in GPG keyring kr.
//
// TODO This fails DRY badlt as we repeat the process for each signature type
// rewrite this to be more generic
func ParseDebianChanges(r io.Reader, kr openpgp.EntityList) (p ChangesFile, err error) {
	var c ChangesFile

	b, err := ioutil.ReadAll(r)
	if err != nil {
		return
	}

	var br io.Reader
	msg, rest := clearsign.Decode(b)
	switch {
	case msg == nil && len(rest) > 0:
		{
			c.signed = false
			br = bytes.NewReader(rest)
		}
	case msg != nil && len(msg.Plaintext) > 0:
		{
			c.signed = true
			br = bytes.NewReader(msg.Bytes)
			if len(rest) > 0 {
				log.Println("trailing content in signed control file will be ignored")
			}
		}
	}

	if c.signed {
		if kr == nil {
			log.Println("Validation requested, but keyring is null")
			c.validated = false
		} else {
			c.signedBy, err = openpgp.CheckDetachedSignature(kr, br, msg.ArmoredSignature.Body)
			if err == nil {
				c.validated = true
			} else {
				c.validated = false
			}
		}
		br = bytes.NewReader(msg.Plaintext)
	} else {
		c.validated = false
	}

	paragraphs, err := ParseDebianControl(br)

	if err != nil {
		return c, err
	}

	if paragraphs == nil {
		return ChangesFile{}, errors.New("No valid paragraphs in changes")
	}

	files, ok := paragraphs[0].GetValues("Files")

	if !ok {
		return ChangesFile{}, errors.New("No Files section in changes")
	}

	c.Files = make([]ChangesItem, 0)
	for _, f := range files {
		fileDesc := strings.Fields(*f)
		if len(fileDesc) == 5 {
			size, _ := strconv.ParseInt(fileDesc[1], 10, 64)
			cf := ChangesItem{
				Filename: fileDesc[4],
				Size:     size,
				Md5:      fileDesc[0],
			}
			c.Files = append(c.Files, cf)
		}
	}

	sha1s, ok := paragraphs[0].GetValues("Checksums-Sha1")
	if ok {
		for _, s := range sha1s {
			fileDesc := strings.Fields(*s)
			if len(fileDesc) == 3 {
				name := fileDesc[2]
				ok := false
				for j, f := range c.Files {
					if name == f.Filename {
						ok = true
						c.Files[j].Sha1 = fileDesc[0]
						break
					}
				}

				if !ok {
					log.Printf("Ignoring sha1 for file not listed in Files: %s", name)
				}
			}
		}
	}

	sha256s, ok := paragraphs[0].GetValues("Checksums-Sha256")
	if ok {
		for _, s := range sha256s {
			fileDesc := strings.Fields(*s)
			if len(fileDesc) == 3 {
				name := fileDesc[2]
				ok := false
				for j, f := range c.Files {
					if name == f.Filename {
						ok = true
						c.Files[j].Sha256 = fileDesc[0]
						break
					}
				}

				if !ok {
					log.Printf("Ignoring sha256 for file not listed in Files: %s", name)
				}
			}
		}
	}

	return c, err
}

// ChangesFromHTTPRequest seperates a changes file from any other files in a
// http request
func ChangesFromHTTPRequest(r *http.Request) (
	changesReader io.Reader,
	other []*multipart.FileHeader,
	err error) {

	err = r.ParseMultipartForm(mimeMemoryBufferSize)
	if err != nil {
		return
	}

	form := r.MultipartForm
	files := form.File["debfiles"]
	for _, f := range files {
		if strings.HasSuffix(f.Filename, ".changes") {
			changesReader, _ = f.Open()
		} else {
			other = append(other, f)
		}
	}

	return
}
