package main

import (
	"strings"
	"testing"

	"code.google.com/p/go.crypto/openpgp"
)

var testParseDebianChanges = []struct {
	s      string
	kr     *openpgp.KeyRing
	expect map[string]ChangesFile
}{
	{testInvalidUnsigned, nil, nil},
	{testUnsigned, nil, testUnsignedResult},
}

func TestParseDebianChanges(t *testing.T) {
	for i, tt := range testParseDebianChanges {
		c, err := ParseDebianChanges(strings.NewReader(tt.s), tt.kr)
		if err == nil {
			if tt.expect != nil {
				match := true

				for k, v := range tt.expect {
					f, ok := c.Files[k]

					if !ok {
						match = false
						break
					}
					if v != *f {
						match = false
						break
					}
				}

				if match == false {
					t.Errorf("$d. failed:\n  got: %q\n  expected: %q\n", i, c.Files, tt.expect)
				}
			}
		} else {
			if !(c == nil && tt.expect == nil) {
				t.Errorf("$d. failed: %q\n", i, err.Error())
			}
		}
	}
}

var testInvalidUnsigned = `Just some random malformed rubbish`
var testUnsigned = `Checksums-Sha1: 
 7670839693da39075134c1a1f5faad6623df70af 2416 collectd_5.4.0-3.dsc
 8f06307bf1c17b83351fccdd7a93b4a822ecbdf4 76417 collectd_5.4.0-3.diff.gz
Checksums-Sha256: 
 c0679d60f28ceecd09b3c000361c691e373dba599e3878135bc36bede14e109d 2416 collectd_5.4.0-3.dsc
 e6d7f21737d2146a9bb30a46137fbd0f00be7971e8c3edc6e66a5981498a261e 76417 collectd_5.4.0-3.diff.gz
Files: 
 cd9aa41b337352fd160f326523a9c3d8 2416 utils optional collectd_5.4.0-3.dsc
 d1270867f1c9517dd92016ea9f2d5afe 76417 utils optional collectd_5.4.0-3.diff.gz
 `
var testUnsignedResult = map[string]ChangesFile{
	"collectd_5.4.0-3.dsc": ChangesFile{
		Filename: "collectd_5.4.0-3.dsc",
		Size:     "2416",
		Md5:      "cd9aa41b337352fd160f326523a9c3d8",
		Sha1:     "7670839693da39075134c1a1f5faad6623df70af",
		Sha256:   "c0679d60f28ceecd09b3c000361c691e373dba599e3878135bc36bede14e109d",
	},
	"collectd_5.4.0-3.diff.gz": ChangesFile{
		Filename: "collectd_5.4.0-3.diff.gz",
		Size:     "76417",
		Md5:      "d1270867f1c9517dd92016ea9f2d5afe",
		Sha1:     "8f06307bf1c17b83351fccdd7a93b4a822ecbdf4",
		Sha256:   "e6d7f21737d2146a9bb30a46137fbd0f00be7971e8c3edc6e66a5981498a261e",
	},
}
