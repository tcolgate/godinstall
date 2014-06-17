CWD=$(shell pwd)

PREFIX=/usr/bin

export GOPATH=$(CWD)/build
BINDIR=$(GOPATH)/bin
SRCDIR=$(GOPATH)/src/github.com/tcolgate/godinstall/


all: $(BINDIR)/godinstall

$(GOPATH): *.go
	mkdir $(GOPATH)
	mkdir -p $(GOPATH)/pkg
	mkdir -p $(BINDIR)
	mkdir -p $(SRCDIR)
	cp *.go $(SRCDIR)

$(BINDIR)/godep: $(GOPATH)
	cd $(SRCDIR) && go get github.com/tools/godep

$(BINDIR)/godinstall:  $(BINDIR)/godep 
	cd $(SRCDIR) && $(GOPATH)/bin/godep go install

install: $(BINDIR)/godinstall
	cd $(BINDIR) && install -D godinstall $(DESTDIR)/$(PREFIX)/godinstall

clean:
	rm -rf build
