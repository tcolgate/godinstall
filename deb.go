package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"code.google.com/p/go.crypto/openpgp"
	"code.google.com/p/go.crypto/openpgp/clearsign"
	"github.com/blakesmith/ar"
)

// DebVersion contains the componenet of a debian version
type DebVersion struct {
	Epoch    int    // Primarily used for fixing previous errors in versiongin
	Version  string // The "upstream" version, of the actual packaged applications
	Revision string // This is the revision for the packaging of the main upstream version
}

func (d *DebVersion) String() string {
	output := ""

	if d.Epoch != 0 {
		output = strconv.FormatInt(int64(d.Epoch), 10) + ":"
	}

	output = output + d.Version

	if d.Revision != "" {
		output = output + "-" + d.Revision
	}

	return output
}

// DebVersionFromString converts a string to the debian version components
func DebVersionFromString(str string) (version DebVersion, err error) {
	epochSplit := strings.SplitN(str, ":", 2)
	if len(epochSplit) > 1 {
		var epoch int64
		epochStr := epochSplit[0]
		epoch, err = strconv.ParseInt(epochStr, 10, 64)
		if err != nil {
			return
		}
		version.Epoch = int(epoch)
	}

	verRevStr := epochSplit[len(epochSplit)-1]

	verRevSplit := strings.Split(verRevStr, "-")

	if len(verRevSplit) > 1 {
		version.Version = strings.Join(verRevSplit[:len(verRevSplit)-1], "-")
		version.Revision = verRevSplit[len(verRevSplit)-1]
	} else {
		version.Version = verRevSplit[0]
	}

	return
}

// DebVersionCompare compares two version, returning 0 if equal, < 0 if a < b or
// > 0 if a > b. Annoyingly 'subtle' code, poached from
// http://anonscm.debian.org/cgit/dpkg/dpkg.git/tree/lib/dpkg/version.c?h=wheezy
func DebVersionCompare(a DebVersion, b DebVersion) int {
	if a.Epoch > b.Epoch {
		return 1
	}
	if a.Epoch < b.Epoch {
		return -1
	}

	res := compareComponent(a.Version, b.Version)

	if res == 0 {
		res = compareComponent(a.Revision, b.Revision)
	}

	return res
}
func charOrder(c int) int {
	/**
	 * @param c An ASCII character.
	 */
	switch {
	case unicode.IsDigit(rune(c)):
		return 0
	case unicode.IsLetter(rune(c)):
		return c
	case c == '~':
		return -1
	case c != 0:
		return c + 256
	default:
		return 0
	}
}

func compareComponent(a string, b string) int {
	var i, j int
	for {
		if i < len(a) || j < len(b) {
			firstdiff := 0
			for {
				if (i < len(a) && !unicode.IsDigit(rune(a[i]))) ||
					(j < len(b) && !unicode.IsDigit(rune(b[j]))) {
					var ac, bc int
					if i < len(a) {
						ac = charOrder(int(a[i]))
					}
					if j < len(b) {
						bc = charOrder(int(b[j]))
					}
					if ac != bc {
						return ac - bc
					}
					i++
					j++
				} else {
					break
				}
			}
			for {
				if i < len(a) && rune(a[i]) == '0' {
					i++
				} else {
					break
				}
			}
			for {
				if j < len(b) && rune(b[j]) == '0' {
					j++
				} else {
					break
				}
			}
			for {
				if (i < len(a) && unicode.IsDigit(rune(a[i]))) &&
					(j < len(b) && unicode.IsDigit(rune(b[j]))) {
					if firstdiff == 0 {
						firstdiff = int(a[i]) - int(b[j])
					}
					i++
					j++
				} else {
					break
				}
			}
			if i < len(a) && unicode.IsDigit(rune(a[i])) {
				return 1
			}
			if j < len(b) && unicode.IsDigit(rune(b[j])) {
				return -1
			}
			if firstdiff != 0 {
				return firstdiff
			}
		} else {
			break
		}
	}

	return 0
}

// DebPackageInfoer for describing extracting information relating to
// a debian package
type DebPackageInfoer interface {
	Parse() error
	Name() (string, error)
	Version() (DebVersion, error)
	Description() (string, error)
	Maintainer() (string, error)
	Control() (*ControlParagraph, error) // Map of all the package metadata

	IsSigned() (bool, error)            // This package contains a dpkg-sig signature
	IsValidated() (bool, error)         // The signature has been validated against provided keyring
	SignedBy() (*openpgp.Entity, error) // The entity that signed the package

	Md5() ([]byte, error)
	Sha1() ([]byte, error)
	Sha256() ([]byte, error)
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

	md5    []byte // Package md5
	sha1   []byte // Package sha1
	sha256 []byte // Package sha256

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
	paragraphs, _ := ParseDebianControl(sigDataReader)

	strVersion, _ := paragraphs[0].GetValue("Version")
	result.version, _ = strconv.Atoi(strVersion)

	fileData, _ := paragraphs[0].GetValues("Files")
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

// NewDebPackage creates a debian package description, which will be lazily parsed and
// verified against the provided keyring
func NewDebPackage(r io.Reader, kr openpgp.EntityList) DebPackageInfoer {
	return &debPackage{
		reader:  r,
		keyRing: kr,
	}
}

// Does this package contain signatures
func (d *debPackage) Parse() error {
	var err error

	if !d.parsed {
		err = d.parseDebPackage()
	}

	return err
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
func (d *debPackage) Control() (*ControlParagraph, error) {
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
	return d.getMandatoryControl("Package")
}

// The package version string
func (d *debPackage) Version() (debver DebVersion, err error) {
	verStr, err := d.getMandatoryControl("Version")
	if err != nil {
		return
	}
	return DebVersionFromString(verStr)
}

// The package description
func (d *debPackage) Description() (string, error) {
	return d.getMandatoryControl("Description")
}

// The package maintainer contact details
func (d *debPackage) Maintainer() (string, error) {
	return d.getMandatoryControl("Maintainer")
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
		err = d.parseDebPackage()
		if err != nil {
			return "", false, err
		}
	}

	ctrl, err := d.Control()
	result, ok := ctrl.GetValue(key)

	return result, ok, nil
}

func (d *debPackage) Md5() ([]byte, error) {
	var err error

	if !d.parsed {
		err = d.parseDebPackage()
		if err != nil {
			return nil, err
		}
	}

	return d.md5, nil
}

func (d *debPackage) Sha1() ([]byte, error) {
	var err error

	if !d.parsed {
		err = d.parseDebPackage()
		if err != nil {
			return nil, err
		}
	}

	return d.sha1, nil
}

func (d *debPackage) Sha256() ([]byte, error) {
	var err error

	if !d.parsed {
		err = d.parseDebPackage()
		if err != nil {
			return nil, err
		}
	}

	return d.sha256, nil
}

// parse a debian pacakge.. Debian packages are ar archives containing;
//  data.tar.gz - The actual package content - changelog is in here
//  contro.tar.gz - The control data from when the package was built
//  debian-binary - Package version info
//  _gpg* - dpkg-sig signatures covering hashes of the other files
func (d *debPackage) parseDebPackage() (err error) {
	pkghasher := MakeWriteHasher(ioutil.Discard)
	pkgtee := io.TeeReader(d.reader, pkghasher)

	arReader := ar.NewReader(pkgtee)
	d.signatures = make(map[string]*debSigsFile)

	var control bytes.Buffer
	var controlReader io.Reader

	// We'll keep a running set of file signatures to
	// compare with any included signatures
	d.calcedSigs = make(map[string]debSigs)
	d.signed = false
	d.validated = false

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

		hasher := MakeWriteHasher(ioutil.Discard)

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
			md5:  hex.EncodeToString(hasher.MD5Sum()),
			sha1: hex.EncodeToString(hasher.SHA1Sum()),
		}
	}

	d.md5 = pkghasher.MD5Sum()
	d.sha1 = pkghasher.SHA1Sum()
	d.sha256 = pkghasher.SHA256Sum()

	if control.Len() == 0 {
		return errors.New("did not find control file in debian archive")
	}

	controlReader = bytes.NewReader(control.Bytes())
	paragraphs, err := ParseDebianControl(controlReader)
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

// ControlFile repsents a set of debian control data
// paragraphs
type ControlFile []*ControlParagraph

// ControlParagraph represents a set of key value mappings
// read from a debian control file
type ControlParagraph map[string][]*string

// MakeControlParagraph initialises a new control paragraph
func MakeControlParagraph() ControlParagraph {
	return make(ControlParagraph)
}

// GetValues returns the set of values associated with a key
// from a paragraph of control data
func (ctrl ControlParagraph) GetValues(item string) ([]*string, bool) {
	v, ok := ctrl[item]
	return v, ok
}

// GetValue returns the first value associated with a key in a control
// paragraph
func (ctrl ControlParagraph) GetValue(item string) (string, bool) {
	v, ok := ctrl.GetValues(item)
	return *v[0], ok
}

// SetValue sets a control paragraph item to the single value provided
func (ctrl ControlParagraph) SetValue(item string, val string) {
	ctrl[item] = []*string{&val}
	return
}

// AddValue adds an additional value to a paragraph item
func (ctrl ControlParagraph) AddValue(item string, val string) {
	field, ok := ctrl[item]
	if ok {
		ctrl[item] = append(field, &val)
	} else {
		ctrl[item] = []*string{&val}
	}
	return
}

// ParseDebianControl parses the contents of the reader as a debian
// control file. It does not collapse folding or multiple value fields
// and assumes this has already been done.
func ParseDebianControl(rawin io.Reader) (ControlFile, error) {
	var paras = make(ControlFile, 1)
	var newpara = MakeControlParagraph()
	paras[0] = &newpara
	scanner := bufio.NewScanner(rawin)
	var currfield string

	for scanner.Scan() {
		currpara := paras[len(paras)-1]
		line := scanner.Text()
		switch {
		case line == "":
			{
				var newp = MakeControlParagraph()
				paras = append(paras, &newp)
			}
		case line[0] == ' ', line[0] == '\t':
			{
				currpara.AddValue(currfield, strings.TrimSpace(line))
			}
		default:
			{
				vs := strings.SplitN(line, ":", 2)
				currfield = vs[0]
				val := ""
				if len(vs) > 1 {
					val = strings.TrimSpace(vs[1])
				}
				currpara.AddValue(currfield, val)
			}
		}
	}

	return paras, nil
}

// WriteDebianControl writes the control paragraphs to the io.Writer, using the keys
// from start first, if present. Any keys from end are output last, if present,
// any remaining keys. Other keys are output in between, sorted by key
func WriteDebianControl(out io.Writer, paragraphs ControlFile, start []string, end []string) {
	for p := range paragraphs {
		fields := paragraphs[p]
		orderedMap := make(map[string]bool, len(*fields))

		// We don't want to repeat fields used in the start and
		// end lists, so track which ones we will output there
		for f := range start {
			orderedMap[start[f]] = true
		}
		for f := range end {
			orderedMap[end[f]] = true
		}

		// Output first fields
		for i := range start {
			fieldName := start[i]
			lines, ok := fields.GetValues(fieldName)
			if ok {
				out.Write([]byte(fieldName + ": " + *lines[0] + "\n"))
				rest := lines[1:]
				for j := range rest {
					out.Write([]byte(" " + *rest[j] + "\n"))
				}
			}
		}

		// Collate the remaining fields
		var middle []string
		for fieldName := range *fields {
			if !orderedMap[fieldName] {
				middle = append(middle, fieldName)
			}
		}

		sort.Strings(middle)
		for i := range middle {
			fieldName := middle[i]
			lines, ok := fields.GetValues(fieldName)
			if ok {
				out.Write([]byte(fieldName + ": " + *lines[0] + "\n"))
				rest := lines[1:]
				for j := range rest {
					out.Write([]byte(" " + *rest[j] + "\n"))
				}
			}
		}

		// Output final fields
		for i := range end {
			fieldName := end[i]
			lines, ok := fields.GetValues(fieldName)
			if ok {
				out.Write([]byte(fieldName + ": " + *lines[0] + "\n"))
				rest := lines[1:]
				for j := range rest {
					out.Write([]byte(" " + *rest[j] + "\n"))
				}
			}
		}

		// If this isn't the last paragraph, output a newline
		if p < len(paragraphs)-1 {
			out.Write([]byte("\n"))
		}
	}
}

// FormatControlFile outputs a debian control file, with some commong fields in
// a sensible order
func FormatControlFile(ctrlWriter io.Writer, paragraphs ControlFile) {
	debStartFields := []string{"Package", "Version", "Filename", "Directory", "Size"}
	debEndFields := []string{"MD5Sum", "MD5sum", "SHA1", "SHA256", "Description"}
	WriteDebianControl(ctrlWriter, paragraphs, debStartFields, debEndFields)
}
