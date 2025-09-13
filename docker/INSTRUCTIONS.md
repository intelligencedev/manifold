# Deploy Postgres with PGVector, PostGIS and PGRouting

## Build Image
```
docker build -f docker/pgsql.Dockerfile -t localhost/intelligence-dev-postgres .
```

### Create a volume for persistent data
```
docker volume create intelligence-dev-postgres-data
```

### Run the container with persistent storage
```
docker run -d \
  --name intelligence-dev-postgres \
  -e POSTGRES_DB=intelligence_dev \
  -e POSTGRES_USER=intelligence_dev \
  -e POSTGRES_PASSWORD=intelligence_dev \
  -p 5432:5432 \
  -v intelligence-dev-postgres-data:/var/lib/postgresql/data \
  localhost/intelligence-dev-postgres:latest
```

### Configure in config.yaml
```
databases:
  defaultDSN: "postgres://intelligence_dev:intelligence_dev@localhost:5432/intelligence_dev?sslmode=disable"
```