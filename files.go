package main

import (
	"io"
)

type DebFile struct {
	Filename string
	r        io.Reader
}
