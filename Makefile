all: godinstall

godinstall:  *.go
	godep go build

install: godinstall
	install godinstall /usr/bin/godinstall

clean:
	godep go clean
