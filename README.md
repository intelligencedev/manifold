<div align="center">

# Manifold

</div>

![Manifold](docs/completions.jpg)

Manifold is a powerful platform designed for workflow automation using AI models. It supports text generation, image generation, and retrieval-augmented generation, integrating seamlessly with popular AI endpoints including OpenAI, llama.cpp, Apple's MLX LM, Google Gemini, Anthropic Claude, ComfyUI, and MFlux. Additionally, Manifold provides robust semantic search capabilities using PGVector combined with the SEFII (Semantic Embedding Forest with Inverted Index) engine.

> **Note:** Manifold is under active development, and breaking changes are expected. It is **NOT** production-ready. Contributions are highly encouraged!

---

## Vision Support - Local Models + OpenAI Compatible

![Manifold](docs/vision.jpg)

---

## Web Search and Retrieval

![Manifold](docs/web.jpg)

And more!

## Prerequisites

Ensure the following software is installed before proceeding:

- **Chrome Browser:** Required for web tools. Used as a headless browser and managed by Manifold. WebGPU support should be enabled for text-to-speech to work.
- **Python:** Version 3.10 or newer ([Download](https://www.python.org/downloads/)).
- **Docker:** Recommended for easy setup of PGVector ([Download](https://www.docker.com/get-started)).

For development, you'll also need:
- **Go:** Version 1.21 or newer ([Download](https://golang.org/dl/)).
- **Node.js:** Version 20 managed via `nvm` ([Installation Guide](https://github.com/nvm-sh/nvm)).

---

## Quick Start with Pre-built Binaries

The easiest way to get started with Manifold is to download a pre-built binary from the [releases page](https://github.com/intelligencedev/manifold/releases).

1. Download the appropriate binary for your platform:
   - macOS: `manifold-darwin-universal.zip` (Universal binary for both Intel and Apple Silicon)
   - Linux: `manifold-linux-amd64.zip` or `manifold-linux-arm64.zip`
   
2. Extract the zip file and navigate to the extracted directory.

3. Create a `config.yaml` file in the same directory as the binary (a template `config.yaml.example` is included).

4. Run the binary:
   ```bash
   # On macOS/Linux
   $ chmod +x manifold-*
   $ ./manifold-*
   ```

---

## Installation from Source

### 1. Clone the Repository

```bash
$ git clone https://github.com/intelligencedev/manifold.git
$ cd manifold
```

### 2. Initialize Submodules

After cloning the repository, initialize and update the git submodules:

```bash
$ git submodule update --init --recursive
```

This will fetch the required dependencies:
- llama.cpp for local model inference
- pgvector for vector similarity search in PostgreSQL

#### PGVector Setup

Manifold will automatically manage the lifecycle of the PGVector container using Docker. Ensure Docker is installed and running on your system.

---

### 3. Install an Image Generation Backend (Choose One)

#### Option A: ComfyUI (Cross-platform)

- Follow the [official ComfyUI installation guide](https://github.com/comfyanonymous/ComfyUI#manual-install-windows-linux).
- No extra configuration needed; Manifold connects via proxy.

#### Option B: MFlux (M-series Macs Only)

- Follow the [MFlux installation guide](https://github.com/filipstrand/mflux).

---

### 4. Configuration

Use the provided `config.yaml.example` template to create a new `config.yaml` file. This file must be placed in the same path as the main.go file if running in development mode, or in the same path as the manifold binary if you build the project.

Ensure to update the values to match your environment.

### 5. Build and Run Manifold

For development it is not necessary to build the application. See development notes at the bottom of this guide.

Execute the following commands:

```bash
$ cd frontend
$ nvm use 20
$ npm install
$ npm run build
$ cd ..
$ go build -ldflags="-s -w" -trimpath -o ./dist/manifold .
$ cd dist

# 1. Place config.yaml in the same path as the binary
# 2. Run the binary
$ ./manifold
```

This sequence will:

- Switch Node.js to version 20.
- Build frontend assets.
- Compile the Go backend, generating the executable.
- Launch Manifold from the `dist` directory.

Upon first execution, Manifold creates necessary directories and files (e.g., `data`).

Note that Manifold builds the frontend and embeds it in its binary. When building the application, the frontend is not a separate web server.

- On first boot, the application will take longer as it downloads the required models for completions, embeddings, and reranker services.
- The application defaults to a single node instance configuration, managing the lifecycle of services using the llama-server backend and bootstrapping PGVector.
- Services can be configured to run on remote hosts to alleviate load on a single host, but users must manage the lifecycle of remote services manually.

---

### 6. Configuration (`config.yaml`)

Create or update your configuration based on the provided `config.yaml.example` in the repository. Manifold uses a flexible configuration system that supports both YAML files and environment variables.

Ensure you rename the file or create a new `config.yaml` with your unique configuration, and the file is present in the same path as the Manifold binary.

**Crucial Points:**

- Update database credentials (`myuser`, `changeme`) according to your PGVector setup.
- When `single_node_instance` is enabled, Manifold auto-manages the lifecycle of llama-server instances.
- When using external API services (OpenAI, Claude, etc.), provide the corresponding API keys.

---

## Accessing Manifold

Launch your browser and navigate to:

```
http://localhost:8080
```

> Replace the host configuration if you customized it in `config.yaml`.

### Default Authentication Credentials

When you first access Manifold, use these default credentials to log in:

```
Username: admin
Password: M@nif0ld@dminStr0ngP@ssw0rd
```

> **⚠️ IMPORTANT SECURITY WARNING:** These are publicly known default credentials. Immediately after logging in, change your password by clicking on your account name in the top right corner and selecting "Change Password".

---

## Supported Endpoints

Manifold is compatible with OpenAI-compatible endpoints:

- [llama.cpp Server](https://github.com/ggerganov/llama.cpp/tree/master/examples/server)
- [Apple MLX LM Server](https://github.com/ml-explore/mlx-examples/blob/main/llms/mlx_lm/SERVER.md)

Any inference engine that serves the standard completions endpoints will work such as vLLM, etc.
---

## Docker Images

### Building the Manifold MCP Docker Image

Manifold provides a Docker image for the Model Context Protocol (MCP) server component, allowing you to run the MCP server in containerized environments.

To build the Docker image:

```bash
# Navigate to the manifold root directory
$ cd manifold

# Build the Docker image with the intelligencedev/manifold:latest tag
$ docker build -t intelligencedev/manifold-mcp:latest -f manifold-mcp.Dockerfile .
```

Once built, you can configure the MCP server in your `config.yaml`:

```yaml
mcpServers:
  manifold:
    command: docker
    args:
      - run
      - --rm
      - -e
      - DATA_PATH=/app/projects
      - -i
      - --volume
      - /Users/$USER/.manifold/tmp:/app/projects
      - --volume
      - /Users/$USER/.ssh:/home/manifold/.ssh:ro
      - -e
      - GIT_SSH_COMMAND="ssh -o StrictHostKeyChecking=no -i /home/manifold/.ssh/id_rsa"
      - intelligencedev/manifold-mcp
```

---

## Troubleshooting Common Issues

- **Port Conflict:** If port 8080 is occupied, either terminate conflicting processes or choose a new port in `config.yaml`.
- **PGVector Connectivity:** Confirm your `database.connection_string` matches PGVector container credentials.
- **Missing Config File:** Ensure `config.yaml` exists in the correct directory. Manifold will not launch without it.

---

## Contributing

Manifold welcomes contributions! Check the open issues for tasks and feel free to submit pull requests.
