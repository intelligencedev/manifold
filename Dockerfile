FROM golang:1.24-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o agent-tui ./cmd/agent-tui

FROM alpine:latest
WORKDIR /app

# Install runtime dependencies including Chromium for potential web automation
RUN apk add --no-cache \
    ca-certificates \
    chromium \
    nss \
    freetype \
    freetype-dev \
    harfbuzz \
    ttf-freefont \
    dumb-init \
    && addgroup -g 1000 appuser \
    && adduser -D -s /bin/sh -u 1000 -G appuser appuser

# Create chrome user and set up directories
RUN mkdir -p /home/appuser/Downloads /app \
    && chown -R appuser:appuser /home/appuser \
    && chown -R appuser:appuser /app

COPY --from=builder /app/agent-tui ./
COPY .env* ./

# Switch to non-root user for security
USER appuser

# Set Chrome/Chromium environment variables for headless operation (Not currently used in web tools)
ENV CHROME_BIN=/usr/bin/chromium-browser \
    CHROME_PATH=/usr/bin/chromium-browser \
    CHROMIUM_FLAGS="--disable-software-rasterizer --disable-background-timer-throttling --disable-backgrounding-occluded-windows --disable-renderer-backgrounding --disable-features=TranslateUI --disable-ipc-flooding-protection --no-sandbox --disable-setuid-sandbox --disable-dev-shm-usage --disable-extensions --no-first-run --no-default-browser-check --disable-default-apps --disable-popup-blocking --disable-translate --disable-background-networking --disable-sync --disable-web-security --disable-features=VizDisplayCompositor --headless --remote-debugging-port=0"

ENTRYPOINT ["dumb-init", "--", "./agent-tui"]
