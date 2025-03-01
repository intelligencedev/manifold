<div align="center">

# Manifold

</div>

![Manifold](docs/manifold_splash.jpg)

Manifold is a powerful platform designed for workflow automation using AI models. It supports text generation, image generation, and retrieval-augmented generation, integrating seamlessly with popular AI endpoints including OpenAI, llama.cpp, Apple's MLX LM, Google Gemini, Anthropic Claude, ComfyUI, and MFlux. Additionally, Manifold provides robust semantic search capabilities using PGVector combined with the SEFII (Semantic Embedding Forest with Inverted Index) engine.

> **Note:** Manifold is under active development, and breaking changes are expected. It is **NOT** production-ready. Contributions are highly encouraged!

---

## Prerequisites

Ensure the following software is installed before proceeding:

- **Go:** Version 1.21 or newer ([Download](https://golang.org/dl/)).
- **Python:** Version 3.10 or newer ([Download](https://www.python.org/downloads/)).
- **Node.js:** Version 20 managed via `nvm` ([Installation Guide](https://github.com/nvm-sh/nvm)).
- **Docker:** Recommended for easy setup of PGVector ([Download](https://www.docker.com/get-started)).

---

## Installation Steps

### 1. Clone the Repository

```bash
git clone <repository_url>  # Replace with actual repository URL
cd manifold
```

### 2. Set Up PGVector

PGVector provides efficient similarity search for retrieval workflows.

**Docker Installation (Recommended):**

```bash
docker run -d \
  --name pg-manifold \
  -p 5432:5432 \
  -v postgres-data:/var/lib/postgresql/data \
  -e POSTGRES_USER=myuser \
  -e POSTGRES_PASSWORD=changeme \
  -e POSTGRES_DB=manifold \
  pgvector/pgvector:latest
```

> **Important:** Update `myuser` and `changeme` with your preferred username and password.

**Verification:**

Verify your PGVector installation using `psql`:

```bash
psql -h localhost -p 5432 -U myuser -d manifold
```

You should see a prompt like `manifold=#`. Type `\q` to exit.

**Alternate Installation:**

For non-Docker methods, refer to the [PGVector documentation](https://github.com/pgvector/pgvector#installation).

---

### 3. Configure an Image Generation Backend (Choose One)

#### Option A: ComfyUI (Cross-platform)

- Follow the [official ComfyUI installation guide](https://github.com/comfyanonymous/ComfyUI#manual-install-windows-linux).
- No extra configuration needed; Manifold connects via proxy.

#### Option B: MFlux (M-series Macs Only)

- Follow the [MFlux installation guide](https://github.com/filipstrand/mflux).

---

### 4. Build and Run Manifold

Execute the following commands:

```bash
nvm use 20
npm run build
go build -ldflags="-s -w" -trimpath -o ./dist/manifold main.go
cd dist
./manifold
```

This sequence will:

- Switch Node.js to version 20.
- Build frontend assets.
- Compile the Go backend, generating the executable.
- Launch Manifold from the `dist` directory.

Upon first execution, Manifold creates necessary directories and files (e.g., `data`).

---

### 5. Configuration (`config.yaml`)

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

## Contributing

Manifold welcomes contributions! Check the open issues for tasks and feel free to submit pull requests.

---
