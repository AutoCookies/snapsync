BINARY_NAME := snapsync

ifeq ($(OS),Windows_NT)
	BINARY_EXT := .exe
else
	BINARY_EXT :=
endif

.PHONY: test test-race lint build ci

test:
	go test ./...

test-race:
	go test -race ./...

lint:
	golangci-lint run

build:
	mkdir -p bin
	go build -o ./bin/$(BINARY_NAME)$(BINARY_EXT) ./cmd/snapsync

ci: lint test
