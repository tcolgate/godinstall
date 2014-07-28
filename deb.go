package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"

	"code.google.com/p/go.crypto/openpgp"
	"code.google.com/p/go.crypto/openpgp/clearsign"
	"github.com/blakesmith/ar"
	"github.com/stapelberg/godebiancontrol"
)

// An interface for describing a debian package.
type DebPackageInfoer interface {
	Name() (string, error)
	Version() (string, error)
	Description() (string, error)
	Maintainer() (string, error)
	MetaData() (map[string]string, error) // Map of all the package metadata

	IsSigned() (bool, error)            // This package contains a dpkg-sig signature
	IsValidated() (bool, error)         // The signature has been validated against provided keyring
	SignedBy() (*openpgp.Entity, error) // The entity that signed the package
}

// A set of signatures as contained in a dpkg-gig signed package
type debSigs struct {
	md5  string
	sha1 string
}

// This describes the contains of one signature file contained in a dpkg-sig
// signed package. Several of these may be present in one package
type debSigsFile struct {
	version    int
	signatures map[string]debSigs // A mapping of filenames to signatures
	signedBy   *openpgp.Entity
}

//  This contains the details  of a package, along with calculated hashes of the
// contents of hte packag, and the signature details contained in any _gpg signature
// files contained within.
type debPackage struct {
	reader  io.Reader          // the reader pointing to the file
	keyRing openpgp.EntityList // Keyring for verification, nil if no signature is to be validated

	parsed    bool // Have we already parsed this file.
	signed    bool // Whether this changes file signed
	validated bool // Whether the signature is valid

	signatures map[string]*debSigsFile // Details of any signature files contained
	calcedSigs map[string]debSigs      // Calculated signature of the contained files
	signedBy   *openpgp.Entity         // The verified siger of the package

	controlMap map[string]string // This content of the debian/control file
}

// This parses a dpkg-deb signature file. These are files named _gpg.*
// within the ar archive
func parseSigsFile(sig string, kr openpgp.EntityList) (*debSigsFile, error) {
	// An _gpg file contains a clear signed "debian conrol" style text blob.
	// Version 4 contains lines with an md5, sha1 and size for each file contained
	// within the ar archive that makes up the debian package.

	result := &debSigsFile{}
	result.signatures = make(map[string]debSigs)

	msg, _ := clearsign.Decode([]byte(sig))
	bsig := bytes.NewReader(msg.Bytes)

	result.signedBy, _ = openpgp.CheckDetachedSignature(kr, bsig, msg.ArmoredSignature.Body)

	sigDataReader := bytes.NewReader(msg.Plaintext)
	paragraphs, _ := godebiancontrol.Parse(sigDataReader)

	result.version, _ = strconv.Atoi(paragraphs[0]["Version"])

	fileData := paragraphs[0]["Files"]
	fdLineReader := strings.NewReader(fileData)

	scanner := bufio.NewScanner(fdLineReader)
	for scanner.Scan() {
		fields := strings.Fields(
			strings.TrimSpace(
				scanner.Text()))

		md5 := fields[0]
		sha1 := fields[1]
		fileName := fields[3]

		result.signatures[fileName] = debSigs{
			md5:  md5,
			sha1: sha1,
		}
	}

	return result, nil
}

// Create a debian package description, which will be lazily parsed and
// verified against the provided keyring
func NewDebPackage(r io.Reader, kr openpgp.EntityList) DebPackageInfoer {
	return &debPackage{
		reader:  r,
		keyRing: kr,
	}
}

// Does this package contain signatures
func (d *debPackage) IsSigned() (bool, error) {
	var err error

	if !d.parsed {
		err = d.parseDebPackage()
		if err != nil {
			return false, err
		}
	}

	return d.signed, nil
}

// Are any of the packages validated by our provided
// keyring
func (d *debPackage) IsValidated() (bool, error) {
	var err error

	if !d.parsed {
		err = d.parseDebPackage()
		if err != nil {
			return false, err
		}
	}

	return d.validated, nil
}

// Who signed the package
func (d *debPackage) SignedBy() (*openpgp.Entity, error) {
	var err error

	if !d.parsed {
		err = d.parseDebPackage()
		if err != nil {
			return nil, err
		}
	}

	return d.signedBy, nil
}

// Generic access to the metadata for the package
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

// The package name
func (d *debPackage) Name() (string, error) {
	return d.getMandatoryMetadata("Package")
}

// The package version string
func (d *debPackage) Version() (string, error) {
	return d.getMandatoryMetadata("Version")
}

// The package description
func (d *debPackage) Description() (string, error) {
	return d.getMandatoryMetadata("Description")
}

// The package maintainer contact details
func (d *debPackage) Maintainer() (string, error) {
	return d.getMandatoryMetadata("Maintainer")
}

// Retried any peace of metadata that is mandatory
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

// Retrieve any emtadata from the control infrormation
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

// parse a debian pacakge.. Debian packages are ar archives containing;
//  data.tar.gz - The actual package content - changelog is in here
//  contro.tar.gz - The control data from when the package was built
//  debian-binary - Package version info
//  _gpg* - dpkg-sig signatures covering hashes of the other files
func (d *debPackage) parseDebPackage() (err error) {
	arReader := ar.NewReader(d.reader)
	d.signatures = make(map[string]*debSigsFile)

	var control bytes.Buffer
	var controlReader io.Reader

	// We'll keep a running set of file signatures to
	// compare with any included signatures
	d.calcedSigs = make(map[string]debSigs)

	// We walk over each file in the ar archive, parsing
	// as we go
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
			// This file is a dpkg-sig signature file
			d.signed = true
			var sig bytes.Buffer

			io.Copy(
				io.MultiWriter(&sig, hasher),
				arReader)

			d.signatures[header.Name], _ = parseSigsFile(sig.String(), d.keyRing)

		case header.Name == "control.tar.gz":
			// This file is the control.tar.gz, we can get the
			// package metadata from here in a file called ./control
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
			// We copy other files so that the hasher builds the hash
			// that we can then verify against the sigs in any sigs file
			io.Copy(hasher, arReader)
		}

		d.calcedSigs[header.Name] = debSigs{
			md5:  hex.EncodeToString(md5er.Sum(nil)),
			sha1: hex.EncodeToString(sha1er.Sum(nil)),
		}
	}

	if control.Len() == 0 {
		return errors.New("did not find control file in archive")
	}

	controlReader = bytes.NewReader(control.Bytes())
	paragraphs, err := godebiancontrol.Parse(controlReader)
	if err != nil {
		return errors.New("parsing control failed, " + err.Error())
	}

	// For each signature file we find, we'll check if we can
	// validate it, and if it covers all included files
	d.validated = false
	for s := range d.signatures {
		sigs := d.signatures[s].signatures
		testSigs := make(map[string]debSigs)

		// The calculated signature list includes the file containing
		// the actual signature we are verifying, we must filter it out
		for k, v := range d.calcedSigs {
			if k != s {
				testSigs[k] = v
			}
		}

		if reflect.DeepEqual(testSigs, sigs) {
			d.validated = true
			d.signedBy = d.signatures[s].signedBy
			break
		}
	}

	d.controlMap = paragraphs[0]
	d.parsed = true

	return nil
}