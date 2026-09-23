BINARY := alerts-cli
PREFIX ?= $(HOME)/.local

.PHONY: build fmt vet check install clean

build:
	go build -o $(BINARY) .

fmt:
	gofmt -w .

vet:
	go vet ./...

check:
	test -z "$$(gofmt -l .)" || { gofmt -l .; exit 1; }
	go vet ./...

install: build
	install -D -m 0755 $(BINARY) $(PREFIX)/bin/$(BINARY)

clean:
	rm -f $(BINARY)
