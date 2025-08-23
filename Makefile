SHELL := /bin/bash

# Binaries are discovered under cmd/*
## Discover cmd/* directories but exclude embedctl (we no longer ship the embedctl CLI)
BINS := $(shell for d in cmd/*; do if [ -d "$$d" ]; then bn=$$(basename $$d); if [ "$$bn" != "embedctl" ]; then echo $$bn; fi; fi; done)

# Output directory
DIST := dist

# Platforms to build for in cross (Option A: single job builds all platforms)
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

GOLANGCI_LINT_VERSION := v1.59.0

.PHONY: all help fmt fmt-check imports-check vet lint test ci build cross checksums tools clean

help:
	@echo "Available targets:"
	@echo "  make tools        # install dev tools (goimports, golangci-lint)"
	@echo "  make fmt          # format code with gofmt"
	@echo "  make fmt-check    # check formatting"
	@echo "  make imports-check# check imports with goimports"
	@echo "  make vet          # run go vet"
	@echo "  make lint         # run golangci-lint"
	@echo "  make test         # run tests with -race and generate coverage.out"
	@echo "  make build        # build host platform binaries into $(DIST)/"
	@echo "  make cross        # build all platforms (tar/zip) into $(DIST)/"
	@echo "  make checksums    # generate SHA256 checksums for artifacts in $(DIST)/"
	@echo "  make ci           # run CI checks (fmt-check, imports-check, vet, lint, test)"
	@echo "  make clean        # clean $(DIST) and coverage.out"

# Install developer tools
tools:
	@echo "Installing goimports and golangci-lint..."
	@echo "Installing goimports"
	GOFLAGS= go install golang.org/x/tools/cmd/goimports@latest
	@echo "Installing golangci-lint $(GOLANGCI_LINT_VERSION)"
	GOFLAGS= go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	@echo "Done"

fmt:
	gofmt -w .

fmt-check:
	@unformatted=$$(gofmt -l .); if [ -n "$$unformatted" ]; then echo "gofmt needs to be run on:"; echo "$$unformatted"; exit 1; fi

imports-check:
	@missing=$$(goimports -l .); if [ -n "$$missing" ]; then echo "goimports needs to be run on:"; echo "$$missing"; exit 1; fi

vet:
	go vet ./...

lint:
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		echo "golangci-lint not found; run 'make tools' or install it in CI"; exit 1; \
	fi
	golangci-lint run --timeout=5m

test:
	@echo "Running tests with race detector and coverage"
	go test -race -coverprofile=coverage.out ./...

ci: fmt-check imports-check vet lint test
	@echo "CI checks passed"

# Build binaries for host platform. Builds each cmd/* directory (if present)
build: clean | $(DIST)
	@echo "Building host platform binaries into $(DIST)/"
	for b in $(BINS); do \
		out=$(DIST)/$$b; \
		echo "Building $$b -> $$out"; \
		go build -o "$$out" ./cmd/$$b; \
	done
	@echo "Host build complete"

# Cross compile all platforms and package them into $(DIST)/
# Option A: single job builds all platforms (do not run cross in parallel across jobs)
cross: clean | $(DIST)
	@echo "Cross-building for: $(PLATFORMS)"
	set -e
	for plat in $(PLATFORMS); do \
		os=$${plat%%/*}; arch=$${plat##*/}; \
		for b in $(BINS); do \
			mkdir -p $(DIST)/$${os}_$${arch}; \
			outfile=$${b}; \
			if [ "$${os}" = "windows" ]; then outfile=$${b}.exe; fi; \
			echo "Building $$b for $${os}/$${arch} -> $(DIST)/$${os}_$${arch}/$$outfile"; \
			CGO_ENABLED=0 GOOS=$${os} GOARCH=$${arch} go build -o "$(DIST)/$${os}_$${arch}/$$outfile" ./cmd/$$b; \
		done; \
		# Package per-platform directory
		pushd $(DIST) >/dev/null; \
		if [ "$${os}" = "windows" ]; then \
			zipname="$${os}-$${arch}.zip"; \
			zip -r "$$zipname" "$${os}_$${arch}" >/dev/null; \
			sha256sum "$$zipname" > "$${zipname}.sha256"; \
			echo "Packaged $$zipname"; \
			# remove per-platform dir after packaging
			rm -rf "$${os}_$${arch}"; \
		else \
			tarname="$${os}-$${arch}.tar.gz"; \
			tar -czf "$$tarname" "$${os}_$${arch}"; \
			sha256sum "$$tarname" > "$${tarname}.sha256"; \
			echo "Packaged $$tarname"; \
			rm -rf "$${os}_$${arch}"; \
		fi; \
		popd >/dev/null; \
	done
	@echo "Cross-build complete. Artifacts are in $(DIST)/"

# Generate a combined checksums file sorted for reproducibility
checksums:
	@echo "Generating combined checksums in $(DIST)/checksums.txt"
	@cd $(DIST) && ls -1 | sort | grep -v "checksums.txt" | xargs -I{} sh -c 'if [ -f "{}" ]; then sha256sum "{}"; fi' > checksums.txt
	@echo "Checksums written to $(DIST)/checksums.txt"

# Ensure dist exists
$(DIST):
	@mkdir -p $(DIST)

clean:
	rm -rf $(DIST) coverage.out || true
	@echo "Cleaned"
