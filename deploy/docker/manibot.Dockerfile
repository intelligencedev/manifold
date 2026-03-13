FROM golang:1.25.0-alpine3.22 AS builder

WORKDIR /src/app

ARG TARGETOS=linux
ARG TARGETARCH

RUN apk add --no-cache ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags='-s -w' -o /out/manibot ./cmd/manibot

FROM golang:1.25.0-alpine3.22

WORKDIR /app
COPY --from=builder /out/manibot /app/manibot

USER 65532:65532
ENTRYPOINT ["/app/manibot"]