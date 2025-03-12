<div align="center">

# Manifold

</div>

![Manifold](docs/manifold_splash.jpg)

Manifold is a powerful platform designed for workflow automation using AI models. It supports text generation, image generation, and retrieval-augmented generation, integrating seamlessly with popular AI endpoints including OpenAI, llama.cpp, Apple's MLX LM, Google Gemini, Anthropic Claude, ComfyUI, and MFlux. Additionally, Manifold provides robust semantic search capabilities using PGVector combined with the SEFII (Semantic Embedding Forest with Inverted Index) engine.

> **Note:** Manifold is under active development, and breaking changes are expected. It is **NOT** production-ready. Contributions are highly encouraged!

---

## Prerequisites

Ensure the following software is installed before proceeding:

- **Chrome Browser:** Required for web tools. Used as a headless browser and managed by Manifold. WebPGU support should be enabled for text-to-speech to work.
- **Go:** Version 1.21 or newer ([Download](https://golang.org/dl/)).
- **Python:** Version 3.10 or newer ([Download](https://www.python.org/downloads/)).
- **Node.js:** Version 20 managed via `nvm` ([Installation Guide](https://github.com/nvm-sh/nvm)).
- **PGVector:** Required for retrieval augmented generation.
- **Docker:** Recommended for easy setup of PGVector ([Download](https://www.docker.com/get-started)).

---

## Installation Steps

### 1. Clone the Repository

```bash
$ git clone https://github.com/intelligencedev/manifold.git
$ cd manifold
```

### 3. Initialize Submodules

After cloning the repository, initialize and update the git submodules:

```bash
$ git submodule update --init --recursive
```

This will fetch the required dependencies:
- llama.cpp for local model inference
- pgvector for vector similarity search in PostgreSQL

#### PGVector Setup

After initializing the submodules, set up pgvector using Docker:

1. Navigate to the pgvector directory and build the Docker image:
```bash
$ cd external/pgvector
$ docker build -t pgvector .
```

2. Run the PostgreSQL container with pgvector:
```bash
$ docker run --name pgvector -e POSTGRES_USER=myuser -e POSTGRES_PASSWORD=changeme -e POSTGRES_DB=manifold -p 5432:5432 -d pgvector
```

3. The vector extension will be automatically enabled in your database. You can verify it by connecting to the database and running:
```sql
SELECT * FROM pg_extension WHERE extname = 'vector';
```

### 4. Install an Image Generation Backend (Choose One)

#### Option A: ComfyUI (Cross-platform)

- Follow the [official ComfyUI installation guide](https://github.com/comfyanonymous/ComfyUI#manual-install-windows-linux).
- No extra configuration needed; Manifold connects via proxy.

#### Option B: MFlux (M-series Macs Only)

- Follow the [MFlux installation guide](https://github.com/filipstrand/mflux).

---

### 5. Configuration

Use the provided `.config.yaml` template to create a new `config.yaml` file. This file must be placed in the same path as the main.go file if running in development mode, or in the same path as the manifold binary if you build the project.

Ensure to update the values to match your environment.

### 6. Build and Run Manifold

For development it is not necessary to build the application. See development notes at the bottom of this guide.

Execute the following commands:

```bash
$ cd frontend
$ nvm use 20
$ npm run build
$ cd ..
$ go build -ldflags="-s -w" -trimpath -o ./dist/manifold .
$ cd dist

# 1. Ensure PG Vector is running
# 2. Place config.yaml in the same path as the binary
# 3. Run the binary
$ ./manifold
```

This sequence will:

- Switch Node.js to version 20.
- Build frontend assets.
- Compile the Go backend, generating the executable.
- Launch Manifold from the `dist` directory.

Upon first execution, Manifold creates necessary directories and files (e.g., `data`).

Note that Manifold builds the frontend and embeds it in its binary. When building the application, the frontend is not a separate web server.

---

### 6. Configuration (`config.yaml`)

Create or update your configuration based on the provided `.config.yaml` example in the repository root:

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
  default_host: "http://localhost:8081"  # Example: llama.cpp server
  api_key: ""

embeddings:
  host: "http://localhost:8081"  # Example: llama.cpp server
  api_key: ""
  embedding_vectors: 1024
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

> Replace `8080` if you customized your port in `config.yaml`.

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

Ensure `config.yaml` is present at the root of the project by using the provided `.config.yaml` template and configuring your values.

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

## Contributing

Manifold welcomes contributions! Check the open issues for tasks and feel free to submit pull requests.
