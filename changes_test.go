package main

import (
	"strings"
	"testing"

	"code.google.com/p/go.crypto/openpgp"
)

var testParseDebianChanges = []struct {
	s      string
	krStr  string
	expect map[string]ChangesItem
	signed bool
	valid  bool
}{
	{testInvalidUnsigned, "", nil, false, false},
	{testUnsigned, "", testResult, false, false},
	{testSignedValid, "", testResult, true, false},
	{testSignedValid, testKrStr, testResult, true, true},
	{testSignedInvalid, testKrStr, testResult, true, false},
}

func TestParseDebianChanges(t *testing.T) {
	for i, tt := range testParseDebianChanges {
		var krp *openpgp.KeyRing
		if tt.krStr != "" {
			kreader := strings.NewReader(tt.krStr)
			el, _ := openpgp.ReadArmoredKeyRing(kreader)
			kr := openpgp.KeyRing(el)
			krp = &kr
		}
		c, err := ParseDebianChanges(strings.NewReader(tt.s), krp)
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
			if !(c == nil && tt.expect == nil) {
				t.Errorf("%d. failed: %q\n", i, err.Error())
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
var testResult = map[string]ChangesItem{
	"collectd_5.4.0-3.dsc": ChangesItem{
		Filename: "collectd_5.4.0-3.dsc",
		Size:     "2416",
		Md5:      "cd9aa41b337352fd160f326523a9c3d8",
		Sha1:     "7670839693da39075134c1a1f5faad6623df70af",
		Sha256:   "c0679d60f28ceecd09b3c000361c691e373dba599e3878135bc36bede14e109d",
	},
	"collectd_5.4.0-3.diff.gz": ChangesItem{
		Filename: "collectd_5.4.0-3.diff.gz",
		Size:     "76417",
		Md5:      "d1270867f1c9517dd92016ea9f2d5afe",
		Sha1:     "8f06307bf1c17b83351fccdd7a93b4a822ecbdf4",
		Sha256:   "e6d7f21737d2146a9bb30a46137fbd0f00be7971e8c3edc6e66a5981498a261e",
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
