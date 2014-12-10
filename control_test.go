package main

import (
	"bytes"
	"strings"
	"testing"
)

var testDebControlPersist = []string{
	"Package: first",
	"fieldb: bit of stuff",
	"fieldc: and other stuff",
	"thiny2: ",
	" val1",
	" val2",
	" val3",
	"Description: thingy with",
	" multiple lines of",
	" stuff",
	"",
	"Package: second",
	"fielda: with a",
	"fieldb: bit of stuff",
	"fieldc: and other stuff",
	"thiny2: ",
	" val1",
	" val2",
	" val3",
	"Description: thingy with",
	" multiple lines of",
	" stuff",
}

// Check that we output unknown fields in a consistant order
func TestDebControlPersist(t *testing.T) {
	inStr := strings.Join(testDebControlPersist, "\n") + "\n"

	paragraphs, err := ParseDebianControl(strings.NewReader(inStr))
	if err != nil {
		t.Errorf("ParseDebianControl failure: %s", inStr)
	}

	var buf bytes.Buffer
	FormatPkgControlFile(&buf, paragraphs)
	outStr := string(buf.Bytes())
	if outStr != inStr {
		t.Errorf("\nExpected:\n%s\nGot:\n%s", inStr, outStr)
	}
}
