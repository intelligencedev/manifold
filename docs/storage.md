# Project Storage

This document describes Manifold's project storage system, which supports both local filesystem and S3-compatible object storage backends.

## Overview

Manifold stores user project files using a pluggable storage backend:

| Backend | Use Case | Configuration |
|---------|----------|---------------|
| **Filesystem** | Development, single-node deployments | Default, no additional setup |
| **S3** | Production, multi-node, durable storage | Requires S3/MinIO configuration |

Both backends support optional encryption at rest (see [Encryption](#encryption)).

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        agentd                               │
├─────────────────────────────────────────────────────────────┤
│  Projects Service (internal/projects/service.go)            │
│    ├── Metadata: PostgreSQL (projects table)                │
│    └── Files: ObjectStore interface                         │
├─────────────────────────────────────────────────────────────┤
│  ObjectStore (internal/objectstore/)                        │
│    ├── FilesystemStore (backend: filesystem)                │
│    └── S3Store (backend: s3)                                │
└─────────────────────────────────────────────────────────────┘
```

### Key Concepts

- **Project**: A collection of files belonging to a user, identified by UUID
- **Storage Backend**: Where project files are physically stored
- **ObjectStore**: Abstract interface for file operations (Get, Put, Delete, List)
- **Workspace**: The working directory used by agent tools during execution

## Configuration

### Environment Variables

#### Backend Selection

| Variable | Values | Default | Description |
|----------|--------|---------|-------------|
| `PROJECTS_BACKEND` | `filesystem`, `s3` | `filesystem` | Storage backend type |

#### Filesystem Backend

| Variable | Default | Description |
|----------|---------|-------------|
| `WORKDIR` | Required | Base directory for all project files |

Files are stored at: `${WORKDIR}/users/<userID>/projects/<projectID>/`

#### S3 Backend

| Variable | Default | Description |
|----------|---------|-------------|
| `PROJECTS_S3_ENDPOINT` | — | S3 API endpoint URL (required for MinIO) |
| `PROJECTS_S3_REGION` | `us-east-1` | AWS region |
| `PROJECTS_S3_BUCKET` | — | Bucket name (required) |
| `PROJECTS_S3_PREFIX` | `workspaces` | Key prefix for all objects |
| `PROJECTS_S3_ACCESS_KEY` | — | Access key ID |
| `PROJECTS_S3_SECRET_KEY` | — | Secret access key |
| `PROJECTS_S3_USE_PATH_STYLE` | `false` | Use path-style addressing (required for MinIO) |
| `PROJECTS_S3_TLS_INSECURE` | `false` | Skip TLS certificate verification |

##### S3 Server-Side Encryption

| Variable | Values | Description |
|----------|--------|-------------|
| `PROJECTS_S3_SSE_MODE` | `none`, `sse-s3`, `sse-kms` | Server-side encryption mode |
| `PROJECTS_S3_SSE_KMS_KEY_ID` | — | KMS key ID (when using `sse-kms`) |

### S3 Key Structure

Objects are stored with the following key format:

```
<prefix>/users/<userID>/projects/<projectID>/files/<relativePath>
```

Example:
```
workspaces/users/1/projects/abc123/files/src/main.go
workspaces/users/1/projects/abc123/files/README.md
```

## Local Development with MinIO

MinIO provides S3-compatible object storage for local development and testing.

### Quick Start

1. **Start MinIO with docker-compose:**

   ```bash
   docker-compose up -d minio
   ```

   This starts MinIO server on ports 9002 (S3 API) and 9003 (Web Console).

2. **Create the bucket:**

   The bucket must be created before using S3 storage. You can do this via:

   **Option A: Using the minio-init container:**
   ```bash
   docker-compose up minio-init
   ```

   **Option B: Using MinIO CLI (mc) inside the container:**
   ```bash
   # Set up the MinIO client alias
   docker exec manifold_minio mc alias set local http://localhost:9000 minioadmin minioadmin

   # Create the bucket
   docker exec manifold_minio mc mb local/manifold-workspaces
   ```

   **Option C: Using the MinIO Web Console:**
   1. Open http://localhost:9003
   2. Login with `minioadmin` / `minioadmin`
   3. Click "Create Bucket"
   4. Enter `manifold-workspaces` and click "Create"

3. **Configure environment:**

   ```bash
   # .env
   PROJECTS_BACKEND=s3
   PROJECTS_S3_ENDPOINT=http://localhost:9002
   PROJECTS_S3_BUCKET=manifold-workspaces
   PROJECTS_S3_ACCESS_KEY=minioadmin
   PROJECTS_S3_SECRET_KEY=minioadmin
   PROJECTS_S3_USE_PATH_STYLE=true
   ```

3. **Access MinIO Console:**

   Open http://localhost:9003 in your browser.
   
   - Username: `minioadmin`
   - Password: `minioadmin`

### Docker Compose Services

| Service | Port | Purpose |
|---------|------|---------|
| `minio` | 9002 | S3 API endpoint |
| `minio` | 9003 | Web console |
| `minio-init` | — | Creates bucket on startup |

### MinIO CLI (mc)

The MinIO client is available inside the container:

```bash
# List buckets
docker exec manifold_minio mc ls local/

# List objects in bucket
docker exec manifold_minio mc ls local/manifold-workspaces/

# Get bucket info
docker exec manifold_minio mc stat local/manifold-workspaces
```

## Production Deployment

### AWS S3

```bash
PROJECTS_BACKEND=s3
PROJECTS_S3_REGION=us-east-1
PROJECTS_S3_BUCKET=my-manifold-projects
PROJECTS_S3_PREFIX=workspaces
# Use IAM roles for credentials (recommended)
# Or explicitly:
# PROJECTS_S3_ACCESS_KEY=AKIA...
# PROJECTS_S3_SECRET_KEY=...
```

**Recommended IAM Policy:**

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:GetObject",
        "s3:PutObject",
        "s3:DeleteObject",
        "s3:ListBucket"
      ],
      "Resource": [
        "arn:aws:s3:::my-manifold-projects",
        "arn:aws:s3:::my-manifold-projects/*"
      ]
    }
  ]
}
```

### Google Cloud Storage (S3-compatible)

```bash
PROJECTS_BACKEND=s3
PROJECTS_S3_ENDPOINT=https://storage.googleapis.com
PROJECTS_S3_BUCKET=my-manifold-bucket
PROJECTS_S3_ACCESS_KEY=<HMAC_ACCESS_ID>
PROJECTS_S3_SECRET_KEY=<HMAC_SECRET>
```

### DigitalOcean Spaces

```bash
PROJECTS_BACKEND=s3
PROJECTS_S3_ENDPOINT=https://nyc3.digitaloceanspaces.com
PROJECTS_S3_REGION=nyc3
PROJECTS_S3_BUCKET=my-manifold-space
PROJECTS_S3_ACCESS_KEY=<SPACES_KEY>
PROJECTS_S3_SECRET_KEY=<SPACES_SECRET>
```

## Migration

### Filesystem to S3

Use the migration tool to transfer existing projects from filesystem to S3:

```bash
go run ./cmd/migrateprojects-s3/... \
  -workdir /path/to/workdir \
  -dsn "postgres://user:pass@host:5432/manifold" \
  -endpoint "http://localhost:9002" \
  -bucket "manifold-workspaces" \
  -access-key "minioadmin" \
  -secret-key "minioadmin" \
  -path-style \
  -verbose
```

**Flags:**

| Flag | Description |
|------|-------------|
| `-workdir` | Path to WORKDIR containing filesystem projects |
| `-dsn` | PostgreSQL connection string |
| `-endpoint` | S3 endpoint URL |
| `-bucket` | Target S3 bucket |
| `-access-key` | S3 access key |
| `-secret-key` | S3 secret key |
| `-region` | S3 region (default: us-east-1) |
| `-prefix` | S3 key prefix (default: workspaces) |
| `-path-style` | Use path-style addressing (required for MinIO) |
| `-dry-run` | Show what would be migrated without making changes |
| `-verbose` | Show detailed progress |
| `-skip-existing` | Skip files that already exist in S3 |

**What the migration does:**

1. Scans PostgreSQL for all projects
2. For each project with `storage_backend = 'filesystem'`:
   - Uploads all files to S3 with SHA256 verification
   - Updates `storage_backend` column to `'s3'`
3. Reports summary with file counts and bytes transferred

**Dry run first:**

```bash
go run ./cmd/migrateprojects-s3/... \
  -workdir /path/to/workdir \
  -dsn "..." \
  -dry-run -verbose
```

### Rollback

To revert a project to filesystem storage:

1. Download files from S3 to the filesystem path
2. Update the database: `UPDATE projects SET storage_backend = 'filesystem' WHERE id = '<uuid>'`

## Encryption

Manifold supports application-layer encryption for project files, independent of storage backend. **Both filesystem and S3 backends support encryption.**

### Overview

| Mode | Provider | Use Case |
|------|----------|----------|
| **None** | — | Development, already-encrypted storage |
| **File** | Local master key | Development, single-node |
| **Vault** | HashiCorp Vault Transit | Production, enterprise |
| **AWS KMS** | AWS Key Management Service | Production, AWS environments |

### S3 + Encryption Quick Start

To enable encrypted S3 storage with Vault:

1. **Start required services:**
   ```bash
   docker-compose up -d minio vault
   docker-compose up minio-init vault-init
   ```

2. **Configure environment (.env):**
   ```bash
   # S3 Backend
   PROJECTS_BACKEND=s3
   PROJECTS_S3_ENDPOINT=http://localhost:9002
   PROJECTS_S3_BUCKET=manifold-workspaces
   PROJECTS_S3_ACCESS_KEY=minioadmin
   PROJECTS_S3_SECRET_KEY=minioadmin
   PROJECTS_S3_USE_PATH_STYLE=true

   # Encryption with Vault
   PROJECTS_ENCRYPT=true
   PROJECTS_ENCRYPTION_PROVIDER=vault
   PROJECTS_ENCRYPTION_VAULT_ADDRESS=http://localhost:8200
   PROJECTS_ENCRYPTION_VAULT_KEY_NAME=manifold-kek
   VAULT_TOKEN=dev-only-token
   ```

3. **Or configure via config.yaml:**
   ```yaml
   projects:
     backend: s3
     encrypt: true
     s3:
       endpoint: "http://localhost:9002"
       region: "us-east-1"
       bucket: "manifold-workspaces"
       prefix: "workspaces"
       accessKey: "minioadmin"
       secretKey: "minioadmin"
     encryption:
       provider: vault
       vault:
         address: "http://localhost:8200"
         keyName: "manifold-kek"
         mountPath: "transit"
   ```

4. **Verify encryption is working:**
   ```bash
   # Upload a test file
   curl -X POST 'http://localhost:32180/api/projects/<project-id>/files?path=&name=test.txt' \
     -H 'Content-Type: text/plain' \
     --data-raw 'SECRET MESSAGE'

   # Check raw content in S3 (should be ciphertext with MGCM header)
   docker exec manifold_minio mc cat local/manifold-workspaces/workspaces/.../test.txt | xxd | head -5
   # Expected: 4d47 434d 01... (MGCM magic header + encrypted bytes)

   # Verify enc.json was created (wrapped DEK)
   docker exec manifold_minio mc cat local/manifold-workspaces/workspaces/.../project-id/.meta/enc.json
   # Expected: {"alg":"envelope","wrap_version":2,"wrapped_dek":"...","provider_type":"vault"}

   # Read back through API (should return plaintext)
   curl 'http://localhost:32180/api/projects/<project-id>/files?path=test.txt'
   # Expected: SECRET MESSAGE
   ```

### Encryption Architecture

```
┌─────────────────────────────────────────────┐
│              Project File                    │
└─────────────────────────────────────────────┘
                    │
                    ▼ Encrypt with DEK
┌─────────────────────────────────────────────┐
│         Data Encryption Key (DEK)            │
│         (per-project, random AES-256)        │
└─────────────────────────────────────────────┘
                    │
                    ▼ Wrap with KEK
┌─────────────────────────────────────────────┐
│         Key Encryption Key (KEK)             │
│         (from KeyProvider)                   │
└─────────────────────────────────────────────┘
```

- **DEK**: Random AES-256 key generated per project, stored wrapped in `.meta/enc.json`
- **KEK**: Master key from KeyProvider that wraps/unwraps DEKs

### Encrypted File Format

Encrypted files use the following binary format:

```
┌──────────┬─────────┬──────────┬─────────────────┐
│ Magic(4) │ Ver(1)  │ Nonce(12)│ Ciphertext(...) │
│  MGCM    │   0x01  │  Random  │   AES-GCM       │
└──────────┴─────────┴──────────┴─────────────────┘
```

- **Magic**: 4 bytes `MGCM` (0x4D 0x47 0x43 0x4D)
- **Version**: 1 byte, currently `0x01`
- **Nonce**: 12 bytes random IV for AES-GCM
- **Ciphertext**: AES-256-GCM encrypted content with authentication tag

Files without the `MGCM` header are treated as plaintext (for backward compatibility with files uploaded before encryption was enabled).

### Per-Project DEK Storage

Each encrypted project has a `.meta/enc.json` file containing the wrapped DEK:

```json
{
  "alg": "envelope",
  "wrap_version": 2,
  "wrapped_dek": "dmF1bHQ6djE6...",
  "provider_type": "vault"
}
```

| Field | Description |
|-------|-------------|
| `alg` | Always `"envelope"` for KeyProvider-based encryption |
| `wrap_version` | `2` for KeyProvider, `1` for legacy masterKey |
| `wrapped_dek` | Base64-encoded wrapped DEK (opaque to Manifold) |
| `provider_type` | `"vault"`, `"awskms"`, or `"file"` |

### Configuration

#### Enable Encryption

```bash
PROJECTS_ENCRYPT=true
```

#### File Provider (Default)

```bash
PROJECTS_ENCRYPT=true
PROJECTS_ENCRYPTION_PROVIDER=file
PROJECTS_ENCRYPTION_FILE_KEYSTORE_PATH=/path/to/keystore
```

The master key is stored at `<keystore_path>/master.key` and auto-generated if missing.

#### HashiCorp Vault

```bash
PROJECTS_ENCRYPT=true
PROJECTS_ENCRYPTION_PROVIDER=vault
PROJECTS_ENCRYPTION_VAULT_ADDRESS=https://vault.example.com:8200
PROJECTS_ENCRYPTION_VAULT_KEY_NAME=manifold-kek
VAULT_TOKEN=hvs.xxxxx
```

**Local Development with Vault (docker-compose):**

1. Start Vault dev server:
   ```bash
   docker-compose up -d vault
   ```

2. Initialize Transit engine and create encryption key:
   ```bash
   docker-compose up vault-init
   ```

3. Configure environment:
   ```bash
   # .env
   PROJECTS_ENCRYPT=true
   PROJECTS_ENCRYPTION_PROVIDER=vault
   PROJECTS_ENCRYPTION_VAULT_ADDRESS=http://localhost:8200
   VAULT_TOKEN=dev-only-token
   ```

4. Access Vault UI (optional):
   - Open http://localhost:8200
   - Token: `dev-only-token`

| Service | Port | Purpose |
|---------|------|---------|
| `vault` | 8200 | Vault API and UI |
| `vault-init` | — | One-shot Transit setup |

> **Note:** The dev server uses in-memory storage. Data is lost on restart. For persistent dev testing, use a production Vault setup.

**Production Vault Setup:**

```bash
# Enable Transit secrets engine
vault secrets enable transit

# Create encryption key
vault write -f transit/keys/manifold-kek

# Policy for Manifold
vault policy write manifold-encryption - <<EOF
path "transit/encrypt/manifold-kek" {
  capabilities = ["update"]
}
path "transit/decrypt/manifold-kek" {
  capabilities = ["update"]
}
EOF
```

| Variable | Default | Description |
|----------|---------|-------------|
| `PROJECTS_ENCRYPTION_VAULT_ADDRESS` | — | Vault server URL |
| `PROJECTS_ENCRYPTION_VAULT_TOKEN` | — | Auth token (or use `VAULT_TOKEN`) |
| `PROJECTS_ENCRYPTION_VAULT_KEY_NAME` | `manifold-kek` | Transit key name |
| `PROJECTS_ENCRYPTION_VAULT_MOUNT_PATH` | `transit` | Transit mount path |
| `PROJECTS_ENCRYPTION_VAULT_NAMESPACE` | — | Enterprise namespace |
| `PROJECTS_ENCRYPTION_VAULT_TLS_SKIP_VERIFY` | `false` | Skip TLS verification |

#### AWS KMS

```bash
PROJECTS_ENCRYPT=true
PROJECTS_ENCRYPTION_PROVIDER=awskms
PROJECTS_ENCRYPTION_AWSKMS_KEY_ID=arn:aws:kms:us-east-1:123456789:key/abc-123
PROJECTS_ENCRYPTION_AWSKMS_REGION=us-east-1
```

| Variable | Default | Description |
|----------|---------|-------------|
| `PROJECTS_ENCRYPTION_AWSKMS_KEY_ID` | — | KMS key ARN or alias |
| `PROJECTS_ENCRYPTION_AWSKMS_REGION` | — | AWS region |
| `PROJECTS_ENCRYPTION_AWSKMS_ENDPOINT` | — | Custom endpoint (LocalStack) |

### Key Rotation

Rotate a project's DEK:

```go
// API endpoint: POST /api/projects/{id}/rotate-key
```

This re-encrypts all project files with a new DEK.

### Migrated Files and Encryption

**Important:** Files migrated to S3 before encryption was enabled remain as plaintext. Encryption only applies to **new writes** after encryption is configured.

To verify which files are encrypted:

```bash
# Check if file has MGCM header (encrypted)
docker exec manifold_minio mc cat local/manifold-workspaces/.../file.txt | head -c 4 | xxd
# Encrypted: 4d47 434d (MGCM)
# Plaintext: Readable text

# Check if project has enc.json (has DEK)
docker exec manifold_minio mc ls local/manifold-workspaces/.../project-id/.meta/
# Should show enc.json if any encrypted files exist
```

To encrypt existing plaintext files, re-upload them through the API after encryption is enabled.

### Defense in Depth

For maximum security, combine:

1. **Application-layer encryption** (KeyProvider) — Protects against storage admin access
2. **S3 SSE** — Protects against physical storage compromise
3. **TLS** — Protects data in transit

## Database Schema

Projects metadata is stored in PostgreSQL:

```sql
CREATE TABLE projects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id BIGINT NOT NULL,
    name TEXT NOT NULL,
    storage_backend TEXT NOT NULL DEFAULT 'filesystem',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_projects_user_id ON projects(user_id);
CREATE UNIQUE INDEX idx_projects_user_name ON projects(user_id, name);
```

| Column | Description |
|--------|-------------|
| `id` | Unique project identifier (UUID) |
| `user_id` | Owner's user ID |
| `name` | Project display name |
| `storage_backend` | `filesystem` or `s3` |
| `created_at` | Creation timestamp |
| `updated_at` | Last modification timestamp |

## Troubleshooting

### Common Issues

#### "NoSuchBucket" Error

**Cause:** The S3 bucket doesn't exist.

**Solution:**
```bash
# MinIO
docker exec manifold_minio mc mb local/manifold-workspaces

# AWS CLI
aws s3 mb s3://manifold-workspaces
```

#### "SignatureDoesNotMatch" Error

**Cause:** Incorrect credentials or clock skew.

**Solution:**
- Verify `PROJECTS_S3_ACCESS_KEY` and `PROJECTS_S3_SECRET_KEY`
- Check system clock is synchronized

#### "connection refused" to MinIO

**Cause:** MinIO container not running or wrong port.

**Solution:**
```bash
docker-compose up -d minio
# Verify: should show healthy
docker ps | grep minio
```

#### Files Not Appearing After Migration

**Cause:** Project still marked as `filesystem` backend in database.

**Solution:**
```sql
SELECT id, name, storage_backend FROM projects WHERE user_id = <uid>;
-- If storage_backend is 'filesystem', migration didn't complete
```

#### Vault "permission denied" Error

**Cause:** Invalid token or missing Transit policy.

**Solution:**
```bash
# Verify token is valid
curl -H "X-Vault-Token: $VAULT_TOKEN" http://localhost:8200/v1/sys/health

# Check Transit engine is enabled
curl -H "X-Vault-Token: $VAULT_TOKEN" http://localhost:8200/v1/sys/mounts | jq '.["transit/"]'

# Verify key exists
curl -H "X-Vault-Token: $VAULT_TOKEN" http://localhost:8200/v1/transit/keys/manifold-kek
```

#### Vault "connection refused" Error

**Cause:** Vault container not running.

**Solution:**
```bash
docker-compose up -d vault
docker ps | grep vault
```

#### Cannot Decrypt Files After Switching KeyProvider

**Cause:** Files encrypted with one KeyProvider cannot be decrypted with another.

**Solution:**
- Projects encrypted with File provider require File provider to decrypt
- Projects encrypted with Vault require Vault to decrypt
- To switch providers, you must re-encrypt all project files (migration tool needed)

### Diagnostic Commands

```bash
# Check MinIO health
curl -I http://localhost:9002/minio/health/live

# List objects in bucket
docker exec manifold_minio mc ls --recursive local/manifold-workspaces/

# Check project storage backend
psql $DATABASE_URL -c "SELECT id, name, storage_backend FROM projects"

# Test S3 connectivity from agentd
curl -v http://localhost:9002/manifold-workspaces/

# Check Vault health
curl http://localhost:8200/v1/sys/health

# Verify Vault Transit key exists
curl -H "X-Vault-Token: dev-only-token" http://localhost:8200/v1/transit/keys/manifold-kek

# Test Vault encrypt/decrypt
echo -n "test" | base64 | xargs -I {} curl -s -H "X-Vault-Token: dev-only-token" \
  -d '{"plaintext":"{}"}' http://localhost:8200/v1/transit/encrypt/manifold-kek

# Verify file is encrypted in S3 (look for MGCM header)
docker exec manifold_minio mc cat local/manifold-workspaces/.../file.txt | xxd | head -3

# Check enc.json exists for project
docker exec manifold_minio mc cat local/manifold-workspaces/.../project-id/.meta/enc.json
```

### Logs

Enable debug logging for storage operations:

```bash
LOG_LEVEL=debug ./agentd
```

Look for log entries with:
- `objectstore` — S3 operations
- `projects` — Project service operations

## Performance Considerations

### Filesystem vs S3

| Aspect | Filesystem | S3 |
|--------|------------|-----|
| Latency | ~1ms | 10-100ms |
| Throughput | Disk-bound | Network-bound |
| Durability | Single node | 11 9's (AWS) |
| Scalability | Single node | Unlimited |

### Optimization Tips

1. **Use regional S3 endpoints** — Minimize latency
2. **Enable connection pooling** — Reuse HTTP connections
3. **Consider caching** — For frequently accessed files
4. **Use multipart uploads** — For files > 100MB (automatic)

## Security Best Practices

1. **Use IAM roles** instead of static credentials in production
2. **Enable bucket versioning** for accidental deletion protection
3. **Configure bucket policies** to restrict access
4. **Enable access logging** for audit trails
5. **Use VPC endpoints** for private S3 access
6. **Rotate credentials** regularly
7. **Enable MFA Delete** for critical buckets

## Related Documentation

- [Security Guide](security.md) — Overall security architecture
- [Authentication](auth.md) — User authentication and authorization
- [Observability](observability.md) — Monitoring and logging
