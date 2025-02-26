<div align="center">

# Manifold

</div>

![Manifold](docs/manifold_splash.jpg)

Manifold is a platform to enable workflow automation using AI models, including text generation, image generation, and retrieval-augmented generation. It integrates with OpenAI-compatible endpoints such as OpenAI API, llama.cpp and Apple's mlx_lm.server. Manifold also supports Google Gemini, Anthropic Claude, ComfyUI and MFlux image generation backends, and provides a powerful search interface using PGVector with SEFII engine (Semantic Embedding Forest with Inverted Index).

NOTE: This software is under heavy development and breaking changes are expected. This project is NOT production ready. Contributions are welcome.

## Prerequisites

Before you begin, ensure you have the following software installed:

*   **Go:** A recent version of Go (e.g., 1.21 or later) is recommended.
*   **Python** A recent version of Python (e.g., 3.10 or later) is recommended.
*   **Node.js and npm:** Node.js v20 is required. Use `nvm` (Node Version Manager) to manage Node.js versions.
*   **Docker:** Docker is used for running PGVector (recommended for ease of setup).

## Installation

### 1. Clone the Repository (Optional)

If you haven't already, clone the Manifold repository:

```bash
git clone <repository_url>  # Replace <repository_url> with the actual URL
cd manifold
```

### 2. Install PGVector (for Retrieval)

PGVector is used for efficient similarity search in retrieval workflows.  The easiest way to install it is using Docker:

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

**Important:**

*   Change `myuser` and `changeme` to your desired username and password.
*   The `-v postgres-data:/var/lib/postgresql/data` part creates a persistent volume, so your data won't be lost when the container stops.

**Verification:**

You can verify the PGVector installation by connecting to the database using a tool like `psql`:

```bash
psql -h localhost -p 5432 -U myuser -d manifold
# Enter the password you set (changeme) when prompted.
```

You should see the `manifold=#` prompt.  Type `\q` to exit.

**Alternative Installation:**

If you prefer not to use Docker, refer to the official PGVector installation instructions for other methods: [PGVector Installation](https://github.com/pgvector/pgvector?tab=readme-ov-file#installation).

### 3. Choose an Image Generation Backend (Choose One)

Manifold supports different backends for image generation.  Choose *one* of the following options:

#### a) ComfyUI (Windows, macOS, Linux)

ComfyUI is a versatile and powerful image generation tool.

*   **Installation:** Follow the official ComfyUI installation instructions: [ComfyUI Installation](https://github.com/comfyanonymous/ComfyUI?tab=readme-ov-file#manual-install-windows-linux).
*  Manifold uses a proxy to connect to ComfyUI. No additional configuration is required.

#### b) MFlux (M-Series Macs Only)

MFlux is specifically designed for image generation on Apple Silicon (M-series) Macs.

*   **Installation:** Follow the MFlux installation instructions: [MFlux Installation](https://github.com/filipstrand/mflux).

### 4. Build and Run Manifold

```bash
nvm use 20
npm run build
go build -ldflags="-s -w" -trimpath -o ./dist/manifold main.go
cd dist
./manifold
```

This will:

1.  Switch to Node.js version 20 using `nvm`.
2.  Build the frontend using `npm run build`.
3.  Build the Go backend, creating the `manifold` executable in the `dist` directory.
4.  Change the current directory to `dist`.
5.  Run the `manifold` application.

**Note:**  The first time you run Manifold, it will likely create a `data` directory and potentially other necessary files.

### 5. Configuration (config.yaml)

Manifold uses a `config.yaml` file for configuration. Reference the example `.config.yaml` at the root of this repository. Rename it to `config.yaml` or create a new file. Here are some key settings you'll likely want to configure:

```yaml
host: localhost
port: 8080
data_path: ./data
jaeger_host: localhost:6831  # Jaeger tracing (optional)

# Optional API Keys (only if you use these services)
anthropic_key: "..."
openai_api_key: "..."
google_gemini_key: "..."
hf_token: "..." #Hugging Face

# Anthropic API token
anthropic_key: "..."

database:
  connection_string: "postgres://myuser:changeme@localhost:5432/manifold"

completions:
  default_host: "http://localhost:8081"  # Example: llama.cpp server
  api_key: ""  # OpenAI Compatible endpoint API key for the completion service (if required by backend service)

embeddings:
  host: "http://localhost:8081"  # Example: llama.cpp server
  api_key: ""  # API key for the embedding service (if required)
  embedding_vectors: 1024  # Dimensionality of embeddings (adjust as needed)
```

*   **`database.connection_string`:**  This *must* match the settings you used when running the PGVector Docker container (or your alternative PGVector installation).  **Crucially, update `myuser` and `changeme` here if you changed them.**
*   **`completions.default_host` and `embeddings.host`:** These point to the address of your language model server (e.g., the `llama-server` or MLX LM server).  The example uses `http://localhost:8081`, which is the default for `llama.cpp`'s server.  Adjust if you're using a different server or port.
*   **API Keys:** The various `*_key` fields are for optional integrations.  Leave them blank if you're not using those services.

### Accessing Manifold

Once Manifold is running, you can access the web UI in your browser at:

```
http://localhost:8080
```

(Replace `8080` with the port you configured in `config.yaml` if you changed it.)

### Supported Endpoints
Manifold supports OpenAI compatible endpoints such as [llama-server](https://github.com/ggerganov/llama.cpp/tree/master/examples/server) and [Apple's MLX LM server](https://github.com/ml-explore/mlx-examples/blob/main/llms/mlx_lm/SERVER.md)

## Troubleshooting
*   **Port Conflicts:** If you encounter errors about port 8080 (or your configured port) being in use, make sure no other applications are using it. You can change the port in `config.yaml`.
*   **PGVector Connection Issues:** Double-check your `database.connection_string` in `config.yaml`. Ensure the username, password, host, and port are correct.
*  **Missing config.yaml** If the config file is not present, the application might not start.
