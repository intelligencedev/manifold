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

Please note the image generation feature only works on M series Macs only. We will implement Windows and Linux support soon.

### MFlux - M Series Mac Only

Mflux is used as the image generation backend on M Series Macs. Please refer to the [MFlux installation instructions](https://github.com/filipstrand/mflux)

## Developer Notes

There are various hard coded configurations that need to be broken out into a configuration file the app loads.
For now, developers can change those endpoints and ports via the appropriate component code, or match the hard configuration.

Backend is `localhost:8080` and frontend is `localhost:3000`

We will implement a solution to change the configuration via a `.env` in a future commit.

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

### External Dependencies

Jaeger is not required, but we recommend [deploying the container](https://www.jaegertracing.io/docs/1.6/getting-started/#all-in-one-docker-image).

[pgvector](https://github.com/pgvector/pgvector) - SQL and vector store

Requires Node 20. We recommend using [NVM](https://github.com/nvm-sh/nvm) to manage Node environments.

Backend - localhost:8080:

The backend has Open Telemetry support and requires the JAEGER_ENDPOINT (Jaeger) endpoint be set. This does not have to exist so a fake endpoint can be set.
The application will just throw an error when attempting to send telemetry to the endpoint but will still function.

```
$ export JAEGER_ENDPOINT=<my otel endpoint>
$ go mod tidy
$ go run .
```

Frontend - localhost:3000:
```
$ cd frontend
$ nvm use 20
$ npm install
$ npm run dev
```
