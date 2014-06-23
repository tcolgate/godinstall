PREFIX=/usr/bin

CWD=$(shell pwd)
BINNAME=$(shell basename $(CWD))

export GOPATH=$(CWD)/build
BINDIR=$(GOPATH)/bin

all: godinstall

$(GOPATH): 
	mkdir $(GOPATH)
	mkdir -p $(GOPATH)/pkg
	mkdir -p $(BINDIR)

$(BINDIR)/godep: $(GOPATH)
	go get github.com/tools/godep

godinstall:  $(BINDIR)/godep 
	$(GOPATH)/bin/godep go build
	mv $(BINNAME) godinstall

install: godinstall
	install -D godinstall $(DESTDIR)/$(PREFIX)/godinstall

clean:
	rm -rf build godinstall
