# syntax=docker/dockerfile:1

########################  Stage 1 : Go build + Go CLI tools  ########################
FROM golang:1.24.3-alpine3.20 AS go-build

# Build-time deps
RUN apk add --no-cache gcc musl-dev git

WORKDIR /src

# Restore modules
COPY go.mod go.sum ./
RUN go mod download

# Build your application binary
COPY ./cmd/mcp-manifold/ .
RUN CGO_ENABLED=0 GOOS=linux go build \
      -ldflags="-s -w" -trimpath \
      -o /out/mcp-manifold ./...

# Go-based linters / analyzers
RUN go install golang.org/x/tools/gopls@latest \
 && go install honnef.co/go/tools/cmd/staticcheck@latest \
 && go install github.com/securego/gosec/v2/cmd/gosec@latest \
 && go install github.com/mgechev/revive@latest

########################  Stage 2 : Node 20 + JS/TS CLI tools  ######################
FROM node:20-alpine AS node-tools

# Global JS / TS toolchain
RUN npm install -g \
      vite \
      eslint \
      prettier \
      jest \
      typescript \
      ts-node

########################  Stage 3 : Python 3.12 CLI tools  ##########################
FROM python:3.12-alpine AS python-tools
RUN pip install --no-cache-dir \
      flake8 \
      pylint \
      mypy \
      pytest

########################  Stage 4 : Final runnable image  ###########################
FROM alpine:3.20

# Core OS packages needed at runtime
RUN apk add --no-cache \
      ca-certificates \
      tzdata \
      bash \
      git \
      openssh-client \
      curl \
      python3 \
      py3-pip

# ---- mcp-manifold binary ----
COPY --from=go-build /out/mcp-manifold /usr/local/bin/mcp-manifold

# ---- Go runtime + Go CLI tools ----
COPY --from=go-build /usr/local/go /usr/local/go
COPY --from=go-build /go/bin/ /usr/local/bin/

# ---- Node 20 runtime + JS/TS CLI tools ----
COPY --from=node-tools /usr/local/bin/ /usr/local/bin/
COPY --from=node-tools /usr/local/lib/ /usr/local/lib/

# ---- Python CLI tools (site-packages + entrypoints) ----
COPY --from=python-tools /usr/local/lib/python3.12/site-packages/ \
                          /usr/local/lib/python3.12/site-packages/
COPY --from=python-tools /usr/local/bin/flake8   /usr/local/bin/
COPY --from=python-tools /usr/local/bin/pylint   /usr/local/bin/
COPY --from=python-tools /usr/local/bin/mypy     /usr/local/bin/
COPY --from=python-tools /usr/local/bin/pytest   /usr/local/bin/

# ---- PATH & ENV ----
ENV PATH="/usr/local/go/bin:/usr/local/bin:${PATH}"
ENV DATA_PATH=/data

WORKDIR /app/projects
ENTRYPOINT ["mcp-manifold"]
