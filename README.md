<div align="center">

# Manifold

</div>

![Manifold](docs/completions.jpg)

Manifold is a powerful platform designed for workflow automation using AI models. It supports text generation, image generation, and retrieval-augmented generation, integrating seamlessly with popular AI endpoints including OpenAI, llama.cpp, Apple's MLX LM, Google Gemini, Anthropic Claude, ComfyUI, and MFlux. Additionally, Manifold provides robust semantic search capabilities using PGVector combined with the SEFII (Semantic Embedding Forest with Inverted Index) engine.

> **Note:** Manifold is under active development, and breaking changes are expected. It is **NOT** production-ready. Contributions are highly encouraged!

---

## Web Search and Retrieval

![Manifold](docs/web.jpg)

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

Create or update your configuration based on the provided `config.yaml.example` in the repository:

```yaml
host: localhost
port: 8080
data_path: ./data
jaeger_host: localhost:6831  # Optional Jaeger tracing

# API Keys (optional integrations)
anthropic_key: "..."
openai_api_key: "..."
google_gemini_key: "..."
hf_token: "..."

# Database Configuration
database:
  connection_string: "postgres://myuser:changeme@localhost:5432/manifold"

# Completion and Embedding Services
completions:
  default_host: "http://localhost:32186/v1/chat/completions"  # Default: local llama-server
  api_key: ""

embeddings:
  host: "http://localhost:32184/v1/embeddings"  # Default: local llama-server
  api_key: ""
  dimensions: 768
  embed_prefix: "search_document: "
  search_prefix: "search_query: "
reranker:
  host: "http://localhost:32185/v1/rerank"  # Default: local llama-server
```

**Crucial Points:**

- Update database credentials (`myuser`, `changeme`) according to your PGVector setup.
- Adjust `default_host` and `embeddings.host` based on your chosen model server.

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

---

## Troubleshooting Common Issues

- **Port Conflict:** If port 8080 is occupied, either terminate conflicting processes or choose a new port in `config.yaml`.
- **PGVector Connectivity:** Confirm your `database.connection_string` matches PGVector container credentials.
- **Missing Config File:** Ensure `config.yaml` exists in the correct directory. Manifold will not launch without it.

---

## Run in Development Mode

Ensure `config.yaml` is present at the root of the project by using the provided `config.yaml.example` template and configuring your values.

Run the Go backend:
```
$ go mod tidy
$ go run .
```

Run the frontend:
```
$ cd frontend
$ nvm use 20
$ npm install
$ npm run dev
```

## Release Process

Manifold uses GitHub Actions to automatically build and publish releases. To create a new release:

1. Update version references in the codebase as needed
2. Create and push a new tag with the version number (e.g., `v0.1.0`)
3. GitHub Actions will automatically build binaries for all supported platforms and publish them as a GitHub release

## Contributing

Manifold welcomes contributions! Check the open issues for tasks and feel free to submit pull requests.
