BINARY_NAME := snapsync
VERSION ?= dev
COMMIT ?= unknown
DATE ?= unknown
LDFLAGS := -X snapsync/internal/buildinfo.Version=$(VERSION) -X snapsync/internal/buildinfo.Commit=$(COMMIT) -X snapsync/internal/buildinfo.Date=$(DATE)

ifeq ($(OS),Windows_NT)
	BINARY_EXT := .exe
else
	BINARY_EXT :=
endif

.PHONY: test test-race lint build ci release

test:
	go test ./...

test-race:
	go test -race ./...

lint:
	golangci-lint run

build:
	mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o ./bin/$(BINARY_NAME)$(BINARY_EXT) ./cmd/snapsync

release:
	mkdir -p dist
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o ./dist/$(BINARY_NAME)-linux-amd64 ./cmd/snapsync
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o ./dist/$(BINARY_NAME)-windows-amd64.exe ./cmd/snapsync

ci: lint test
