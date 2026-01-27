# Build agentd for Linux (CPU-only, usable on macOS hosts via Docker Desktop)

FROM ubuntu:22.04 AS builder

ARG GO_VERSION=1.24.5
ENV DEBIAN_FRONTEND=noninteractive

# Base toolchain + Node 20 + pnpm
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential git make pkg-config curl ca-certificates \
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

# Build frontend assets and agentd binary using make target
# Use npm instead of pnpm to avoid platform-specific lockfile issues
RUN rm -rf node_modules pnpm-lock.yaml web/agentd-ui/node_modules \
  && cd web/agentd-ui \
  && npm install \
  && npm run build \
  && mkdir -p /src/app/internal/webui/dist \
  && rm -rf /src/app/internal/webui/dist/* \
  && cp -R /src/app/web/agentd-ui/dist/. /src/app/internal/webui/dist/

# Build agentd for linux/amd64
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    mkdir -p dist && go build -v -o dist/agentd ./cmd/agentd


# Runtime image
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
