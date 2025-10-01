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