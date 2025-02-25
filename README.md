# Manifold

![Manifold](docs/manifold_splash.jpg)

Manifold supports OpenAI compatible endpoints such as [llama-server](https://github.com/ggerganov/llama.cpp/tree/master/examples/server) and [Apple's MLX LM server](https://github.com/ml-explore/mlx-examples/blob/main/llms/mlx_lm/SERVER.md)

# Installation Requirements

## PGVector

PGVector is a required for retrieval workflows. Refer to the [PGVector](https://github.com/pgvector/pgvector?tab=readme-ov-file#installation) installation instructions

The Docker build and run instructions are provided for convenience.
```
git clone --branch v0.8.0 https://github.com/pgvector/pgvector.git
cd pgvector
docker build --pull --build-arg PG_MAJOR=17 -t manifold/pgvector .
```

Run the PGVector service:
```
docker run -d \
  --name pg-manifold \
  -p 5432:5432 \
  -v postgres-data:/var/lib/postgresql/data \
  -e POSTGRES_USER=myuser \
  -e POSTGRES_PASSWORD=changeme \
  -e POSTGRES_DB=manifold \
  manifold/pgvector:latest
```

## Image Generation

### ComfyUI - Windows, MaxOS, Linux

ComfyUI can be used as an image generation backend. Please refer to the [ComfyUI installation instructions](https://github.com/comfyanonymous/ComfyUI?tab=readme-ov-file#manual-install-windows-linux).

### MFlux - M Series Mac Only

Mflux is used as the image generation backend on M Series Macs. Please refer to the [MFlux installation instructions](https://github.com/filipstrand/mflux).


## Build and Run

```
$ nvm use 20
$ npm run build
$ go build -ldflags="-s -w" -trimpath -o ./dist/manifold main.go
```

The `manifold` binary will contain the compiled frontend and backend.

Run the compiled application:

```
$ cd dist
$ ./manifold
```
