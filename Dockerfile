FROM golang:1.24-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git ca-certificates build-base
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Build agentd (primary server binary)
RUN go build -o agentd ./cmd/agentd

FROM alpine:latest
WORKDIR /app
RUN apk add --no-cache ca-certificates dumb-init
COPY --from=builder /app/agentd ./
COPY .env* ./
USER nobody
ENTRYPOINT ["dumb-init", "--", "./agentd"]
