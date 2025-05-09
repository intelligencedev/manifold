<div align="center">

# Manifold

</div>

![Manifold](docs/completions.jpg)

Manifold is a powerful platform designed for workflow automation using AI models. It supports text generation, image generation, and retrieval-augmented generation, integrating seamlessly with popular AI endpoints including OpenAI, llama.cpp, Apple's MLX LM, Google Gemini, Anthropic Claude, ComfyUI, and MFlux. Additionally, Manifold provides robust semantic search capabilities using PGVector combined with the SEFII (Semantic Embedding Forest with Inverted Index) engine.

> **Note:** Manifold is under active development, and breaking changes are expected. It is **NOT** production-ready. Contributions are highly encouraged!

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

```yaml
# Server Configuration
host: 'localhost'
port: 8080
data_path: './data'

# Runtime Configuration
single_node_instance: true  # Auto-manages llama-server instances for embeddings, reranking, and completions

# Database Configuration
database:
  # PostgreSQL connection string with PGVector extension
  connection_string: "postgres://myuser:changeme@localhost:5432/manifold?sslmode=disable"

# API Tokens (for optional integrations)
hf_token: ""            # HuggingFace API token for accessing gated models
google_gemini_key: ""   # Google Gemini API token
anthropic_key: ""       # Anthropic API token (Claude models)

# LLM Services Configuration
completions:
  default_host: "http://127.0.0.1:32186/v1/chat/completions"  # Default: local llama-server
  completions_model: 'gpt-4o'  # Ignored if using local endpoint
  api_key: ""  # Required for OpenAI API
  agent:
    max_steps: 100  # Maximum steps for the ReAct framework agent
    memory: false   # Legacy memory setting (will be deprecated)

embeddings:
  host: "http://127.0.0.1:32184/v1/embeddings"  # Default: local llama-server
  api_key: ""
  dimensions: 768
  embed_prefix: "search_document: "
  search_prefix: "search_query: "

reranker:
  host: "http://127.0.0.1:32185/v1/rerank"  # Default: local llama-server

agentic_memory:
  enabled: false  # When true, enables long-term memory across agent sessions
                  # Requires PostgreSQL with pgvector extension
```

#### Environment Variable Support

All configuration settings can be overridden using environment variables, following this convention:

1. Prefix all variables with `MANIFOLD__`
2. Use UPPERCASE for all letters
3. Use DOUBLE underscore (`__`) to separate YAML hierarchy levels
4. Use SINGLE underscore (`_`) for keys containing underscores

**Examples:**
```
MANIFOLD__HOST=api.example.com                    → host: 'api.example.com'
MANIFOLD__PORT=9000                               → port: 9000
MANIFOLD__DATABASE__CONNECTION_STRING=...         → database.connection_string: '...'
MANIFOLD__COMPLETIONS__DEFAULT_HOST=http://...    → completions.default_host: 'http://...'
MANIFOLD__SINGLE_NODE_INSTANCE=false              → single_node_instance: false
```

Different value types are automatically handled:
- Numbers: `MANIFOLD__PORT=8080` → `port: 8080`
- Booleans: `MANIFOLD__SINGLE_NODE_INSTANCE=false` → `single_node_instance: false`
- Strings: `MANIFOLD__HOST=localhost` → `host: 'localhost'`
- Null: `MANIFOLD__HF_TOKEN=null` → `hf_token: null`
- JSON arrays: `MANIFOLD__MCPSERVERS__GITHUB__ARGS='["run","--rm"]'` → `mcpservers.github.args: ["run","--rm"]`
- JSON objects: `MANIFOLD__SOME__CONFIG='{"key":"value"}'` → `some.config: {"key":"value"}`

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
