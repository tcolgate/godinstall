PREFIX=/usr/bin

NAME=godinstall
CWD=$(shell pwd)
BINNAME=$(shell basename $(CWD))

all: $(NAME)

version.go: debian/changelog
	echo "package main\nvar godinstallVersion = \""`dpkg-parsechangelog | grep ^Version | awk '{print $$2}'   `-`git show-ref -s --abbrev HEAD`\" > version.go

$(NAME): version.go
	GO15VENDOREXPERIMENT=1 go build
	if [ -f $(BINNAME) ]; then test $(BINNAME) -ef $(NAME) || mv -f  $(BINNAME) $(NAME) ; fi

install: $(NAME)
	install -D $(NAME) $(DESTDIR)/$(PREFIX)/$(NAME)

check:
	GO15VENDOREXPERIMENT=1 go test -v

clean:
	rm -f version.go
