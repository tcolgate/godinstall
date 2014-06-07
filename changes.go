package main

import (
	//"code.google.com/p/go.crypto/openpgp"
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"strings"

	"code.google.com/p/go.crypto/openpgp"
	"code.google.com/p/go.crypto/openpgp/clearsign"
	"github.com/stapelberg/godebiancontrol"
)

// This could be generalized to break out the different hashes
// more dynamically
type ChangesFile struct {
	Filename string
	Size     string
	Md5      string
	Sha1     string
	Sha256   string
	Uploaded bool
	data     io.Reader
}

type DebChanges struct {
	signed    bool
	validated bool
	signedBy  *openpgp.Entity
	Files     map[string]*ChangesFile
}

func ParseDebianChanges(r io.Reader, kr openpgp.EntityList) (p *DebChanges, err error) {
	var c DebChanges

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
			br = bytes.NewReader(msg.Plaintext)
		}
	} else {
		c.validated = false
	}

	paragraphs, err := godebiancontrol.Parse(br)

	if err != nil {
		return &c, err
	}

	if paragraphs == nil {
		return nil, errors.New("No valid paragraphs in changes")
	}

	if len(paragraphs) > 1 {
		log.Println("Only first section of the changes file will be parsed")
	}

	filesStr, ok := paragraphs[0]["Files"]

	if !ok {
		return nil, errors.New("No Files section in changes")
	}

	c.Files = make(map[string]*ChangesFile)
	files := strings.Split(filesStr, "\n")
	for _, f := range files {
		fileDesc := strings.Fields(f)
		if len(fileDesc) == 5 {
			cf := ChangesFile{
				Filename: fileDesc[4],
				Size:     fileDesc[1],
				Md5:      fileDesc[0],
			}
			c.Files[cf.Filename] = &cf
		}
	}

	sha1sStr, ok := paragraphs[0]["Checksums-Sha1"]
	if ok {
		sha1s := strings.Split(sha1sStr, "\n")
		for _, s := range sha1s {
			fileDesc := strings.Fields(s)
			if len(fileDesc) == 3 {
				name := fileDesc[2]
				f, ok := c.Files[name]
				if ok {
					f.Sha1 = fileDesc[0]
				} else {
					log.Printf("Ignoring sha1 for file not listed in Files: %s", name)
				}
			}
		}
	}

	sha256sStr, ok := paragraphs[0]["Checksums-Sha256"]
	if ok {
		sha256s := strings.Split(sha256sStr, "\n")
		for _, s := range sha256s {
			fileDesc := strings.Fields(s)
			if len(fileDesc) == 3 {
				name := fileDesc[2]
				f, ok := c.Files[name]
				if ok {
					f.Sha256 = fileDesc[0]
				} else {
					log.Printf("Ignoring sha256 for file not listed in Files: %s", name)
				}
			}
		}
	}

	return &c, err
}
