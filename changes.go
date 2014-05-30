package main

import (
	//"code.google.com/p/go.crypto/openpgp"
	//"code.google.com/p/go.crypto/openpgp/armor"
	"github.com/stapelberg/godebiancontrol"
	"io"
)

type DebChanges struct {
	p []godebiancontrol.Paragraph
}

func ParseDebianChanges(r io.Reader) (*DebChanges, error) {
	var changes DebChanges
	var err error

	changes.p, err = godebiancontrol.Parse(r)

	return &changes, err
}
