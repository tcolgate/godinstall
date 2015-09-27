package hasher

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestHasher(t *testing.T) {
	for i, tt := range []struct {
		bs     []byte
		md5    string
		sha1   string
		sha256 string
	}{
		{[]byte(""),
			"1B2M2Y8AsgTpgAmY7PhCfg==",
			"2jmj7l5rSw0yVb/vlWAYkK/YBwk=",
			"47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU="},
		{[]byte("hello work"),
			"S4eyPsUBH8SjPrwck5M7WA==",
			"qxSTuwGkGad0BViNZWTGuRbzqpA=",
			"WHq5PMIXLp8fUucQ2KMmZcs7iT0bA1E+/QTcOGvu38M="},
	} {
		var b bytes.Buffer
		h := New(&b)
		h.Write(tt.bs)

		if bytes.Compare(tt.bs, b.Bytes()) != 0 {
			t.Errorf("%v: Input to hash did not match output", i)
		}
		if cnt := h.Count(); cnt != int64(len(tt.bs)) {
			t.Errorf("%v Expected count of %v, got %v", i, len(tt.bs), cnt)
		}
		if str := base64.StdEncoding.EncodeToString(h.MD5Sum()); str != tt.md5 {
			t.Errorf("%v Expected md5 %v, got %v", i, tt.md5, str)
		}
		if str := base64.StdEncoding.EncodeToString(h.SHA1Sum()); str != tt.sha1 {
			t.Errorf("%v Expected sha1 %v, got %v", i, tt.sha1, str)
		}
		if str := base64.StdEncoding.EncodeToString(h.SHA256Sum()); str != tt.sha256 {
			t.Errorf("%v Expected sha256%v, got %v", i, tt.sha256, str)
		}
	}
}
