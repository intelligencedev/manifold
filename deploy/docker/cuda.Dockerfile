# syntax=docker/dockerfile:1.7

ARG NODE_IMAGE=node:20-bookworm-slim
ARG GO_IMAGE=golang:1.25.0-bookworm
ARG CUDA_RUNTIME_IMAGE=nvidia/cuda:12.4.1-runtime-ubuntu22.04


FROM ${NODE_IMAGE} AS ui-base
ENV PNPM_HOME=/pnpm \
    PATH=/pnpm:${PATH}
RUN corepack enable \
    && pnpm config set store-dir /pnpm/store
WORKDIR /src/web/agentd-ui


FROM ui-base AS ui-deps
COPY --link web/agentd-ui/package.json web/agentd-ui/pnpm-lock.yaml ./
RUN --mount=type=cache,id=manifold-pnpm-store,target=/pnpm/store \
    pnpm fetch --frozen-lockfile


FROM ui-deps AS ui-build
COPY --link web/agentd-ui/package.json web/agentd-ui/pnpm-lock.yaml ./
RUN --mount=type=cache,id=manifold-pnpm-store,target=/pnpm/store \
    pnpm install --frozen-lockfile --offline
COPY --link web/agentd-ui/ ./
RUN --mount=type=cache,id=manifold-pnpm-store,target=/pnpm/store \
    pnpm run build


FROM ${GO_IMAGE} AS go-base
WORKDIR /src


FROM go-base AS go-deps
COPY --link go.mod go.sum ./
RUN --mount=type=cache,id=manifold-go-mod,target=/go/pkg/mod \
    go mod download


FROM go-deps AS go-build
ARG TARGETOS=linux
ARG TARGETARCH=amd64
COPY --link cmd/ ./cmd/
COPY --link internal/ ./internal/
COPY --from=ui-build /src/web/agentd-ui/dist/ ./internal/webui/dist/
RUN --mount=type=cache,id=manifold-go-mod,target=/go/pkg/mod \
    --mount=type=cache,id=manifold-go-build,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -buildvcs=false -ldflags="-s -w" -o /out/agentd ./cmd/agentd


FROM ${CUDA_RUNTIME_IMAGE} AS runtime
ENV DEBIAN_FRONTEND=noninteractive \
    HOME=/home/manifold \
    PATH=/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt,sharing=locked \
    set -eux; \
    apt-get update; \
    apt-get install -y --no-install-recommends \
        bash \
        ca-certificates \
        curl \
        dumb-init \
        git \
        gnupg \
        openssh-client; \
    curl -fsSL https://deb.nodesource.com/setup_20.x | bash -; \
    apt-get update; \
    apt-get install -y --no-install-recommends nodejs; \
    install -m 0755 -d /etc/apt/keyrings; \
    curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg; \
    chmod a+r /etc/apt/keyrings/docker.gpg; \
    . /etc/os-release; \
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu ${VERSION_CODENAME} stable" > /etc/apt/sources.list.d/docker.list; \
    apt-get update; \
    apt-get install -y --no-install-recommends \
        docker-ce-cli \
        docker-compose-plugin; \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=go-base /usr/local/go /usr/local/go
COPY --from=go-build /out/agentd /app/agentd
COPY --link example.env config.yaml.example ./
COPY --link docs/ ./docs/
RUN ln -sf /usr/local/go/bin/go /usr/local/bin/go \
    && ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt \
    && install -d -o 65532 -g 65532 /home/manifold \
    && chown -R 65532:65532 /app

USER 65532:65532
ENTRYPOINT ["dumb-init", "--", "/app/agentd"]
