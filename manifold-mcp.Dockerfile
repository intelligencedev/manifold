FROM golang:1.23.4-alpine AS builder

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

# Create a minimal runtime image
FROM alpine:latest

# Install dependencies for tools
RUN apk add --no-cache git ca-certificates tzdata bash openssh-client

# Create a non-root user to run the application
RUN addgroup -S manifold && adduser -S manifold -G manifold

# Set the working directory
WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/mcp-manifold /app/mcp-manifold

# Create .ssh directory for the manifold user and set proper permissions
RUN mkdir -p /home/manifold/.ssh && \
    chown -R manifold:manifold /home/manifold/.ssh && \
    chmod 700 /home/manifold/.ssh

# Add SSH config to avoid strict host key checking for GitHub
RUN echo "Host github.com\n\tStrictHostKeyChecking no\n\tIdentityFile /home/manifold/.ssh/id_rsa\n" >> /home/manifold/.ssh/config && \
    chown manifold:manifold /home/manifold/.ssh/config && \
    chmod 600 /home/manifold/.ssh/config

# Make sure manifold user owns the app directory
RUN chown -R manifold:manifold /app

# Create a directory for data that can be mounted as a volume
RUN mkdir -p /data
RUN chown -R manifold:manifold /data

# Switch to non-root user
USER manifold

# Configure Git for the manifold user
RUN git config --global user.email "manifold@example.com" && \
    git config --global user.name "Manifold User"

# Set environment variable for data path
ENV DATA_PATH=/data

# Command to run the binary
ENTRYPOINT ["/app/mcp-manifold"]