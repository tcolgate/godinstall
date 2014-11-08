PREFIX=/usr/bin

CWD=$(shell pwd)
BINNAME=$(shell basename $(CWD))

export GOPATH=$(CWD)/build
BINDIR=$(GOPATH)/bin

all: godinstall

version.go:
	echo "package main\nvar godinstallVersion = \""`dpkg-parsechangelog --show-field Version`-`git show-ref -s --abbrev HEAD`\" > version.go

$(GOPATH): 
	mkdir $(GOPATH)
	mkdir -p $(GOPATH)/pkg
	mkdir -p $(BINDIR)

$(BINDIR)/godep: $(GOPATH)
	go get github.com/tools/godep

godinstall:  $(BINDIR)/godep version.go
	$(GOPATH)/bin/godep go build
	mv $(BINNAME) godinstall

install: godinstall
	install -D godinstall $(DESTDIR)/$(PREFIX)/godinstall
	install -D godinstall-upload.py $(DESTDIR)/$(PREFIX)/godinstall-upload.py

check:
	$(GOPATH)/bin/godep go test -v

clean:
	rm -rf build godinstall
	rm -f version.go
