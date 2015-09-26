package deb

import (
	"encoding/hex"
	"reflect"
	"strings"
	"testing"
	"time"
)

var testParseChanges = []struct {
	s      string
	err    string
	expect ChangesFile
}{
	{testChangesInvalid, "Invalid input", ChangesFile{}},
	{testChangesMissing, "Missing field", ChangesFile{}},
	{testChanges1, "", testChanges1Result},
	{testChanges2, "", testChanges1Result},
}

func TestParseChanges(t *testing.T) {
	for i, tt := range testParseChanges {
		c, err := ParseChanges(strings.NewReader(tt.s), nil)
		if err == nil {
			c.Control = ControlFile{}
			if !reflect.DeepEqual(tt.expect, c) {
				t.Errorf("%d. failed:\n  got: %+v\n  expected: %+v\n", i, c, tt.expect)
			}
		} else {
			if tt.err == "" {
				t.Errorf("%d. failed: %q\n", i, err.Error())
			}
		}
	}
}

var testChangesInvalid = `Just some random malformed rubbish`

var testChangesMissing = `Format: 1.8
Date: Fri, 28 Nov 2014 12:00:50 +0000
Version: 1.0.0
Distribution: UNRELEASED
Description: 
 whacky-package - Acme Whacky Web Site
Files: 
 76ec08542b9e2aeba4d6f144d861ec6d 10091104 utils extra whacky-package-assets-1417610953_1.0.0_amd64.deb`

var testChanges1 = `Format: 1.8
Date: Fri, 28 Nov 2014 12:00:50 +0000
Source: whacky-package (1.0.0-1)
Binary: whacky-package whacky-package-assets whacky-package-assets-1417610953
Architecture: source amd64
Version: 1.0.0-2
Distribution: UNRELEASED
Urgency: low
Maintainer: Tristan Colgate-McFarlane <TristanC@acme.com>
Changed-By: michael <michael@acme.com>
Description: 
 whacky-package - Acme Whacky Web Site
 whacky-package-assets - Virtual package for Acme Whacky Web Site Assets
 whacky-package-assets-1417610953 - Acme Whacky Web Site Assets
Changes: 
 whacky-package (1.0.0) UNRELEASED; urgency=low
 .
   * Initial release.
Checksums-Sha1: 
 3c74c3f289c51f85237fcd40e5d8c4371c33cfe3 744 whacky-package_1.0.0.dsc
 778f77115127a934afbf1d2ac7086eaeb3adced9 38524512 whacky-package_1.0.0.tar.gz
 455f8c36ffa6d09c9b4edd265db04aa4661a1955 54960116 whacky-package_1.0.0_amd64.deb
 edb564f5ede2252aa510af0e67bf649c56e6a357 1050 whacky-package-assets_1.0.0_amd64.deb
Checksums-Sha256: 
 fee3db6b1e065c91fd78647c708167269a4e4cda1a8315f793cc0f9a5fd128bb 744 whacky-package_1.0.0.dsc
 7f1df53706f434b762f274c7b1ad84bdb624681350fdbf009f4afff33f1bf5c7 38524512 whacky-package_1.0.0.tar.gz
 00846c0d0038cd8927959ea5d3c5a5b87eb7c76a74178e0dd7fd8b983be4e201 54960116 whacky-package_1.0.0_amd64.deb
 23cfa1d5f5c7bd392278472343d01cf05f9d462d834c92a48ff0c44ecf6b7932 1050 whacky-package-assets_1.0.0_amd64.deb
Files: 
 2c41fac0eea1b8418ee6e1017aa86851 744 utils extra whacky-package_1.0.0.dsc
 2ff43945d0c4610f79dd23146708f196 38524512 utils extra whacky-package_1.0.0.tar.gz
 3d64879e2934e24fd8c4a4b2a379bb8d 54960116 utils extra whacky-package_1.0.0_amd64.deb
 eec26a9aac712cb265dde5faf079ab26 1050 utils extra whacky-package-assets_1.0.0_amd64.deb`

var testChanges1Result = (func() ChangesFile {
	f1md5, _ := hex.DecodeString("2c41fac0eea1b8418ee6e1017aa86851")
	f2md5, _ := hex.DecodeString("2ff43945d0c4610f79dd23146708f196")
	f3md5, _ := hex.DecodeString("3d64879e2934e24fd8c4a4b2a379bb8d")
	f4md5, _ := hex.DecodeString("eec26a9aac712cb265dde5faf079ab26")

	f1sha1, _ := hex.DecodeString("3c74c3f289c51f85237fcd40e5d8c4371c33cfe3")
	f2sha1, _ := hex.DecodeString("778f77115127a934afbf1d2ac7086eaeb3adced9")
	f3sha1, _ := hex.DecodeString("455f8c36ffa6d09c9b4edd265db04aa4661a1955")
	f4sha1, _ := hex.DecodeString("edb564f5ede2252aa510af0e67bf649c56e6a357")

	f1sha256, _ := hex.DecodeString("fee3db6b1e065c91fd78647c708167269a4e4cda1a8315f793cc0f9a5fd128bb")
	f2sha256, _ := hex.DecodeString("7f1df53706f434b762f274c7b1ad84bdb624681350fdbf009f4afff33f1bf5c7")
	f3sha256, _ := hex.DecodeString("00846c0d0038cd8927959ea5d3c5a5b87eb7c76a74178e0dd7fd8b983be4e201")
	f4sha256, _ := hex.DecodeString("23cfa1d5f5c7bd392278472343d01cf05f9d462d834c92a48ff0c44ecf6b7932")

	date, _ := time.Parse("Mon, 02 Jan 2006 15:04:05 -0700", "Fri, 28 Nov 2014 12:00:50 +0000")

	return ChangesFile{
		Date:          date,
		Architectures: []string{"source", "amd64"},
		Binaries:      []string{"whacky-package", "whacky-package-assets", "whacky-package-assets-1417610953"},
		BinaryVersion: MustParseVersion("1.0.0-2"),
		Source:        "whacky-package",
		SourceVersion: MustParseVersion("1.0.0-1"),
		FileHashes: ChangesFilesHashMap{
			ChangesFilesIndex{Name: "whacky-package_1.0.0.dsc", Size: 744}: ChangesFilesHashSet{
				"md5":    f1md5,
				"sha1":   f1sha1,
				"sha256": f1sha256,
			},
			ChangesFilesIndex{Name: "whacky-package_1.0.0.tar.gz", Size: 38524512}: ChangesFilesHashSet{
				"md5":    f2md5,
				"sha1":   f2sha1,
				"sha256": f2sha256,
			},
			ChangesFilesIndex{Name: "whacky-package_1.0.0_amd64.deb", Size: 54960116}: ChangesFilesHashSet{
				"md5":    f3md5,
				"sha1":   f3sha1,
				"sha256": f3sha256,
			},
			ChangesFilesIndex{Name: "whacky-package-assets_1.0.0_amd64.deb", Size: 1050}: ChangesFilesHashSet{
				"md5":    f4md5,
				"sha1":   f4sha1,
				"sha256": f4sha256,
			},
		},
	}
})()

var testChanges2 = `-----BEGIN PGP SIGNED MESSAGE-----
Hash: SHA1

Format: 1.8
Date: Fri, 28 Nov 2014 12:00:50 +0000
Source: whacky-package (1.0.0-1)
Binary: whacky-package whacky-package-assets whacky-package-assets-1417610953
Architecture: source amd64
Version: 1.0.0-2
Distribution: UNRELEASED
Urgency: low
Maintainer: Tristan Colgate-McFarlane <TristanC@acme.com>
Changed-By: michael <michael@acme.com>
Description: 
 whacky-package - Acme Whacky Web Site
 whacky-package-assets - Virtual package for Acme Whacky Web Site Assets
 whacky-package-assets-1417610953 - Acme Whacky Web Site Assets
Changes: 
 whacky-package (1.0.0) UNRELEASED; urgency=low
 .
   * Initial release.
Checksums-Sha1: 
 3c74c3f289c51f85237fcd40e5d8c4371c33cfe3 744 whacky-package_1.0.0.dsc
 778f77115127a934afbf1d2ac7086eaeb3adced9 38524512 whacky-package_1.0.0.tar.gz
 455f8c36ffa6d09c9b4edd265db04aa4661a1955 54960116 whacky-package_1.0.0_amd64.deb
 edb564f5ede2252aa510af0e67bf649c56e6a357 1050 whacky-package-assets_1.0.0_amd64.deb
Checksums-Sha256: 
 fee3db6b1e065c91fd78647c708167269a4e4cda1a8315f793cc0f9a5fd128bb 744 whacky-package_1.0.0.dsc
 7f1df53706f434b762f274c7b1ad84bdb624681350fdbf009f4afff33f1bf5c7 38524512 whacky-package_1.0.0.tar.gz
 00846c0d0038cd8927959ea5d3c5a5b87eb7c76a74178e0dd7fd8b983be4e201 54960116 whacky-package_1.0.0_amd64.deb
 23cfa1d5f5c7bd392278472343d01cf05f9d462d834c92a48ff0c44ecf6b7932 1050 whacky-package-assets_1.0.0_amd64.deb
Files: 
 2c41fac0eea1b8418ee6e1017aa86851 744 utils extra whacky-package_1.0.0.dsc
 2ff43945d0c4610f79dd23146708f196 38524512 utils extra whacky-package_1.0.0.tar.gz
 3d64879e2934e24fd8c4a4b2a379bb8d 54960116 utils extra whacky-package_1.0.0_amd64.deb
 eec26a9aac712cb265dde5faf079ab26 1050 utils extra whacky-package-assets_1.0.0_amd64.deb
-----BEGIN PGP SIGNATURE-----
Version: GnuPG v1

iQEcBAEBAgAGBQJUjuXQAAoJEK3l95w8qDsZBDgH/RT5WjnCBwJyTDgztcwFP5xD
i1wTN/tQDx4pYQLv8JLXshKbLUDTEDuMX3HI2i7hTmyAC7DlY7fEnMEZTi/ChY2+
y/ee8qscLXEJmBqD7wArDYWkoBtIl6D5ZeYnOrdbwBJ5bWfZs+ne51ts7f74OrKs
bOVBL3yam9VyhNZgq7Bwy+Ij+szUPGgFKXBMWByRjckkSJT3X4IPSlGArPJEiPI+
7SLYG7cyTH3Ggje/iOZ3SIMdimHljrcFOmk18/Dn5mPj9Fj9wjOaCNE+wcM0KCHB
fe0I0vF5RM/rGCWMSEY/Co1j7BICi3AleOJUQpFtmJhb6A3r1EBc9qJNt1LhyAA=
=p4tl
-----END PGP SIGNATURE-----`
