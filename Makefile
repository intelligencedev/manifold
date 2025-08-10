SHELL := /bin/bash

.PHONY: all build test lint

all: build

build:
	go build ./...

test:
	go test ./...

lint:
	./scripts/lint.sh

