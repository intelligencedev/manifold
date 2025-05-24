FROM golang:1.24.3-alpine AS builder

# Install Git and other dependencies
RUN apk add --no-cache git gcc musl-dev

# Set working directory
WORKDIR /app

# Copy go.mod and go.sum first to leverage Docker cache
COPY go.mod go.sum ./
RUN go mod download

# Copy the entire codebase
COPY ./cmd/mcp-manifold/ ./

# Build the mcp-manifold binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -trimpath -o /app/mcp-manifold ./

# Use the latest OpenAI Codex image as the base for the final image
FROM ghcr.io/openai/codex-universal:latest

# Install dependencies for tools
RUN apt-get update && apt-get install -y git ca-certificates tzdata bash openssh-client \
    wget gnupg2 \
    # Install Chromium browser (works on both amd64 and arm64)
    chromium-browser \
    # Other required dependencies for headless browser usage
    fonts-ipafont-gothic fonts-wqy-zenhei fonts-thai-tlwg fonts-kacst fonts-freefont-ttf \
    --no-install-recommends && \
    rm -rf /var/lib/apt/lists/*

# Create a non-root user to run the application
RUN groupadd -r manifold && useradd -r -g manifold -m -s /bin/bash manifold

# Set the working directory
WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/mcp-manifold /app/mcp-manifold

# Create .ssh directory for the manifold user and set proper permissions
RUN mkdir -p /home/manifold/.ssh && \
    chown -R manifold:manifold /home/manifold/.ssh && \
    chmod 700 /home/manifold/.ssh

# Add GitHub's public key to known_hosts to ensure secure SSH connections
RUN ssh-keyscan -t rsa github.com >> /home/manifold/.ssh/known_hosts && \
    chown manifold:manifold /home/manifold/.ssh/known_hosts && \
    chmod 600 /home/manifold/.ssh/known_hosts

# Make sure manifold user owns the app directory
RUN chown -R manifold:manifold /app

# Create a directory for data that can be mounted as a volume
RUN mkdir -p /data
RUN chown -R manifold:manifold /data

# Configure Git for the manifold user
RUN git config --global user.email "manifold@example.com" && \
    git config --global user.name "Manifold User"

# Switch to non-root user
USER manifold

# Set environment variable for data path
ENV DATA_PATH=/data

# Configure Chromium for headless mode
ENV CHROME_BIN=/usr/bin/chromium-browser
ENV CHROME_PATH=/usr/lib/chromium/
# Following flags are needed for Chromium to run in a containerized environment
ENV CHROME_FLAGS="--headless --disable-gpu --no-sandbox --disable-dev-shm-usage"

# Command to run the binary
ENTRYPOINT ["/app/mcp-manifold"]