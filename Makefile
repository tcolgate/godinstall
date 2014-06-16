CWD=$(shell pwd)

export GOPATH=${CWD}/build
BINDIR=${GOPATH}/bin
SRCDIR=${GOPATH}/src/github.com/tcolgate/godinstall/

all: ${SRCDIR}/godinstall

${GOPATH}: *.go
	mkdir ${GOPATH}
	mkdir -p ${GOPATH}/pkg
	mkdir -p ${BINDIR}
	mkdir -p ${SRCDIR}
	cp *.go ${SRCDIR}

${BINDIR}/godep: ${GOPATH}
	cd ${SRCDIR}
	go get github.com/tools/godep

${SRCDIR}/godinstall:  ${BINDIR}/godep 
	cd ${SRCDIR}
	GOPATH=${GOPATH} godep go build

install: ${SRCDIR}/godinstall
	cd ${SRCDIR}
	install godinstall /usr/bin/godinstall

clean:
	rm -rf build
