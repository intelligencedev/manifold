# Build agentd with Whisper (CPU-only) for Linux (usable on macOS hosts via Docker Desktop)
# This Dockerfile mirrors the structure of linux.Dockerfile but builds whisper.cpp/ggml without CUDA/Metal (CPU backend only).

FROM ubuntu:22.04 AS builder

ARG GO_VERSION=1.24.5
ENV DEBIAN_FRONTEND=noninteractive

# Base toolchain + Node 20 + pnpm
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential git cmake pkg-config curl ca-certificates \
    python3 python3-distutils openssl gnupg \
  && rm -rf /var/lib/apt/lists/*

# Install Go (explicit 1.24.x)
RUN curl -fsSL https:/go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz | tar -C /usr/local -xz
ENV PATH=/usr/local/go/bin:/go/bin:${PATH} \
    GOPATH=/go

# Node 20 + pnpm@9
RUN curl -fsSL https:/deb.nodesource.com/setup_20.x | bash - \
  && apt-get update && apt-get install -y --no-install-recommends nodejs \
  && corepack enable \
  && corepack prepare pnpm@9 --activate \
  && rm -rf /var/lib/apt/lists/*

WORKDIR /src/app

# Prime go mod cache first
COPY go.mod go.sum ./
RUN go mod download

# Bring the full workspace
COPY . .

# Ensure whisper.cpp submodule is present (fallback clone if missing)
RUN set -e; \
    git config --global url."https:/github.com/".insteadOf "git@github.com:"; \
    if [ ! -f external/whisper.cpp/CMakeLists.txt ]; then \
        echo "Initializing whisper.cpp submodule..."; \
        (git submodule update --init --recursive || true); \
    fi; \
    if [ ! -f external/whisper.cpp/CMakeLists.txt ]; then \
        echo "Submodule missing; shallow clone whisper.cpp"; \
        rm -rf external/whisper.cpp; \
        git clone --depth 1 https:/github.com/ggerganov/whisper.cpp.git external/whisper.cpp; \
    fi

# Build frontend assets and embed
# Use npm instead of pnpm to avoid platform-specific lockfile issues
RUN rm -rf node_modules pnpm-lock.yaml web/agentd-ui/node_modules \
  && cd web/agentd-ui \
  && npm install \
  && npm run build \
  && mkdir -p /src/app/internal/webui/dist \
  && rm -rf /src/app/internal/webui/dist/* \
  && cp -R /src/app/web/agentd-ui/dist/. /src/app/internal/webui/dist/

# Build whisper.cpp with CPU backend only (static libs)
ENV WHISPER_CPP_DIR=/src/app/external/whisper.cpp \
    WHISPER_BUILD_DIR=/src/app/external/whisper.cpp/build_go
RUN rm -rf ${WHISPER_BUILD_DIR} \
  && cmake -S ${WHISPER_CPP_DIR} -B ${WHISPER_BUILD_DIR} \
      -DCMAKE_BUILD_TYPE=Release \
      -DBUILD_SHARED_LIBS=OFF \
      -DGGML_CPU=ON \
      -DGGML_OPENMP=OFF \
      -DGGML_CUDA=OFF \
      -DGGML_METAL=OFF \
      -DWHISPER_BUILD_EXAMPLES=OFF \
      -DWHISPER_BUILD_TESTS=OFF \
      -DGGML_BUILD_TESTS=OFF \
  && cmake --build ${WHISPER_BUILD_DIR} --target whisper --config Release -j1

# CGO include/link paths for CPU backend (no CUDA/Metal)
ENV CPATH=${WHISPER_CPP_DIR}:${WHISPER_CPP_DIR}/include:${WHISPER_CPP_DIR}/ggml/include \
    CGO_CPPFLAGS="-I${WHISPER_CPP_DIR} -I${WHISPER_CPP_DIR}/include -I${WHISPER_CPP_DIR}/ggml/include" \
    CGO_CFLAGS="-I${WHISPER_CPP_DIR} -I${WHISPER_CPP_DIR}/include -I${WHISPER_CPP_DIR}/ggml/include" \
    CGO_LDFLAGS="-L${WHISPER_BUILD_DIR}/src -L${WHISPER_BUILD_DIR}/ggml/src \
                 -lwhisper -lggml -lggml-cpu -lggml-base \
                 -lstdc++ -lm -ldl -lpthread"

# Build agentd for linux/amd64 with CGO enabled
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=1 GOOS=linux GOARCH=amd64 mkdir -p dist && \
    CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -v -o dist/agentd ./cmd/agentd


# Runtime image (no CUDA runtime needed)
FROM ubuntu:22.04 AS runtime
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates dumb-init \
  && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /src/app/dist/agentd /app/agentd
COPY --from=builder /src/app/example.env /app/example.env
COPY --from=builder /src/app/config.yaml.example /app/config.yaml.example
COPY --from=builder /src/app/docs /app/docs

USER 65532:65532
ENTRYPOINT ["dumb-init", "--", "/app/agentd"]
