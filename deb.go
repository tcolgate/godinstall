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
	"io/ioutil"
	"os"
	"reflect"
	"strconv"
	"strings"

	"code.google.com/p/go.crypto/openpgp"
	"code.google.com/p/go.crypto/openpgp/clearsign"
	"github.com/blakesmith/ar"
	"github.com/stapelberg/godebiancontrol"
)

type DebPackageInfoer interface {
	Signed() (bool, error)
	Validated() (bool, error)
	SignedBy() (*openpgp.Entity, error)

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
	signed    bool // Whether this changes file signed
	validated bool //  Whether the signature is valid

	signatures map[string]*debSigsFile
	calcedSigs map[string]debSigs
	signedBy   *openpgp.Entity

	controlMap map[string]string
}

type debSigs struct {
	md5  string
	sha1 string
}

type debSigsFile struct {
	version    int
	signatures map[string]debSigs
	signedBy   *openpgp.Entity
}

func parseSigsFile(sig string, kr openpgp.EntityList) (*debSigsFile, error) {
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
	d.signatures = make(map[string]*debSigsFile)

	var control bytes.Buffer
	var controlReader io.Reader

	// We'll keep a running set of file signatures to
	// compare with any included signatures
	d.calcedSigs = make(map[string]debSigs)

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

			d.signatures[header.Name], _ = parseSigsFile(sig.String(), d.keyRing)

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
