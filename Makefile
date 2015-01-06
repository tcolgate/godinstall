PREFIX=/usr/bin

NAME=godinstall
CWD=$(shell pwd)
BINNAME=$(shell basename $(CWD))

export GOPATH=$(CWD)/build
BINDIR=$(GOPATH)/bin

all: $(NAME)

version.go: debian/changelog
	echo "package main\nvar godinstallVersion = \""`dpkg-parsechangelog | grep ^Version | awk '{print $$2}'   `-`git show-ref -s --abbrev HEAD`\" > version.go

$(GOPATH):
	mkdir $(GOPATH)
	mkdir -p $(GOPATH)/pkg
	mkdir -p $(BINDIR)

$(BINDIR)/godep: $(GOPATH)
	go get github.com/tools/godep

$(NAME): $(BINDIR)/godep version.go
	$(GOPATH)/bin/godep go build
	if [ -f $(BINNAME) ]; then test $(BINNAME) -ef $(NAME) || mv -f  $(BINNAME) $(NAME) ; fi

install: $(NAME)
	install -D $(NAME) $(DESTDIR)/$(PREFIX)/$(NAME)

check:
	$(GOPATH)/bin/godep go test -v

clean:
	rm -rf build $(NAME)
	rm -f version.go
