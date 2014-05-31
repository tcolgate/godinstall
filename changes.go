package main

import (
	//"code.google.com/p/go.crypto/openpgp"
	"bytes"
	"io"
	"io/ioutil"
	"log"

	"code.google.com/p/go.crypto/openpgp"
	"code.google.com/p/go.crypto/openpgp/clearsign"
	"github.com/stapelberg/godebiancontrol"
)

type DebChanges struct {
	signed    bool
	validated bool
	signedBy  *openpgp.Entity
	p         []godebiancontrol.Paragraph
}

func ParseDebianChanges(r io.Reader, kr *openpgp.KeyRing) (p *DebChanges, err error) {
	var c DebChanges

	b, err := ioutil.ReadAll(r)
	if err != nil {
		return
	}

	msg, rest := clearsign.Decode(b)
	switch {
	case len(msg.Plaintext) == 0 && len(rest) > 0:
		{
			c.signed = false
		}
	case len(msg.Plaintext) > 0:
		{
			c.signed = true
			if len(rest) > 0 {
				log.Println("trailing content in signed control file will be ignored")
			}
		}
	}

	br := bytes.NewReader(rest)

	if c.signed {
		br = bytes.NewReader(msg.Bytes)

		c.signedBy, err = openpgp.CheckDetachedSignature(*kr, br, msg.ArmoredSignature.Body)
		if err == nil {
			c.validated = true
		} else {
			c.validated = false
		}
		br = bytes.NewReader(msg.Plaintext)
	} else {
		c.validated = false
	}

	c.p, err = godebiancontrol.Parse(br)
	// Need to check for parse failing here

	return &c, err
}
