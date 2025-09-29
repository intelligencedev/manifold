SHELL := /bin/bash

# Binaries are discovered under cmd/*
## Discover cmd/* directories but exclude embedctl	@echo "Host build complete"

# Build TUI binary for host platform (fast - only rebuild whisper if needed)ger ship the embedctl CLI)
BINS := $(shell for d in cmd/*; do if [ -d "$$d" ]; then bn=$$(basename $$d); if [ "$$bn" != "embedctl" ]; then echo $$bn; fi; fi; done)

# Output directory
DIST := dist

# Platforms to build for in cross (Option A: single job builds all platforms)
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

GOLANGCI_LINT_VERSION := v1.59.0

# Whisper.cpp paths
WHISPER_CPP_DIR := external/whisper.cpp
WHISPER_BUILD_DIR := $(WHISPER_CPP_DIR)/build_go
WHISPER_BINDINGS_DIR := $(WHISPER_CPP_DIR)/bindings/go
WHISPER_INCLUDE_DIR := $(WHISPER_CPP_DIR)/include
WHISPER_LIB_DIR := $(WHISPER_BUILD_DIR)/src
WHISPER_GGML_LIB_DIR := $(WHISPER_BUILD_DIR)/ggml/src

.PHONY: all help fmt fmt-check imports-check vet lint test ci build cross checksums tools clean whisper-cpp whisper-go-bindings build-tui frontend

all: build

help:
	@echo "Available targets:"
	@echo "  make tools              # install dev tools (goimports, golangci-lint)"
	@echo "  make whisper-cpp        # build Whisper.cpp library"
	@echo "  make whisper-go-bindings# build Go bindings for Whisper.cpp"
	@echo "  make fmt                # format code with gofmt"
	@echo "  make fmt-check          # check formatting"
	@echo "  make imports-check      # check imports with goimports"
	@echo "  make vet                # run go vet"
	@echo "  make lint               # run golangci-lint"
	@echo "  make test               # run tests with -race and generate coverage.out"
	@echo "  make build              # build host platform binaries into $(DIST)/ (includes Whisper)"
	@echo "  make build-agentd       # build only the agentd binary (skips Whisper)"
	@echo "  make frontend           # build the Vue.js frontend assets"
	@echo "  make cross              # build all platforms (tar/zip) into $(DIST)/ (includes Whisper)"
	@echo "  make checksums          # generate SHA256 checksums for artifacts in $(DIST)/"
	@echo "  make ci                 # run CI checks (fmt-check, imports-check, vet, lint, test)"
	@echo "  make clean              # clean $(DIST) and coverage.out"

# Install developer tools
tools:
	@echo "Installing goimports and golangci-lint..."
	@echo "Installing goimports"
	GOFLAGS= go install golang.org/x/tools/cmd/goimports@latest
	@echo "Installing golangci-lint $(GOLANGCI_LINT_VERSION)"
	GOFLAGS= go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	@echo "Done"

# Build Whisper.cpp library
whisper-cpp:
	@echo "Building Whisper.cpp library..."
	@if [ ! -d "$(WHISPER_CPP_DIR)" ]; then \
		echo "Error: $(WHISPER_CPP_DIR) not found. Make sure the submodule is initialized."; \
		exit 1; \
	fi
	# Favor Metal backend on macOS runners; disable examples/tests to reduce build
	cd $(WHISPER_CPP_DIR) && \
		CMAKE_ARGS="-DGGML_METAL=ON -DGGML_OPENMP=OFF -DWHISPER_BUILD_EXAMPLES=OFF -DWHISPER_BUILD_TESTS=OFF -DGGML_BUILD_TESTS=OFF" make build
	@echo "Whisper.cpp library built successfully"

# Build Go bindings for Whisper.cpp
whisper-go-bindings: whisper-cpp
	@echo "Building Go bindings for Whisper.cpp (Metal-only)..."
	@if [ ! -d "$(WHISPER_BINDINGS_DIR)" ]; then \
		echo "Error: $(WHISPER_BINDINGS_DIR) not found."; \
		exit 1; \
	fi
	# Configure build_go with Metal backend enabled, BLAS/OpenMP/tests/examples disabled, static libs
	cmake -S $(WHISPER_CPP_DIR) -B $(WHISPER_BUILD_DIR) \
		-DCMAKE_BUILD_TYPE=Release \
		-DBUILD_SHARED_LIBS=OFF \
		-DGGML_METAL=ON \
		-DGGML_CPU=ON \
		-DGGML_OPENMP=OFF \
		-DWHISPER_BUILD_EXAMPLES=OFF \
		-DWHISPER_BUILD_TESTS=OFF \
		-DGGML_BUILD_TESTS=OFF
	# Build only the whisper target which will build required ggml libraries
	cmake --build $(WHISPER_BUILD_DIR) --target whisper --config Release
	@echo "Go bindings built successfully"

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
build: clean whisper-go-bindings | $(DIST)
	@echo "Building host platform binaries into $(DIST)/"
	for b in $(BINS); do \
		out=$(DIST)/$$b; \
		echo "Building $$b -> $$out"; \
		C_INCLUDE_PATH=$(WHISPER_INCLUDE_DIR) \
		LIBRARY_PATH=$(WHISPER_LIB_DIR):$(WHISPER_GGML_LIB_DIR):$(WHISPER_BUILD_DIR)/ggml/src/ggml-blas:$(WHISPER_BUILD_DIR)/ggml/src/ggml-metal \
		go build -o "$$out" ./cmd/$$b; \
	done
	@echo "Host build complete"


# Cross compile all platforms and package them into $(DIST)/
# Option A: single job builds all platforms (do not run cross in parallel across jobs)
cross: clean whisper-go-bindings | $(DIST)
	@echo "Cross-building for: $(PLATFORMS)"
	@echo "Note: CGO-dependent binaries will be skipped in cross-compilation if any are added later"
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
	rm -rf $(WHISPER_CPP_DIR)/build $(WHISPER_CPP_DIR)/build_go $(WHISPER_BINDINGS_DIR)/build_go || true
	@echo "Cleaned"

.PHONY: build-agentd
build-agentd: | $(DIST)
	@echo "Building agentd only into $(DIST)/"
	go build -o $(DIST)/agentd ./cmd/agentd
	@echo "agentd build complete"

# Build only agentd but ensure Whisper dependencies are built and linked (speech-to-text)
.PHONY: build-agentd-whisper
FRONTEND_DIR := web/agentd-ui
FRONTEND_SRC_DIST := $(FRONTEND_DIR)/dist
FRONTEND_EMBED_DIR := internal/webui/dist
PNPM := pnpm

build-agentd-whisper: whisper-go-bindings | $(DIST)
	@echo "Building agentd (with Whisper) into $(DIST)/"
	$(MAKE) frontend
	# Ensure cgo can find headers and link against static libs and Metal frameworks
	CGO_CFLAGS="-I$(WHISPER_INCLUDE_DIR) -I$(WHISPER_CPP_DIR)/ggml/include" \
	CGO_LDFLAGS="-L$(WHISPER_LIB_DIR) -L$(WHISPER_GGML_LIB_DIR) -L$(WHISPER_BUILD_DIR)/ggml/src/ggml-blas -L$(WHISPER_BUILD_DIR)/ggml/src/ggml-metal -lwhisper -lggml -lggml-metal -lggml-blas -lggml-cpu -lggml-base -framework Foundation -framework Metal -framework MetalKit -framework Accelerate" \
	go build -o $(DIST)/agentd ./cmd/agentd
	@echo "agentd (with Whisper) build complete"


# Build web UI server
build-webui:
	@echo "Building webui into $(DIST)/"
	go build -o $(DIST)/webui ./cmd/webui

.PHONY: frontend
frontend:
	@if ! command -v $(PNPM) >/dev/null 2>&1; then \
		echo "pnpm not found; install it from https://pnpm.io/"; \
		exit 1; \
	fi
	@echo "Building frontend in $(FRONTEND_DIR)"
	cd $(FRONTEND_DIR) && $(PNPM) run build
	@if [ ! -d "$(FRONTEND_SRC_DIST)" ]; then \
		echo "Frontend build output not found at $(FRONTEND_SRC_DIST)"; \
		exit 1; \
	fi
	rm -rf $(FRONTEND_EMBED_DIR)
	mkdir -p $(FRONTEND_EMBED_DIR)
	cp -R $(FRONTEND_SRC_DIST)/. $(FRONTEND_EMBED_DIR)/
	@echo "Frontend assets copied into $(FRONTEND_EMBED_DIR)"
