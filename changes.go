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

// DebChanges represents a debian changes file. A changes file
// describes a set of files for upload to a repositry
type DebChanges struct {
	signed    bool                    // Whether this changes file signed
	validated bool                    //  Whether the signature is valid
	signedBy  *openpgp.Entity         // The pgp entity that signed the file
	Files     map[string]*ChangesItem // Descriptions of files to be included in this upload
}

// ChangesItem describes a specific item to be uploaded along
// with the changes file
type ChangesItem struct {
	Filename string
	Size     string
	Md5      string
	Sha1     string
	Sha256   string
	Uploaded bool
	data     io.Reader
}

// Parse a debian chnages file into a DebChanges object and verify any signature
// against keys in PHP keyring kr.
//
// TODO This fails DRY badlt as we repeat the process for each signature type
// rewrite this to be more generic
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
		}
		br = bytes.NewReader(msg.Plaintext)
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

	c.Files = make(map[string]*ChangesItem)
	files := strings.Split(filesStr, "\n")
	for _, f := range files {
		fileDesc := strings.Fields(f)
		if len(fileDesc) == 5 {
			cf := ChangesItem{
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
