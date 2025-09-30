# Build agentd with Whisper (CUDA) for Linux amd64
# This Dockerfile mirrors the macOS CI flow but targets Linux with CUDA backend for whisper.cpp/ggml.

FROM nvidia/cuda:12.4.1-devel-ubuntu22.04 AS builder

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

# Node 20 + pnpm@9 (like workflow)
RUN curl -fsSL https:/deb.nodesource.com/setup_20.x | bash - \
    && apt-get update && apt-get install -y --no-install-recommends nodejs \
    && corepack enable \
    && corepack prepare pnpm@9 --activate \
    && rm -rf /var/lib/apt/lists/*

# CUDA env (ensure linker can find CUDA libs during CGO link) 
ENV CUDA_HOME=/usr/local/cuda \
    LD_LIBRARY_PATH=/usr/local/cuda/lib64:${LD_LIBRARY_PATH} \
    LIBRARY_PATH=/usr/local/cuda/lib64:${LIBRARY_PATH}

WORKDIR /src/app

# Prime go mod cache first
COPY go.mod go.sum ./
RUN go mod download

# Bring the full workspace (frontend, cmd, internal, external, etc.)
COPY . .

# Ensure whisper.cpp submodule is present (fallback clone if missing)
RUN set -e; \
    if [ ! -f external/whisper.cpp/CMakeLists.txt ]; then \
      echo "Initializing whisper.cpp submodule..."; \
      (git submodule update --init --recursive || true); \
    fi; \
    if [ ! -f external/whisper.cpp/CMakeLists.txt ]; then \
      echo "Submodule missing; shallow clone whisper.cpp"; \
      rm -rf external/whisper.cpp; \
      git clone --depth 1 https:/github.com/ggerganov/whisper.cpp.git external/whisper.cpp; \
    fi

# Build frontend assets and embed (mirrors Makefile frontend target)
RUN pnpm install --frozen-lockfile \
    && cd web/agentd-ui && pnpm run build \
    && mkdir -p /src/app/internal/webui/dist \
    && rm -rf /src/app/internal/webui/dist/* \
    && cp -R /src/app/web/agentd-ui/dist/. /src/app/internal/webui/dist/

# Build whisper.cpp with CUDA backend (static libs)
ENV WHISPER_CPP_DIR=/src/app/external/whisper.cpp \
    WHISPER_BUILD_DIR=/src/app/external/whisper.cpp/build_go
RUN cmake -S ${WHISPER_CPP_DIR} -B ${WHISPER_BUILD_DIR} \
      -DCMAKE_BUILD_TYPE=Release \
      -DBUILD_SHARED_LIBS=OFF \
      -DGGML_CUDA=ON \
      -DGGML_CPU=ON \
      -DGGML_OPENMP=OFF \
      -DWHISPER_BUILD_EXAMPLES=OFF \
      -DWHISPER_BUILD_TESTS=OFF \
      -DGGML_BUILD_TESTS=OFF \
    && cmake --build ${WHISPER_BUILD_DIR} --target whisper --config Release -j

# CGO include/link paths + CUDA link flags for Linux
ENV CPATH=${WHISPER_CPP_DIR}:${WHISPER_CPP_DIR}/include:${WHISPER_CPP_DIR}/ggml/include \
    CGO_CPPFLAGS=-I${WHISPER_CPP_DIR} -I${WHISPER_CPP_DIR}/include -I${WHISPER_CPP_DIR}/ggml/include \
    CGO_CFLAGS=-I${WHISPER_CPP_DIR} -I${WHISPER_CPP_DIR}/include -I${WHISPER_CPP_DIR}/ggml/include \
    CGO_LDFLAGS="-L${WHISPER_BUILD_DIR}/src -L${WHISPER_BUILD_DIR}/ggml/src -L${WHISPER_BUILD_DIR}/ggml/src/ggml-cuda \
                 -lwhisper -lggml -lggml-cuda -lggml-cpu -lggml-base \
                 -lcublas -lcublasLt -lcudart -lcuda -lstdc++ -lm -ldl -lpthread \
                 -Wl,-rpath,/usr/local/cuda/lib64"

# Build agentd for linux/amd64 with CGO enabled
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=1 GOOS=linux GOARCH=amd64 mkdir -p dist && \
    CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -v -o dist/agentd ./cmd/agentd


# Runtime image with CUDA runtime
FROM nvidia/cuda:12.4.1-runtime-ubuntu22.04 AS runtime
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates dumb-init \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /src/app/dist/agentd /app/agentd
COPY --from=builder /src/app/example.env /app/example.env
COPY --from=builder /src/app/config.yaml.example /app/config.yaml.example
COPY --from=builder /src/app/docs /app/docs

# NVIDIA runtime env
ENV NVIDIA_VISIBLE_DEVICES=all \
    NVIDIA_DRIVER_CAPABILITIES=compute,utility

USER 65532:65532
ENTRYPOINT ["dumb-init", "--", "/app/agentd"]
