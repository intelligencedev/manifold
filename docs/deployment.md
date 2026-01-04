# Manifold Deployment Guide

This guide covers deploying Manifold in different configurations, from simple single-node setups to enterprise multi-tenant deployments.

## Deployment Modes

Manifold supports two primary deployment modes:

| Mode | Use Case | Requirements |
|------|----------|--------------|
| **Simple** | Development, small teams, single-node | PostgreSQL only |
| **Enterprise** | Production, multi-tenant, high-availability | PostgreSQL + S3 + Redis + Kafka + Vault |

## Quick Start: Simple Mode

Simple mode is the default configuration with minimal infrastructure requirements.

### Prerequisites

- Docker and Docker Compose
- 4GB+ RAM
- OpenAI API key (or compatible LLM endpoint)

### Steps

1. **Clone and configure**:
   ```bash
   git clone https://github.com/intelligencedev/manifold.git
   cd manifold
   cp example.env .env
   cp config.yaml.example config.yaml
   ```

2. **Edit `.env`** with your API keys:
   ```env
   OPENAI_API_KEY=sk-...
   ```

3. **Start services**:
   ```bash
   docker-compose up -d pg-manifold manifold
   ```

4. **Access the UI**:
   Open http://localhost:32180

### What's Running

- **manifold**: Main application server
- **pg-manifold**: PostgreSQL database

Projects are stored as plaintext files in the local `WORKDIR` directory.

---

## Enterprise Mode

Enterprise mode enables all security and scalability features:

- **S3/MinIO**: Durable object storage for projects
- **Redis**: Distributed coordination, generation cache, invalidation
- **Kafka**: Event streaming for audit trail and cross-node sync
- **Vault**: Enterprise key management with Transit secrets engine
- **Encrypted Cache**: Ciphertext-only local disk with tmpfs plaintext

### Prerequisites

- Docker and Docker Compose
- 16GB+ RAM (8GB minimum)
- Persistent storage for databases and object storage
- OpenAI API key (or compatible LLM endpoint)

### Architecture Overview

```
┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│   agentd    │────▶│   Postgres   │     │   Vault     │
│  (N nodes)  │     └──────────────┘     │  (Transit)  │
└─────────────┘            │             └─────────────┘
       │                   │                    │
       ▼                   ▼                    ▼
┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│    Redis    │     │   S3/MinIO   │     │    Kafka    │
│(coord/cache)│     │  (durable)   │     │  (events)   │
└─────────────┘     └──────────────┘     └─────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────┐
│              Local Node Cache                       │
│  ┌─────────────────┐    ┌─────────────────────────┐ │
│  │ Encrypted Cache │    │  tmpfs Working Set      │ │
│  │  (ciphertext)   │───▶│  (plaintext, ephemeral) │ │
│  └─────────────────┘    └─────────────────────────┘ │
└─────────────────────────────────────────────────────┘
```

### Deployment Steps

1. **Clone and configure**:
   ```bash
   git clone https://github.com/intelligencedev/manifold.git
   cd manifold
   cp example.env .env
   ```

2. **Edit `.env`** with production secrets:
   ```env
   # API Keys
   OPENAI_API_KEY=sk-...
   
   # PostgreSQL
   POSTGRES_PASSWORD=<strong-password>
   
   # MinIO/S3
   MINIO_ROOT_USER=<access-key>
   MINIO_ROOT_PASSWORD=<secret-key>
   MINIO_BUCKET=manifold-workspaces
   
   # Vault
   VAULT_DEV_ROOT_TOKEN=<vault-token>
   VAULT_KEY_NAME=manifold-kek
   
   # Redis (optional password)
   REDIS_PASSWORD=<redis-password>
   
   # Kafka
   KAFKA_BROKERS=kafka:29092
   ```

3. **Start all services**:
   ```bash
   docker-compose -f docker-compose.enterprise.yml up -d
   ```

4. **Verify services**:
   ```bash
   # Check all containers are running
   docker-compose -f docker-compose.enterprise.yml ps
   
   # Check logs
   docker-compose -f docker-compose.enterprise.yml logs -f manifold
   ```

5. **Access the UI**:
   Open http://localhost:32180

### Service Endpoints

| Service | Port | Purpose |
|---------|------|---------|
| Manifold | 32180 | Main application |
| PostgreSQL | 5433 | Database |
| MinIO API | 9002 | S3-compatible storage |
| MinIO Console | 9003 | MinIO management UI |
| Redis | 6379 | Coordination/cache |
| Kafka | 9092 | Event streaming |
| Vault | 8200 | Key management |
| ClickHouse | 8123/9000 | Observability |
| OTEL Collector | 4317/4318 | Telemetry ingestion |

---

## Observability Stack

Enterprise mode includes a full observability stack with ClickHouse for storage and the OpenTelemetry Collector for ingestion.

### Components

| Component | Purpose | Port |
|-----------|---------|------|
| ClickHouse | Time-series storage for traces, logs, and metrics | 8123 (HTTP), 9000 (Native) |
| OTEL Collector | Telemetry ingestion and processing | 4317 (gRPC), 4318 (HTTP) |

### Architecture

```
┌─────────────┐     ┌──────────────────┐     ┌─────────────┐
│   agentd    │────▶│  OTEL Collector  │────▶│  ClickHouse │
│  (traces)   │     │   (processing)   │     │  (storage)  │
└─────────────┘     └──────────────────┘     └─────────────┘
```

### Data Retention

Default retention policies:
- **Traces**: 7 days
- **Logs**: 7 days  
- **Metrics**: 30 days

Modify TTL settings in `configs/clickhouse/init-otel.sql` to adjust retention.

### Querying Data

Access ClickHouse directly for custom queries:

```bash
# Connect to ClickHouse CLI
docker exec -it manifold_clickhouse clickhouse-client

# Example: Recent traces
SELECT 
    ServiceName, 
    SpanName, 
    Duration/1000000 AS duration_ms 
FROM otel.traces 
WHERE Timestamp > now() - INTERVAL 1 HOUR 
ORDER BY Timestamp DESC 
LIMIT 10;

# Example: Token usage by model
SELECT 
    Attributes['llm.model'] AS model,
    sum(Value) AS total_tokens
FROM otel.metrics_sum 
WHERE MetricName = 'llm.prompt_tokens'
GROUP BY model;
```

### Configuration

OTEL Collector config: `configs/otel/collector.yaml`
ClickHouse schema: `configs/clickhouse/init-otel.sql`

Environment variables for the application:
```env
OTEL_SERVICE_NAME=manifold
OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4318
CLICKHOUSE_DSN=tcp://clickhouse:9000?database=otel
```

---

## Configuration Reference

### Projects Configuration

```yaml
projects:
  # Storage backend
  backend: s3  # "filesystem" | "s3"
  
  # At-rest encryption
  encrypt: true
  encryption:
    provider: vault  # "file" | "vault" | "awskms"
    vault:
      address: "http://vault:8200"
      token: "${VAULT_TOKEN}"
      keyName: "manifold-kek"
      mountPath: "transit"
  
  # Workspace mode
  workspace:
    mode: ephemeral  # "legacy" | "ephemeral"
    cacheDir: "/app/cache"      # Encrypted cache (disk)
    tmpfsDir: "/app/tmpfs"      # Plaintext working set (RAM)
    ttlSeconds: 86400           # 24h workspace TTL
  
  # S3 storage
  s3:
    endpoint: "http://minio:9000"
    bucket: "manifold-workspaces"
    usePathStyle: true
  
  # Redis coordination
  redis:
    enabled: true
    addr: "redis:6379"
  
  # Kafka events
  events:
    enabled: true
    brokers: "kafka:29092"
    topic: "manifold.project.commits"
```

### Skills Configuration

```yaml
skills:
  # Redis cache TTL for rendered prompts
  redisCacheTTLSeconds: 3600  # 1 hour
  
  # Direct S3 skills loading (skip full workspace hydration)
  useS3Loader: true
```

### Environment Variables

All configuration values support environment variable expansion with `${VAR}` syntax.

| Variable | Description | Default |
|----------|-------------|---------|
| `WORKDIR` | Base directory for workspaces | `/app/manifold` |
| `PROJECTS_BACKEND` | Storage backend | `filesystem` |
| `PROJECTS_ENCRYPT` | Enable encryption | `false` |
| `PROJECTS_WORKSPACE_MODE` | Workspace isolation | `legacy` |
| `REDIS_ENABLED` | Enable Redis | `false` |
| `REDIS_ADDR` | Redis address | `localhost:6379` |
| `VAULT_ADDR` | Vault server URL | - |
| `VAULT_TOKEN` | Vault authentication token | - |
| `KAFKA_BROKERS` | Kafka broker addresses | - |

---

## Security Considerations

### Encryption Architecture

Enterprise mode uses envelope encryption:

1. **KEK (Key Encryption Key)**: Managed by Vault Transit engine
2. **DEK (Data Encryption Key)**: Per-project, wrapped with KEK
3. **Local Cache**: Always ciphertext on disk
4. **tmpfs**: Plaintext only in RAM, never touches disk

### Network Security

- Use TLS for all inter-service communication in production
- Configure firewall rules to restrict access to management ports
- Use Vault namespaces for multi-tenant isolation

### Access Control

- Enable authentication (`auth.enabled: true`)
- Configure OIDC/OAuth2 with your identity provider
- Use RBAC for fine-grained permissions

---

## Scaling

### Horizontal Scaling

Multiple `manifold` instances can run behind a load balancer:

```yaml
# docker-compose.scale.yml
services:
  manifold:
    deploy:
      replicas: 3
```

Redis provides:
- Distributed generation cache
- Pub/sub invalidation across nodes
- Distributed locks for commits

### Resource Sizing

| Component | Small (< 100 users) | Medium (100-1000) | Large (1000+) |
|-----------|---------------------|-------------------|---------------|
| manifold | 2 CPU, 4GB RAM | 4 CPU, 8GB RAM | 8 CPU, 16GB RAM |
| PostgreSQL | 2 CPU, 2GB RAM | 4 CPU, 4GB RAM | 8 CPU, 16GB RAM |
| Redis | 1 CPU, 512MB | 2 CPU, 1GB | 4 CPU, 4GB |
| MinIO | 2 CPU, 2GB RAM | 4 CPU, 4GB RAM | 8 CPU, 8GB RAM |
| tmpfs | 1GB | 2GB | 4GB+ |

---

## Monitoring

### Health Checks

```bash
# Application health
curl http://localhost:32180/health

# Redis
redis-cli ping

# Kafka
kafka-broker-api-versions --bootstrap-server localhost:9092
```

### Metrics

Manifold exports OpenTelemetry metrics to the OTEL Collector:

- `llm.prompt_tokens`: Prompt token usage
- `llm.completion_tokens`: Completion token usage
- `workspace.checkout.duration`: Workspace checkout latency
- `workspace.commit.duration`: Workspace commit latency

### Logging

```bash
# Application logs
docker-compose logs -f manifold

# All service logs
docker-compose logs -f
```

---

## Troubleshooting

### Common Issues

**Workspace checkout slow**
- Check Redis connectivity for generation cache
- Verify S3 endpoint latency
- Monitor tmpfs memory usage

**Encryption errors**
- Verify Vault is healthy: `vault status`
- Check Transit key exists: `vault list transit/keys`
- Ensure VAULT_TOKEN has correct permissions

**Kafka connection issues**
- Verify broker connectivity
- Check topic auto-creation is enabled
- Monitor consumer lag

### Debug Mode

Enable verbose logging:

```yaml
# config.yaml
logLevel: debug
logPayloads: true
```

---

## Backup and Recovery

### PostgreSQL

```bash
# Backup
docker exec postgres pg_dump -U manifold manifold > backup.sql

# Restore
docker exec -i postgres psql -U manifold manifold < backup.sql
```

### MinIO/S3

```bash
# Backup with mc
mc mirror manifold/manifold-workspaces ./backup/

# Restore
mc mirror ./backup/ manifold/manifold-workspaces
```

### Vault

For production, use Vault's built-in backup mechanisms:
- Integrated Storage snapshots
- Transit key export (if enabled)

---

## Migration

### Simple → Enterprise

1. Export projects from filesystem
2. Configure S3 backend
3. Upload projects to S3
4. Enable encryption (new files only)
5. Switch workspace mode to ephemeral

### Re-encryption

To rotate the KEK:

1. Create new Vault Transit key
2. Update `VAULT_KEY_NAME`
3. Run re-encryption job (tool TBD)

---

## Support

- Documentation: `docs/`
- Issues: GitHub Issues
- Architecture: `docs/workspace_architecture_plan.md`
