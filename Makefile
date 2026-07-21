.PHONY: build test e2e vet fmt lint vulncheck install uninstall

PREFIX ?= /usr/local

build:
	go build -o isthmos ./cmd/isthmos

test:
	go test ./...

e2e: build
	./scripts/e2e.sh ./isthmos

vet:
	go vet ./...

fmt:
	go fmt ./...

lint:
	golangci-lint run

vulncheck:
	govulncheck ./...

install: build
	install -d $(PREFIX)/bin
	install -m 0755 isthmos $(PREFIX)/bin/isthmos

uninstall:
	rm -f $(PREFIX)/bin/isthmos
