# Deploy Postgres with PGVector, PostGIS and PGRouting

## Build Image
```
docker build -f postgres.Dockerfile -t localhost/pg-manifold .
```

### Create a volume for persistent data
```
docker volume create pg-manifold-data
```

### Run the container with persistent storage
```
docker run -d \
  --name pg-manifold \
  -e POSTGRES_DB=manifold \
  -e POSTGRES_USER=intelligence_dev \
  -e POSTGRES_PASSWORD=intelligence_dev \
  -p 5432:5432 \
  -v pg-manifold-data:/var/lib/postgresql/data \
  localhost/pg-manifold:latest
```

### Configure in config.yaml
```
databases:
  defaultDSN: "postgres://intelligence_dev:intelligence_dev@localhost:5432/manifold?sslmode=disable"
```

---

# Deploy With Qdrant Vector Search

You can use [Qdrant](https://qdrant.tech/) as an alternate vector search provider.

### Run the container

```
docker run -d -p 6334:6334 qdrant/qdrant
```

### Configure in config.yaml
```
databases:
  vector:
    backend: qdrant
    dsn: "http://localhost:6334"
    # With API key: "http://localhost:6334?api_key=your-secret-api-key"
    index: "embeddings"  # collection name
    dimensions: 1536     # vector dimensions
    metric: cosine       # cosine | dot | l2 | manhattan
```