package deb

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"reflect"
	"strconv"
	"strings"

	"code.google.com/p/go.crypto/openpgp"
	"code.google.com/p/go.crypto/openpgp/clearsign"
	"github.com/blakesmith/ar"
	"github.com/tcolgate/godinstall/hasher"
)

// PackageInfoer for describing extracting information relating to
// a debian package
type PackageInfoer interface {
	Name() (string, error)
	Version() (Version, error)
	Description() (string, error)
	Maintainer() (string, error)
	Architecture() (string, error)
	Control() (*ControlFile, error) // Map of all the package metadata

	IsSigned() (bool, error)            // This package contains a dpkg-sig signature
	IsVerified() (bool, error)          // The signature has been verified against provided keyring
	SignedBy() (*openpgp.Entity, error) // The entity that signed the package

	Md5() ([]byte, error)
	Sha1() ([]byte, error)
	Sha256() ([]byte, error)
	Size() (int64, error)
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
	keyRing openpgp.EntityList // Keyring for verification, nil if no signature is to be verified

	parsed   bool // Have we already parsed this file.
	signed   bool // Whether this changes file signed
	verified bool // Whether the signature is valid

	signatures map[string]*debSigsFile // Details of any signature files contained
	calcedSigs map[string]debSigs      // Calculated signature of the contained files
	signedBy   *openpgp.Entity         // The verified siger of the package

	md5    []byte // Package md5
	sha1   []byte // Package sha1
	sha256 []byte // Package sha256
	size   int64  // Package size

	controlMap *ControlParagraph // This content of the debian/control file
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
	controlFile, _ := ParseControl(sigDataReader, nil)
	control := controlFile.Data[0]

	strVersion, _ := control.GetValue("Version")
	result.version, _ = strconv.Atoi(strVersion)

	fileData, _ := control.GetValues("Files")
	for i := range fileData {
		if *fileData[i] == "" {
			continue
		}

		fields := strings.Fields(
			strings.TrimSpace(
				*fileData[i]))

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

// NewPackage creates a debian package description, which will be lazily parsed and
// verified against the provided keyring
func NewPackage(r io.Reader, kr openpgp.EntityList) PackageInfoer {
	return &debPackage{
		reader:  r,
		keyRing: kr,
	}
}

// Does this package contain signatures
func (d *debPackage) IsSigned() (bool, error) {
	var err error

	if !d.parsed {
		err = d.parsePackage()
		if err != nil {
			return false, err
		}
	}

	return d.signed, nil
}

// Are any of the packages verified by our provided
// keyring
func (d *debPackage) IsVerified() (bool, error) {
	var err error

	if !d.parsed {
		err = d.parsePackage()
		if err != nil {
			return false, err
		}
	}

	return d.verified, nil
}

// Who signed the package
func (d *debPackage) SignedBy() (*openpgp.Entity, error) {
	var err error

	if !d.parsed {
		err = d.parsePackage()
		if err != nil {
			return nil, err
		}
	}

	return d.signedBy, nil
}

// Generic access to the metadata for the package
func (d *debPackage) Control() (*ControlFile, error) {
	var err error

	if !d.parsed {
		err = d.parsePackage()
		if err != nil {
			return nil, err
		}
	}

	return &ControlFile{
		Data: []*ControlParagraph{d.controlMap},
	}, nil
}

// The package name
func (d *debPackage) Name() (string, error) {
	return d.getMandatoryControl("Package")
}

// The package version string
func (d *debPackage) Version() (debver Version, err error) {
	verStr, err := d.getMandatoryControl("Version")
	if err != nil {
		return
	}
	return ParseVersion(verStr)
}

// The package description
func (d *debPackage) Description() (string, error) {
	return d.getMandatoryControl("Description")
}

// The package maintainer contact details
func (d *debPackage) Maintainer() (string, error) {
	return d.getMandatoryControl("Maintainer")
}

// The package maintainer contact details
func (d *debPackage) Architecture() (string, error) {
	return d.getMandatoryControl("Architecture")
}

// Retried any peace of metadata that is mandatory
func (d *debPackage) getMandatoryControl(key string) (string, error) {
	res, ok, err := d.getControl(key)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("key %s not found in package metadata", key)
	}

	return res, nil
}

// Retrieve any emtadata from the control infrormation
func (d *debPackage) getControl(key string) (string, bool, error) {
	var err error

	if !d.parsed {
		err = d.parsePackage()
		if err != nil {
			return "", false, err
		}
	}

	ctrl, err := d.Control()
	result, ok := ctrl.Data[0].GetValue(key)

	return result, ok, nil
}

func (d *debPackage) Md5() ([]byte, error) {
	var err error

	if !d.parsed {
		err = d.parsePackage()
		if err != nil {
			return nil, err
		}
	}

	return d.md5, nil
}

func (d *debPackage) Sha1() ([]byte, error) {
	var err error

	if !d.parsed {
		err = d.parsePackage()
		if err != nil {
			return nil, err
		}
	}

	return d.sha1, nil
}

func (d *debPackage) Sha256() ([]byte, error) {
	var err error

	if !d.parsed {
		err = d.parsePackage()
		if err != nil {
			return nil, err
		}
	}

	return d.sha256, nil
}

func (d *debPackage) Size() (int64, error) {
	var err error

	if !d.parsed {
		err = d.parsePackage()
		if err != nil {
			return 0, err
		}
	}

	return d.size, nil
}

// parse a debian pacakge.. Debian packages are ar archives containing;
//  data.tar.gz - The actual package content - changelog is in here
//  contro.tar.gz - The control data from when the package was built
//  debian-binary - Package version info
//  _gpg* - dpkg-sig signatures covering hashes of the other files
func (d *debPackage) parsePackage() (err error) {
	pkghasher := hasher.New(ioutil.Discard)
	pkgtee := io.TeeReader(d.reader, pkghasher)

	arReader := ar.NewReader(pkgtee)
	d.signatures = make(map[string]*debSigsFile)

	var controlBytes bytes.Buffer
	var controlReader io.Reader

	// We'll keep a running set of file signatures to
	// compare with any included signatures
	d.calcedSigs = make(map[string]debSigs)
	d.signed = false
	d.verified = false

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

		hasher := hasher.New(ioutil.Discard)

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
				if tarHeader.Name == "./control" || tarHeader.Name == "control" {
					io.Copy(&controlBytes, tarReader)
				}
			}
		default:
			// We copy other files so that the hasher builds the hash
			// that we can then verify against the sigs in any sigs file
			io.Copy(hasher, arReader)
		}

		d.calcedSigs[header.Name] = debSigs{
			md5:  hex.EncodeToString(hasher.MD5Sum()),
			sha1: hex.EncodeToString(hasher.SHA1Sum()),
		}
	}

	d.md5 = pkghasher.MD5Sum()
	d.sha1 = pkghasher.SHA1Sum()
	d.sha256 = pkghasher.SHA256Sum()
	d.size = pkghasher.Count()

	if controlBytes.Len() == 0 {
		return errors.New("did not find control file in debian archive")
	}

	controlReader = bytes.NewReader(controlBytes.Bytes())
	controlFile, err := ParseControl(controlReader, nil)
	if err != nil {
		return errors.New("parsing control failed, " + err.Error())
	}
	control := controlFile.Data[0]

	// For each signature file we find, we'll check if we can
	// verify it, and if it covers all included files
	d.verified = false
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
			d.verified = true
			d.signedBy = d.signatures[s].signedBy
			break
		}
	}

	d.controlMap = control
	d.parsed = true

	return nil
}

// FormatDpkgControlFile outputs a debian control file, with some commong fields in
// a sensible order
func FormatDpkgControlFile(ctrlWriter io.Writer, paragraphs ControlFile) {
	debStartFields := []string{"Package", "Version", "Filename", "Directory", "Size"}
	debEndFields := []string{"MD5Sum", "MD5sum", "SHA1", "SHA256", "Description"}
	WriteControl(ctrlWriter, paragraphs, debStartFields, debEndFields)
}
