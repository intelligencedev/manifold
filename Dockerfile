# Stage 1: Build the frontend
FROM node:20 AS frontend-builder

# Set the working directory for the frontend build
WORKDIR /manifold/frontend

# Copy the entire repository into the image
# (Make sure your .dockerignore does not exclude backend files)
COPY . /manifold

# Install frontend dependencies and build the frontend
RUN npm install
RUN npm run build

# Stage 2: Build the Go backend
FROM golang:1.23.5 AS backend-builder

WORKDIR /manifold

# Copy dependency files and download Go modules
COPY go.mod go.sum ./
RUN go mod download

# Copy the entire repository from the build context
COPY . .

# Copy in the built frontend assets from stage 1
COPY --from=frontend-builder /manifold/frontend/dist ./frontend/dist

# Disable CGO to produce a fully static binary
ENV CGO_ENABLED=0

# Build the Go binary by compiling the entire package
RUN go build -ldflags="-s -w" -trimpath -o ./dist/manifold .

# Stage 3: Create the runtime image
FROM debian:bullseye-slim

ENV JAEGER_ENDPOINT=http://0.0.0.0:16686
ENV DEBIAN_FRONTEND=noninteractive

# Install necessary packages
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        bash \
        ca-certificates \
        curl \
        wget && \
    # Install yq for YAML processing
    # yq is used to process the config file
    wget -qO /usr/local/bin/yq https://github.com/mikefarah/yq/releases/latest/download/yq_linux_amd64 && \
    chmod +x /usr/local/bin/yq && \
    # Cleanup
    apt-get autoremove -y && \
    apt-get clean

WORKDIR /app

# Copy the built binary from stage 2
COPY --from=backend-builder /manifold/dist/manifold /app/

# Copy the tokenized config file and processor script
COPY config.yaml.example /app/
COPY process_config.sh /app/
RUN chmod +x /app/process_config.sh

EXPOSE 8080

# Process config and start the application
CMD ["/bin/bash", "-c", "/app/process_config.sh && /app/manifold"]
