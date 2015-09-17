package deb

import (
	//"code.google.com/p/go.crypto/openpgp"

	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"code.google.com/p/go.crypto/openpgp"
)

// ChangesFilesIndex is used to index individual entries in the list
// of hashes in a changes file entry
type ChangesFilesIndex struct {
	Name string
	Size int64
}

// ChangesFilesHashSet lists the results of the various hashes
type ChangesFilesHashSet map[string][]byte

// ChangesFilesHashMap maps a given file of a given size to a set
// of hash results
type ChangesFilesHashMap map[ChangesFilesIndex]ChangesFilesHashSet

// ChangesFile represents a debian changes file. A changes file
// describes a set of files for upload to a repositry
type ChangesFile struct {
	Control       ControlFile
	Date          time.Time
	Binaries      []string
	BinaryVersion DebVersion
	Architectures []string
	Source        string
	SourceVersion DebVersion
	FileHashes    ChangesFilesHashMap
}

func changesFileHashes(para *ControlParagraph) (ChangesFilesHashMap, error) {
	f2hs := func(para *ControlParagraph, field string, hName string) (ChangesFilesHashMap, error) {
		hStrings, ok := para.GetValues(field)
		if !ok {
			return ChangesFilesHashMap{}, nil
		}

		records := ChangesFilesHashMap{}
		for _, f := range hStrings[1:] {
			fileDesc := strings.Fields(*f)
			if !(len(fileDesc) == 3 || len(fileDesc) == 5) {
				return ChangesFilesHashMap{}, fmt.Errorf("Malformed %v Files entry", hName)
			}
			h, err := hex.DecodeString(fileDesc[0])
			if err != nil {
				return ChangesFilesHashMap{}, fmt.Errorf("Malformed %v hash entry", hName)
			}

			size, _ := strconv.ParseInt(fileDesc[1], 10, 64)
			records[ChangesFilesIndex{
				Name: fileDesc[len(fileDesc)-1],
				Size: size,
			}] = map[string][]byte{hName: h}
		}

		return records, nil
	}

	md5s, err := f2hs(para, "Files", "md5")
	if err != nil {
		return ChangesFilesHashMap{}, errors.New("Error processing file iformation, " + err.Error())
	}

	sha1s, err := f2hs(para, "Checksums-Sha1", "sha1")
	if err != nil {
		return ChangesFilesHashMap{}, errors.New("Error processing file iformation, " + err.Error())
	}
	if len(sha1s) != 0 && len(sha1s) != len(md5s) {
		return ChangesFilesHashMap{}, errors.New("Mismatch in hashes count")
	}
	for i := range md5s {
		_, ok := sha1s[i]
		if !ok {
			return ChangesFilesHashMap{}, errors.New("Mismatch in hashes count")
		}
	}

	sha256s, err := f2hs(para, "Checksums-Sha256", "sha256")
	if err != nil {
		return ChangesFilesHashMap{}, errors.New("Error processing file information, " + err.Error())
	}
	if len(sha256s) != 0 && len(sha256s) != len(md5s) {
		return ChangesFilesHashMap{}, errors.New("Mismatch in hashes count")
	}
	for i := range md5s {
		_, ok := sha256s[i]
		if !ok {
			return ChangesFilesHashMap{}, errors.New("Mismatch in hashes count")
		}
	}

	for i, v := range md5s {
		s1, ok := sha1s[i]
		if ok {
			v["sha1"] = s1["sha1"]
		}
		s256, ok := sha256s[i]
		if ok {
			v["sha256"] = s256["sha256"]
		}
		md5s[i] = v
	}

	return md5s, nil
}

var changesFileRequiredFields = []string{
	"Format",
	"Date",
	"Source",
	"Binary",
	"Architecture",
	"Version",
	"Distribution",
	"Maintainer",
	"Description",
	"Changes",
	"Files",
}

// ParseDebianChanges parses a debian chnages file into a ChangesFile object
// and verify any signature against keys in GPG keyring kr.
func ParseDebianChanges(r io.Reader, kr openpgp.EntityList) (p ChangesFile, err error) {
	controlFile, err := ParseDebianControl(r, kr)
	if err != nil {
		return ChangesFile{}, err
	}
	if len(controlFile.Data) != 1 {
		return ChangesFile{}, errors.New("Wrong number of paragraphs in change file")
	}

	control := controlFile.Data[0]

	for _, key := range changesFileRequiredFields {
		_, ok := control.GetValues(key)
		if !ok {
			return ChangesFile{}, fmt.Errorf("Changes is missing the %s mandatory field", key)
		}
	}

	files, err := changesFileHashes(control)
	if err != nil {
		return ChangesFile{}, err
	}

	dateStr, _ := control.GetValue("Date")
	date, err := ParseDebianDate(dateStr)
	if err != nil {
		return ChangesFile{}, err
	}

	archsStr, _ := control.GetValue("Architecture")
	archs := strings.Split(archsStr, " ")

	binaryStr, _ := control.GetValue("Binary")
	binaries := strings.Split(binaryStr, " ")

	versionStr, _ := control.GetValue("Version")
	version, err := ParseDebVersion(versionStr)

	sourceStr, _ := control.GetValue("Source")
	source := sourceStr
	sourceVersion := version

	srcVerStart := strings.Index(source, "(")
	if srcVerStart != -1 {
		srcVerEnd := strings.Index(source[srcVerStart+1:], ")")
		if srcVerEnd == -1 {
			return ChangesFile{}, errors.New("Corrupt Source field")
		}

		var err error
		sourceVersion, err = ParseDebVersion(source[srcVerStart+1 : srcVerStart+1+srcVerEnd])
		if err != nil {
			return ChangesFile{}, errors.New("Corrupt Source version")
		}

		source = strings.TrimRight(source[:srcVerStart], " ")
	}

	return ChangesFile{
		Control:       controlFile,
		FileHashes:    files,
		Date:          date,
		Architectures: archs,
		Binaries:      binaries,
		BinaryVersion: version,
		Source:        source,
		SourceVersion: sourceVersion,
	}, nil
}

// ChangesFromHTTPRequest seperates a changes file from any other files in a
// http request
func ChangesFromHTTPRequest(r *http.Request) (
	changesReader io.ReadCloser,
	other []*multipart.FileHeader,
	err error) {

	mimeMemoryBufferSize := int64(64000000)
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

// LoneChanges generates a changes file that covers this package
func LoneChanges(pkg DebPackageInfoer, fileName, dist string) (*ChangesFile, error) {
	name, err := pkg.Name()
	if err != nil {
		return nil, err
	}

	md5, _ := pkg.Md5()
	sha1, _ := pkg.Sha1()
	sha256, _ := pkg.Sha256()
	size, _ := pkg.Size()
	maintainer, _ := pkg.Maintainer()
	version, _ := pkg.Version()

	ctrlFile, _ := pkg.Control()
	origPara := ctrlFile.Data[0]
	section, _ := origPara.GetValue("Section")
	priority, _ := origPara.GetValue("Priority")
	arch, _ := origPara.GetValue("Architecture")
	desc, _ := origPara.GetValue("Description")

	newPara := ControlParagraph{}

	newPara.AddValue("Format", "1.8")
	newPara.AddValue("Date", DebFormatTime(time.Now()))
	newPara.AddValue("Source", name)
	newPara.AddValue("Binary", name)
	newPara.AddValue("Architecture", arch)
	newPara.AddValue("Version", version.String())
	newPara.AddValue("Maintainer", maintainer)
	newPara.AddValue("Distribution", dist)
	newPara.AddValue("Description", "")
	newPara.AddValue("Description", fmt.Sprintf("%s - %s", name, desc))
	newPara.AddValue("Changes", "")

	newPara.SetValue("Files", "")
	newPara.AddValue("Files", fmt.Sprintf("%s %d %s %s %s",
		hex.EncodeToString(md5),
		size,
		section,
		priority,
		fileName))
	newPara.SetValue("Checksums-Sha1", "")
	newPara.AddValue("Checksums-Sha1", fmt.Sprintf("%s %d %s",
		hex.EncodeToString(sha1),
		size,
		fileName))
	newPara.SetValue("Checksums-Sha256", "")
	newPara.AddValue("Checksums-Sha256", fmt.Sprintf("%s %d %s",
		hex.EncodeToString(sha256),
		size,
		fileName))

	return &ChangesFile{
		Date:          time.Now(),
		Source:        name,
		SourceVersion: version,
		Binaries:      []string{name},
		BinaryVersion: version,
		Control:       ControlFile{Data: []*ControlParagraph{&newPara}},
		FileHashes: ChangesFilesHashMap{
			ChangesFilesIndex{
				Name: fileName,
				Size: size,
			}: {"md5": md5, "sha1": sha1, "sha256": sha256},
		},
	}, nil
}

// FormatChangesFile outputs a debian changes file
func FormatChangesFile(w io.Writer, c *ChangesFile) {
	changesStartFields := []string{"Format", "Date", "Source", "Binary"}
	changesEndFields := []string{"Checksums-Sha1", "Checksums-Sha256", "Files"}

	WriteDebianControl(w, c.Control, changesStartFields, changesEndFields)
}
