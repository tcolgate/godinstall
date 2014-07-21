package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"code.google.com/p/go.crypto/openpgp"
	"github.com/blakesmith/ar"
	"github.com/stapelberg/godebiancontrol"
)

type DebPackage struct {
	signed    bool            // Whether this changes file signed
	validated bool            //  Whether the signature is valid
	signedBy  *openpgp.Entity // The pgp entity that signed the file
}

func ParseDebPackage(r io.Reader, kr openpgp.EntityList) (p *DebPackage, err error) {
	arReader := ar.NewReader(r)
	var signature bytes.Buffer
	var control bytes.Buffer

	content, err := ioutil.TempFile("", "")
	if err != nil {
		return nil, err
	}

	controlTgz, err := ioutil.TempFile("", "")
	if err != nil {
		return nil, err
	}

	for {
		header, err := arReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		log.Println(header)

		switch {
		case strings.HasPrefix(header.Name, "_gpg"):
			log.Println("signature found")
			io.Copy(&signature, arReader)
		case header.Name == "control.tar.gz":
			io.Copy(
				io.MultiWriter(controlTgz, content),
				arReader)
			controlTgz.Close()
			f, err := os.Open(controlTgz.Name())

			gzReader, err := gzip.NewReader(f)
			if err != nil {
				return nil, err
			}

			tarReader := tar.NewReader(gzReader)
			if err != nil {
				return nil, err
			}

			for {
				tarHeader, err := tarReader.Next()
				if err == io.EOF {
					break
				}
				log.Println(tarHeader)
				if tarHeader.Name == "./control" {
					io.Copy(&control, tarReader)
				}
			}
		default:
			io.Copy(content, arReader)
		}
	}

	if control.Len() == 0 {
		return nil, errors.New("did not file control file in archive")
	}

	log.Println(signature)
	log.Println(control)

	if control.Len() == 0 {
		return nil, errors.New("did not file control file in archive")
	}

	controlReader := bytes.NewReader(control.Bytes())
	paragraphs, err := godebiancontrol.Parse(controlReader)

	log.Println(paragraphs)

	return &DebPackage{}, nil
}
