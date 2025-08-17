# Deploy Postgres with PGVector, PostGIS and PGRouting

## Build Image
```
docker build -f docker/pgsql.Dockerfile -t localhost/singularityio-postgres .
```

### Create a volume for persistent data
```
docker volume create singularityio-postgres-data
```

### Run the container with persistent storage
```
docker run -d \
  --name singularityio-postgres \
  -e POSTGRES_DB=singularityio \
  -e POSTGRES_USER=singularityio \
  -e POSTGRES_PASSWORD=singularityio \
  -p 5432:5432 \
  -v singularityio-postgres-data:/var/lib/postgresql/data \
  localhost/singularityio-postgres:latest
```

### Configure in config.yaml
```
databases:
  defaultDSN: "postgres://singularityio:singularityio@localhost:5432/singularityio?sslmode=disable"
```