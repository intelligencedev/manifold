# Manual Docker Database Instructions

These instructions are only needed if you are not using the repository root `docker compose` flow.

For most users, the recommended path is still:

```bash
docker compose up -d pg-manifold manifold
```

## Build The Postgres Image Manually

From the repository root:

```bash
docker build -f deploy/docker/postgres.Dockerfile -t localhost/pg-manifold .
docker volume create pg-manifold-data
```

## Run PostgreSQL Manually

```bash
docker run -d \
  --name pg-manifold \
  -e POSTGRES_DB=manifold \
  -e POSTGRES_USER=manifold \
  -e POSTGRES_PASSWORD=manifold \
  -p 5433:5432 \
  -v pg-manifold-data:/var/lib/postgresql/data \
  localhost/pg-manifold:latest
```

Host access:

- PostgreSQL will be reachable at `localhost:5433`

Container-to-container access:

- Other containers should still use port `5432` on the Postgres container hostname

## Configure Manifold To Use The Manual Database

If Manifold is running outside Docker on the host:

```yaml
databases:
  defaultDSN: "postgres://manifold:manifold@localhost:5433/manifold?sslmode=disable"
```

If Manifold is running in Docker Compose beside `pg-manifold`, keep using:

```yaml
databases:
  defaultDSN: "postgres://manifold:manifold@pg-manifold:5432/manifold?sslmode=disable"
```

## Optional: Qdrant As A Vector Backend

You can use [Qdrant](https://qdrant.tech/) instead of Postgres vector storage.

```bash
docker run -d --name qdrant -p 6334:6333 qdrant/qdrant
```

Example configuration:

```yaml
databases:
  vector:
    backend: qdrant
    dsn: "http://localhost:6334"
    index: "embeddings"
    dimensions: 1536
    metric: cosine
```
