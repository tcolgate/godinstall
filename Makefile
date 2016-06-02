PREFIX=/usr/bin

NAME=godinstall
CWD=$(shell pwd)
VERSION=$(shell sed -n '1s/godinstall (\([0-9.]*\)) .*/\1/p' debian/changelog)
GITREF=$(shell git show-ref -s --abbrev HEAD)

.PHONY: tags clean all install check

all: $(NAME)

version.go: debian/changelog
	echo -e "package main\nvar godinstallVersion = \""$(VERSION)-$(GITREF)\" > version.go

$(NAME): debian/changelog version.go
	go build -o $(NAME)

install: $(NAME)
	install -D $(NAME) $(DESTDIR)/$(PREFIX)/$(NAME)

check:
	go test -v

tags:
	gotags -R . > tags

clean:
	rm -rf build $(NAME)
