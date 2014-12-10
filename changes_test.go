package main

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"code.google.com/p/go.crypto/openpgp"
)

var testParseDebianChanges = []struct {
	s      string
	krStr  string
	expect ChangesFile
	signed bool
	valid  bool
}{
	{testInvalidUnsigned, "", ChangesFile{}, false, false},
	{testUnsigned, "", testResult, false, false},
	{testSignedValid, "", testResult, true, false},
	{testSignedValid, testKrStr, testResult, true, true},
	{testSignedInvalid, testKrStr, testResult, true, false},
}

func TestParseDebianChanges(t *testing.T) {
	for i, tt := range testParseDebianChanges {
		var kr openpgp.EntityList
		if tt.krStr != "" {
			kreader := strings.NewReader(tt.krStr)
			kr, _ = openpgp.ReadArmoredKeyRing(kreader)
		}
		c, err := ParseDebianChanges(strings.NewReader(tt.s), kr)
		if err == nil {
			if tt.expect != nil {
				match := true

				for _, v := range tt.expect {
					var file ChangesItem
					ok := false
					for _, f := range c.Files {
						if v.Filename == f.Filename {
							ok = true
							file = f
							break
						}
					}

					if !ok {
						match = false
						break
					}
					if !reflect.DeepEqual(v, file) {
						match = false
						break
					}
				}

				if match == false {
					t.Errorf("%d. failed:\n  got: %+v\n  expected: %+v\n", i, c.Files, tt.expect)
				}

				if c.signed != tt.signed {
					t.Errorf("%d. Signed flag wrong:\n  got: %t\n  expected: %t\n", i, c.signed, tt.signed)
				}

				if c.validated != tt.valid {
					t.Errorf("%d. Valid flag wrong:\n  got: %t\n  expected: %t\n", i, c.validated, tt.valid)
				}
			}
		} else {
			if !(reflect.DeepEqual(c, ChangesFile{}) && tt.expect == nil) {
				t.Errorf("%d. failed: %q\n", i, err.Error())
			}
		}
	}
}

var testInvalidUnsigned = `Just some random malformed rubbish`

var testUnsigned = `Format: 1.8
Date: Fri, 28 Nov 2014 12:00:50 +0000
Source: whacky-package
Binary: whacky-package whacky-package-assets whacky-package-assets-1417610953
Architecture: source amd64
Version: 1.0.0
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
 e384e065839e4d64b255c14d5c3ba806052fe988 10091104 whacky-package-assets-1417610953_1.0.0_amd64.deb
Checksums-Sha256: 
 fee3db6b1e065c91fd78647c708167269a4e4cda1a8315f793cc0f9a5fd128bb 744 whacky-package_1.0.0.dsc
 7f1df53706f434b762f274c7b1ad84bdb624681350fdbf009f4afff33f1bf5c7 38524512 whacky-package_1.0.0.tar.gz
 00846c0d0038cd8927959ea5d3c5a5b87eb7c76a74178e0dd7fd8b983be4e201 54960116 whacky-package_1.0.0_amd64.deb
 23cfa1d5f5c7bd392278472343d01cf05f9d462d834c92a48ff0c44ecf6b7932 1050 whacky-package-assets_1.0.0_amd64.deb
 997a5b2571954a6ca16802e7ff0f15672310a106e773ccb02684a467592495c4 10091104 whacky-package-assets-1417610953_1.0.0_amd64.deb
Files: 
 2c41fac0eea1b8418ee6e1017aa86851 744 utils extra whacky-package_1.0.0.dsc
 2ff43945d0c4610f79dd23146708f196 38524512 utils extra whacky-package_1.0.0.tar.gz
 3d64879e2934e24fd8c4a4b2a379bb8d 54960116 utils extra whacky-package_1.0.0_amd64.deb
 eec26a9aac712cb265dde5faf079ab26 1050 utils extra whacky-package-assets_1.0.0_amd64.deb
 76ec08542b9e2aeba4d6f144d861ec6d 10091104 utils extra whacky-package-assets-1417610953_1.0.0_amd64.deb`

var testSignedValid = `-----BEGIN PGP SIGNED MESSAGE-----
Hash: SHA1

Checksums-Sha1: 
 7670839693da39075134c1a1f5faad6623df70af 2416 collectd_5.4.0-3.dsc
 8f06307bf1c17b83351fccdd7a93b4a822ecbdf4 76417 collectd_5.4.0-3.diff.gz
Checksums-Sha256: 
 c0679d60f28ceecd09b3c000361c691e373dba599e3878135bc36bede14e109d 2416 collectd_5.4.0-3.dsc
 e6d7f21737d2146a9bb30a46137fbd0f00be7971e8c3edc6e66a5981498a261e 76417 collectd_5.4.0-3.diff.gz
Files: 
 cd9aa41b337352fd160f326523a9c3d8 2416 utils optional collectd_5.4.0-3.dsc
 d1270867f1c9517dd92016ea9f2d5afe 76417 utils optional collectd_5.4.0-3.diff.gz
-----BEGIN PGP SIGNATURE-----
Version: GnuPG v1

iQEcBAEBAgAGBQJTiG5VAAoJEK9K5quiVC/K9IkH/3nBYgl89/DnjI6ESksYrtkT
Zv/7Cpfg7YsvUmMrghh1hWklK8zj2Tm3NgKshW/HF7orn5cmPUMVZZU8EFa5uR43
FEJ+r1zA40dcOUtKgRESke4b8MNebWq01Op7zrlU1w/4fZ2MFxhNiQ4Xr3ziEl61
kPPR+1ZG43h+wy1h6QXzcNdqcwUnCfX4Uqlhz1giJ1/1qEdW6HlLOiIomZLGhg6b
K5JmdVY4fiH1Fv0tSq7mVnN7LbXJBo8KyzbqJAGkWNu0zh/G/5whz6n2ohgWC/SJ
uiTJMgKpAOxBFeEzO1quFyWnQePIjQ2zWVaTwqDPiZNQ6+377gCrC4Fu+SYdmlQ=
=JixZ
-----END PGP SIGNATURE-----
`

var testSignedInvalid = `-----BEGIN PGP SIGNED MESSAGE-----
Hash: SHA1

Checksums-Sha1: 
 7670839693da39075134c1a1f5faad6623df70af 2416 collectd_5.4.0-3.dsc
 8f06307bf1c17b83351fccdd7a93b4a822ecbdf4 76417 collectd_5.4.0-3.diff.gz
Checksums-Sha256: 
 c0679d60f28ceecd09b3c000361c691e373dba599e3878135bc36bede14e109d 2416 collectd_5.4.0-3.dsc
 e6d7f21737d2146a9bb30a46137fbd0f00be7971e8c3edc6e66a5981498a261e 76417 collectd_5.4.0-3.diff.gz
Files: 
 cd9aa41b337352fd160f326523a9c3d8 2416 utils optional collectd_5.4.0-3.dsc
 d1270867f1c9517dd92016ea9f2d5afe 76417 utils optional collectd_5.4.0-3.diff.gz
-----BEGIN PGP SIGNATURE-----
Version: GnuPG v1

iQEcBAEBAgAGBQJTiG5VAAoJEK9K5quiVC/K9IkH/3nBYgl89/DnjI6ESksYrtkT
Zv/7Cpfg7YsvUmMrghh1hWklK8zj2Tm3NgKshW/HF7orn5cmPUMVZZU8EFa5uR43
FEJ+r1zA40dcOUtKgRESke4b8MNebWq01Op7zrlU1w/4fZ2MFxhNiQ4Xr3ziEl61
kPPR+1ZG43h+wy1h6QXzcNdqcwUnCfX4Uqlhz1giJ1/1qEdW6HlLOiIomZLGhg6b
K5JmdVY4fiH1Fv0tSq7mVnN7LbXJBo8KyzbqJAFkWNu0zh/G/5whz6n2ohgWC/SJ
uiTJMgKpAOxBFeEzO1quFyWnQePIjQ2zWVaTwqDPiZNQ6+377gCrC4Fu+SYdmlQ=
=JixZ
-----END PGP SIGNATURE-----
`

var testResult = ChangesFile{
	Original:     []byte(""),
	Date:         time.Time{},
	Distribution: "UNRELEASED",
	Maintainer:   "Tristan Colgate-McFarlane <TristanC@acme.com>",
	ChangedBy:    "michael <michael@acme.com>",
	Urgency:      "low",
	Description:  "",
	Closes:       "",
	SignedBy:     "",
	SourceItem: ChangesItem{
		Type:         ChangesSourceItem,
		Name:         "",
		Version:      DebVersion{},
		Architecture: "source",
		Component:    "",
		Suite:        "",
		ControlID:    []byte(""),
		Files: []ChangesItemFile{
			ChangesItemFile{
				Name:             "", // File name as it will appear in the repo
				StoreID:          []byte{},
				Size:             0,
				Md5:              []byte{},
				Sha1:             []byte{},
				Sha256:           []byte{},
				SignedBy:         []string{},
				UploadHookResult: HookOutput{},
			},
		},
	},
	BinaryItems: []ChangesItem{
		ChangesItem{
			Type:         ChangesSourceItem,
			Name:         "",
			Version:      DebVersion{},
			Architecture: "source",
			Component:    "",
			Suite:        "",
			ControlID:    []byte(""),
			Files: []ChangesItemFile{
				ChangesItemFile{
					Name:             "whacky-package_1.0.0_amd64.deb", // File name as it will appear in the repo
					StoreID:          []byte{},
					Size:             54960116,
					Md5:              []byte{},
					Sha1:             []byte{},
					Sha256:           []byte{},
					SignedBy:         []string{},
					UploadHookResult: HookOutput{},
				},
			},
		},
		ChangesItem{
			Type:         ChangesSourceItem,
			Name:         "",
			Version:      DebVersion{},
			Architecture: "source",
			Component:    "",
			Suite:        "",
			ControlID:    []byte(""),
			Files: []ChangesItemFile{
				ChangesItemFile{
					Name:             "whacky-package-assets_1.0.0_amd64.deb", // File name as it will appear in the repo
					StoreID:          []byte{},
					Size:             1050,
					Md5:              []byte{},
					Sha1:             []byte{},
					Sha256:           []byte{},
					SignedBy:         []string{},
					UploadHookResult: HookOutput{},
				},
			},
		},
		ChangesItem{
			Type:         ChangesSourceItem,
			Name:         "",
			Version:      DebVersion{},
			Architecture: "source",
			Component:    "",
			Suite:        "",
			ControlID:    []byte(""),
			Files: []ChangesItemFile{
				ChangesItemFile{
					Name:             "whacky-package-assets-1417610953_1.0.0_amd64.deb", // File name as it will appear in the repo
					StoreID:          []byte{},
					Size:             10091104,
					Md5:              []byte{},
					Sha1:             []byte{},
					Sha256:           []byte{},
					SignedBy:         []string{},
					UploadHookResult: HookOutput{},
				},
			},
		},
	},
}

var testKrStr = `-----BEGIN PGP PUBLIC KEY BLOCK-----
Version: GnuPG v1

mQENBFOIYWUBCACzPZm2qE7DrXc+feQ23ll24ZOQD4V7AjFPz1D20iXZqgZIJoQw
40CPu7/WRrJO4lf6Ak/VgeCnijzRuNkDUk2QT8fsNFVgSWxj6FAM1wan0oWqDyR3
yM9RYojQBpzt0uTolE4g4rs5ywWKaWmA6ra0fU6CCxETM+b1UaD+t2NNzOiQFzaZ
UNRs77Anm1/E6Yu3SRCJC5rUI4NZ3YqDUhI3B5OSdXiPQS1CHM6MoUI7fyVWsNJ9
hR/zTVAdB11nN6UAmGKyn36Xt1nIWlSJW/uPzT/sMu+xlfdHCqtzEsOYPzPNLVc4
YGUwEl++NogWZohJpH1FpWRkzpQaEAXodYDNABEBAAG0MVRyaXN0YW4gQ29sZ2F0
ZSAoYSB0ZXN0IGtleSkgPHRjb2xnYXRlQGdtYWlsLmNvbT6JATgEEwECACIFAlOI
YWUCGwMGCwkIBwMCBhUIAgkKCwQWAgMBAh4BAheAAAoJEK9K5quiVC/KtDYIAJ7b
I7jnzaBPjdzIpczdeOLT+AKA75NMXAqI0bWvjNjh2BvkPWcrmVuhYEkFYupiz2Vo
74AnSbd13iqrhtO9Ix9l5mer6Q03hN9JGWPuxWe31E46JWNUwj8pvy9M1WSjGu9I
HgLGef4vNDwMYCgKdTvJ8ckF2U/myUtt69AG3Kbo3xGucjhjGjqeKgSluqirDIl0
OfXKQUaHqKTZOa3MTKDYStycosFfA1ukOHD8wu25F7/m5SR9o8t/yDboHm2ClO67
1xIowZtFfN1inRsONWA9lC3fHeiffu4E6FzbkUq+vxGsS4XIWkSrvTDKDDYJelah
H/nuZh6Jugj+APniCIi5AQ0EU4hhZQEIAMHKn/mYgVJLSeJyIXnbcOqUXAyT6BPW
iF7p29g/y1j6qBWsOf25DyAg9fShBu1Ay3+UkFERgXePc68s0rZG2aRRY8t/kj0p
znx9ePW3wp6NjV4MSez8wzD7VDtFfWjI9HNV0vjre8sdfNbjHVKljg5zCMD8hFCQ
Tk5tMLAyBnaZTg7qWq9m2l+lO6ODlyQlfmN/4F9c8dvt+oRKvPRo2r6itg7CaX9Y
0+Ab9IvX8dIiqqJRZEQ6VHeYFUy1cdePx3ZUqgYkmVLwT7vXJlD7M4MtoYjWP9F9
/dmwzxgM/duKCdURI4hs3y2mAibRM5O9TaVvZhLqdj67W8bO94E6+JsAEQEAAYkB
HwQYAQIACQUCU4hhZQIbDAAKCRCvSuarolQvymmZB/9Mq/wmmy0DKHov2kJ9ZdLb
ULTghTMPHznVn5lrTu4DuBqocgw7fwLwTwYHp2gy4naJtOq7hhLhWmwsRW7C+51H
wl9Lz93KPPPI0g5tBy8tqq6wcUfmnsD71SKeBqd1v0TcuKNzj++pi/oGmfkS2d2b
Loj/6OElbUyzhAbXunJcZ/aBm+u5DPqYDr1XXg/8bp5UZ7h9vYuxPLGRyhSPdL7I
E6ZHucq5u1AleysWUUt4llmcJ69jtFK69PV4+Q1It+iFAWrS+H2kwMsZAUir2edO
7uk10RWxE1m3xhzU13Hke3osxYrN6BWViZzGUEeXRYk+DfSVegNT7dm92upzixMA
=vk0u
-----END PGP PUBLIC KEY BLOCK-----`
