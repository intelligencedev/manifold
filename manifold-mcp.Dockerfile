FROM golang:1.24.3-alpine3.20 AS builder

# Install build-time deps only
RUN apk add --no-cache gcc musl-dev

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY ./cmd/mcp-manifold/ .
RUN CGO_ENABLED=0 GOOS=linux go build \
      -ldflags="-s -w" -trimpath \
      -o /out/mcp-manifold ./...

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata bash git openssh-client

COPY --from=builder /out/mcp-manifold /usr/local/bin/mcp-manifold

ENV DATA_PATH=/data

WORKDIR /app/projects

ENTRYPOINT ["mcp-manifold"]
