# Dockerfile.multilang
FROM ubuntu:20.04

# Set environment to non-interactive to avoid prompts during installation.
ENV DEBIAN_FRONTEND=noninteractive

# Install dependencies for Python, Go, and Node.js.
RUN apt-get update && \
    apt-get install -y \
      python3 python3-pip \
      golang-go \
      curl \
      ca-certificates \
      build-essential && \
    rm -rf /var/lib/apt/lists/*

# Install Node.js (using NodeSource setup for Node 16.x as an example).
RUN curl -fsSL https://deb.nodesource.com/setup_16.x | bash - && \
    apt-get update && \
    apt-get install -y nodejs && \
    rm -rf /var/lib/apt/lists/*

# Set the working directory where the sandbox code will be mounted.
WORKDIR /sandbox

# Default command - open a bash shell.
CMD ["/bin/bash"]
