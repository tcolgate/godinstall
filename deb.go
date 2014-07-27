package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"code.google.com/p/go.crypto/openpgp"
	"github.com/blakesmith/ar"
	"github.com/stapelberg/godebiancontrol"
)

type DebPackageInfoer interface {
	Signed() (bool, error)
	Validated() (bool, error)
	SignedBy() ([]*openpgp.Entity, error)

	Name() (string, error)
	Version() (string, error)
	Description() (string, error)
	Maintainer() (string, error)
	MetaData() (map[string]string, error)
}

type debPackage struct {
	reader  io.Reader          // the reader pointing to the file
	keyRing openpgp.EntityList // Keyring for verification, nil if no signature is to be validated

	parsed    bool
	signed    bool              // Whether this changes file signed
	validated bool              //  Whether the signature is valid
	signedBy  []*openpgp.Entity // The pgp entity that signed the file

	controlMap map[string]string
}

func NewDebPackage(r io.Reader, kr openpgp.EntityList) DebPackageInfoer {
	return &debPackage{
		reader:  r,
		keyRing: kr,
	}
}

func (d *debPackage) Signed() (bool, error) {
	var err error

	if !d.parsed {
		err = d.parseDebPackage()
		if err != nil {
			return false, err
		}
	}

	return d.signed, nil
}

func (d *debPackage) Validated() (bool, error) {
	var err error

	if !d.parsed {
		err = d.parseDebPackage()
		if err != nil {
			return false, err
		}
	}

	return d.validated, nil
}

func (d *debPackage) SignedBy() ([]*openpgp.Entity, error) {
	var err error

	if !d.parsed {
		err = d.parseDebPackage()
		if err != nil {
			return nil, err
		}
	}

	return d.signedBy, nil
}

func (d *debPackage) MetaData() (map[string]string, error) {
	var err error

	if !d.parsed {
		err = d.parseDebPackage()
		if err != nil {
			return nil, err
		}
	}

	return d.controlMap, nil
}

func (d *debPackage) Name() (string, error) {
	return d.getMandatoryMetadata("Package")
}

func (d *debPackage) Version() (string, error) {
	return d.getMandatoryMetadata("Version")
}

func (d *debPackage) Description() (string, error) {
	return d.getMandatoryMetadata("Description")
}

func (d *debPackage) Maintainer() (string, error) {
	return d.getMandatoryMetadata("Maintainer")
}

func (d *debPackage) getMandatoryMetadata(key string) (string, error) {
	res, ok, err := d.getMetadata(key)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("key %s not found in package metadata", key)
	}

	return res, nil
}

func (d *debPackage) getMetadata(key string) (string, bool, error) {
	var err error

	if !d.parsed {
		err = d.parseDebPackage()
		if err != nil {
			return "", false, err
		}
	}

	result, ok := d.controlMap[key]

	return result, ok, nil
}

func (d *debPackage) parseDebPackage() (err error) {
	arReader := ar.NewReader(d.reader)

	signatures := make(map[string]string)
	var control bytes.Buffer
	var controlReader io.Reader

	// if we are required to validate a signature, we must reconstruct the
	// entity that was signed. To do this we need a temporary file to build
	// the contents, which we then verify against the signature provide in
	// the _gpg file
	//var contentFile os.File

	if d.keyRing != nil {
		contentFile, err := ioutil.TempFile("", "")
		defer os.Remove(contentFile.Name())
		if err != nil {
			return err
		}
	}

	for {
		header, err := arReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		md5er := md5.New()
		sha1er := sha1.New()
		hasher := io.MultiWriter(md5er, sha1er)

		switch {
		case strings.HasPrefix(header.Name, "_gpg"):
			d.signed = true
			var sig bytes.Buffer

			io.Copy(
				io.MultiWriter(&sig, hasher),
				arReader)

			signatures[header.Name] = sig.String()
			log.Println(sig.String())
		case header.Name == "control.tar.gz":
			tee := io.TeeReader(arReader, hasher)

			gzReader, err := gzip.NewReader(tee)
			if err != nil {
				return err
			}

			tarReader := tar.NewReader(gzReader)
			if err != nil {
				return err
			}

			for {
				tarHeader, err := tarReader.Next()
				if err == io.EOF {
					break
				}
				if tarHeader.Name == "./control" {
					io.Copy(&control, tarReader)
				}
			}
		default:
			io.Copy(hasher, arReader)
		}

		md5 := hex.EncodeToString(md5er.Sum(nil))
		sha1 := hex.EncodeToString(sha1er.Sum(nil))

		log.Println("hashes " + md5 + " " + sha1)
	}

	if control.Len() == 0 {
		return errors.New("did not find control file in archive")
	}

	controlReader = bytes.NewReader(control.Bytes())
	paragraphs, err := godebiancontrol.Parse(controlReader)
	if err != nil {
		return errors.New("parsing control failed, " + err.Error())
	}

	log.Println(paragraphs)
	d.controlMap = paragraphs[0]
	d.parsed = true
	return nil
}
