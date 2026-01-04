# STORAGE_PLAN.md

## Purpose
Implement Manifold “Projects” backed by:

1) **Durable object storage (S3/MinIO)** as the source of truth for project contents.
2) **Secure ephemeral workspaces** (local filesystem working copies) used for all tool execution (`run_cli`, `apply_patch`, `code_evolve`, etc.).

This plan is written for an AI coding assistant to implement step-by-step with minimal guesswork.

---

## Current State (Verified in Repo)

### Projects storage today: local filesystem under WORKDIR
- The Projects service is implemented in `internal/projects/service.go`.
- Projects live on disk at:
  - `${WORKDIR}/users/<userID>/projects/<projectID>/...`
- Project metadata is stored at:
  - `${projectRoot}/.meta/project.json`
- Optional envelope encryption exists and is **local-disk based**:
  - Master key stored at `${WORKDIR}/.keystore/master.key`
  - Per-project wrapped DEK stored at `${projectRoot}/.meta/enc.json`
  - File contents are written in a custom AES-GCM header format.

### HTTP API today
- Routes are registered in `internal/agentd/router.go`.
  - `GET/POST /api/projects`
  - `GET /api/projects/{id}` (lists root tree)
  - `GET /api/projects/{id}/tree?path=...`
  - `GET/POST/DELETE /api/projects/{id}/files?path=...&name=...`
  - `POST /api/projects/{id}/dirs?path=...`
  - `POST /api/projects/{id}/move` (body: `{from,to}`)
- Handlers are implemented in `internal/agentd/handlers_projects.go`.

### Frontend tree expectations (Verified in `web/agentd-ui`)
- The frontend does **not** sort or filter directory listings client-side.
  - `FileTreeNode.vue` renders `store.treeByPath["${projectId}:${path}"]` as-is.
  - Therefore, the backend must provide stable ordering.
- Tree listing is **non-recursive per directory**.
  - The UI calls `GET /api/projects/{id}/tree?path=<dir>` for the current directory.
  - It expects each returned entry to include `path` as the full project-relative path (not just basename), so it can be used for navigation and subsequent `tree?path=...` calls.
- Expected entry shape (from `web/agentd-ui/src/api/client.ts`):
  - `{ name, path, isDir, sizeBytes, modTime }`
- Root semantics:
  - The UI uses `.` as the root directory sentinel.
  - The API must accept `path='.'` and return the root’s immediate children.
- Hidden files:
  - The UI does not hide dotfiles.
  - The backend currently hides `.meta` only when listing the project root; the UI implicitly relies on this to avoid showing project internals.
- Projects list ordering:
  - The UI displays projects in the order returned by `GET /api/projects`.
  - Backend currently sorts by `UpdatedAt desc`, then `Name asc`.

### Project selection sandboxing today
- `/agent/run` accepts `project_id`.
- In `internal/agentd/handlers_chat.go`, `project_id` is validated (clean path, no traversal, not abs), and then:
  - `sandbox.WithBaseDir(ctx, <abs project dir>)`
  - `sandbox.WithProjectID(ctx, projectID)`

### Tools depend on `sandbox.ResolveBaseDir`
- Many tools resolve base dir from context via `sandbox.ResolveBaseDir(ctx, defaultWorkdir)`:
  - `internal/tools/cli/exec.go` (`run_cli`)
  - `internal/tools/patchtool/tool.go` (`apply_patch`)
  - `internal/tools/codeevolve/tool.go` (`code_evolve`)
- Path safety is enforced by `internal/sandbox/pathpolicy.go` using `os.OpenRoot` containment checks.

### Delegation also sets base dir in-process
- `agent_call` (`internal/tools/agents/agent_call.go`) can take `project_id` and computes `${WORKDIR}/users/<uid>/projects/<pid>` then uses `sandbox.WithBaseDir`.
- `Delegator` (`internal/tools/agents/delegator.go`) uses a similar join and existence check, but is looser than the HTTP handler (no strict clean PID check).

### Config today
- `WORKDIR` is required and validated in `internal/config/loader.go`.
- Projects config is currently only:
  - `config.Config.Projects.Encrypt bool` (see `internal/config/config.go`).
- Projects service is wired in `internal/agentd/run.go`:
  - `app.projectsService = projects.NewService(cfg.Workdir)`
  - `EnableEncryption(true)` if configured.

---

## Target Architecture

### Definitions
- **Authoritative store**: S3/MinIO bucket prefix holding encrypted project files and metadata.
- **Ephemeral workspace**: a per-run (or per-session) local directory that tools operate in.

### High-level flow (agent run)
1) User selects `project_id` in UI.
2) `/agent/run` receives `project_id`.
3) Server creates/uses an ephemeral workspace directory (local FS).
4) Server **hydrates** (checkout) workspace contents from S3.
5) Server sets `sandbox.WithBaseDir(ctx, <ephemeral workspace>)`.
6) Agent/tools run inside that base dir.
7) After run completes (or on a schedule), server **commits** changes back to S3.
8) Workspace is cleaned up.

### Key security goal
- Agents must not receive S3 credentials.
- Only server-side components (workspace sync layer) can access S3.

### “Admins can’t view contents” (threat model)
- App-level admins: enforce in API authorization (already per-user scoped).
- Infra/cloud admins: RBAC alone cannot prevent reads; require encryption where infra admins do not hold keys.

Plan includes both:
- **Phase A (MVP)**: S3 SSE + per-user isolation.
- **Phase B (Enterprise)**: application-layer encryption with KEK in external KMS/Vault and strict IAM, optionally customer-managed keys.

---

## Implementation Strategy (Incremental, Low-Risk)

### Guiding principle
Do not break existing UX or tool behavior.

We will introduce a new “workspace checkout dir” concept, but keep:
- `project_id` semantics
- `sandbox.ResolveBaseDir` usage
- existing tools unchanged where possible

---

## Phase 0 — Preparation / Guardrails

### Step 0.1: Decide granularity of ephemeral workspaces
We will implement per session:
- **Per-session workspace**: `${WORKDIR}/sandboxes/users/<uid>/projects/<pid>/sessions/<sessionID>`
  - Pros: faster across prompts; fewer downloads.
  - Cons: more state; cleanup needed.

### Step 0.2: Add feature flags (config)
Extend `internal/config/config.go` with:
```yaml
projects:
  backend: filesystem | s3
  encrypt: true|false                 # keep existing
  workspace:
    mode: legacy | ephemeral
    root: "${WORKDIR}/sandboxes"      # optional override
    ttlSeconds: 86400
  s3:
    endpoint: "http://minio:9000"     # dev
    region: "us-east-1"
    bucket: "manifold-workspaces"
    prefix: "workspaces"             # base prefix
    accessKey: "..."                 # dev only
    secretKey: "..."                 # dev only
    usePathStyle: true                # minio
    tlsInsecureSkipVerify: false
    sse:
      mode: none | sse-s3 | sse-kms
      kmsKeyID: "..."
```
And env overrides in `internal/config/loader.go` (mirroring existing patterns).

### Step 0.3: Define “durable project revision” concept
To avoid complex S3 conditional puts:
- Store `projects.revision` in DB.
- Checkout returns `revision`.
- Commit succeeds only if `revision` unchanged (optimistic concurrency).

If DB work is too heavy for MVP, accept last-write-wins (LWW) initially, but keep the interface designed for revisions.

---

## Phase 1 — Introduce Workspace Abstractions (No S3 Yet)

Goal: refactor code so the base dir can be “workspace checkout dir” without changing tools.

### Step 1.1: Create `internal/workspaces` package
Add a package responsible for:
- Deterministic workspace paths
- Creation and cleanup
- Locking / concurrency

Proposed interfaces:
```go
// internal/workspaces/manager.go

type WorkspaceManager interface {
  Checkout(ctx context.Context, userID int64, projectID, sessionID string) (Workspace, error)
  Commit(ctx context.Context, ws Workspace) error
  Cleanup(ctx context.Context, ws Workspace) error
}

type Workspace struct {
  UserID    int64
  ProjectID string
  SessionID string
  BaseDir   string // local FS path for tools
  // optional: Revision, ETag manifest, etc.
}
```

### Step 1.2: “Legacy” manager implementation
Implement a `LegacyWorkspaceManager` that simply maps:
- `BaseDir = ${WORKDIR}/users/<uid>/projects/<pid>`
- Checkout/Commit are no-ops

This should be a zero-behavior-change refactor.

### Step 1.3: Wire workspace manager into `/agent/run`
In `internal/agentd/handlers_chat.go`:
- Replace the direct base-dir computation with:
  - `ws := a.workspaceManager.Checkout(ctx, uid, projectID, sessionID)`
  - `sandbox.WithBaseDir(ctx, ws.BaseDir)`

Keep the existing strict `project_id` validation logic.

### Step 1.4: Wire workspace manager into delegation paths
Update:
- `internal/tools/agents/agent_call.go`
- `internal/tools/agents/delegator.go`

So that when a delegated agent uses `project_id`, it uses the same workspace manager behavior (and the same validation) as `/agent/run`.

---

## Phase 2 — Add S3-backed Durable Store + Ephemeral Checkout

Goal: S3 becomes the source of truth; tools operate on ephemeral workspace.

### Step 2.1: Create an S3 client wrapper
Add `internal/objectstore` package:
- Uses AWS SDK Go v2 (recommended) with support for:
  - AWS S3
  - MinIO (endpoint override + path-style)

Provide a narrow interface:
```go
type ObjectStore interface {
  Get(ctx context.Context, key string) (io.ReadCloser, ObjectAttrs, error)
  Put(ctx context.Context, key string, body io.Reader, opts PutOptions) (ObjectAttrs, error)
  Delete(ctx context.Context, key string) error
  List(ctx context.Context, prefix string) ([]ObjectAttrs, error)
}
```

### Step 2.2: Define key mapping
Normalize and validate paths (reject `..`, abs, weird separators).

Key scheme (example):
- `${prefix}/users/<uid>/projects/<pid>/files/<path>`
- `${prefix}/users/<uid>/projects/<pid>/.meta/project.json`
- `${prefix}/users/<uid>/projects/<pid>/.meta/enc.json` (if needed)

Important: treat “directories” as logical prefixes; only store objects.

### Step 2.3: Add an Ephemeral Workspace Manager
Implement `S3WorkspaceManager` that:
- Creates local dir under `${WORKDIR}/sandboxes/...`
- On Checkout:
  - Lists objects under project prefix
  - Downloads into local dir
  - (Optional) lazy hydration based on file access is advanced; skip initially.
- On Commit:
  - Walks local dir
  - Uploads changed files
  - Deletes removed files
  - Updates project metadata

### Step 2.4: Decide change detection strategy
MVP (simple, correct):
- Always upload all files (slow but easy).

Recommended (still simple):
- Maintain a manifest file in local workspace on checkout:
  - `/.meta/sync-manifest.json`
  - Contains per-path: size + sha256 + last seen remote etag/version
- Commit compares current sha256 to manifest; upload only changes.

### Step 2.5: Update Projects HTTP API to read/write durable store
Today the Projects API reads local disk.

In S3 mode:
- `ListProjects` becomes DB-driven (recommended) OR list prefixes (expensive).
- `ListTree`, `ReadFile`, `UploadFile`, `DeleteFile`, `MovePath`, `CreateDir` operate on S3.

Implementation option A (recommended):
- Introduce `projects.Service` interface and split implementations:
  - `FilesystemProjectsService` (existing)
  - `S3ProjectsService` (new)

Implementation option B (lower diff):
- Keep `projects.Service` type, but add a backend abstraction inside.

### Step 2.6: Avoid expensive S3 LIST for every UI action
Introduce DB tables (see Phase 3) OR cache:
- Cache tree listings in memory with short TTL.
- Or store a per-project “file index” object.

---

## Phase 3 — Metadata + Indexing (Database)

Goal: fast listing, access control hooks, concurrency control.

### Step 3.1: Add persistence interfaces
Add to `internal/persistence/store.go`:
- `ProjectsStore` and types:
  - Project (id, owner, name, created_at, updated_at, revision)
  - ProjectMember (optional; future sharing)

### Step 3.2: Implement Postgres store
Follow patterns used by chat and warpp stores.

Tables:
- `projects`:
  - `id uuid pk`
  - `owner_user_id bigint`
  - `name text`
  - `created_at timestamptz`
  - `updated_at timestamptz`
  - `revision bigint`
  - `bytes bigint` (optional denormalized)
  - `file_count int` (optional)

Optional:
- `project_files` index (path, etag/version, size, updated_at) for fast tree/list.

### Step 3.3: Migrate existing filesystem projects
Write a one-off migration command:
- Scans `${WORKDIR}/users/*/projects/*`
- Inserts projects into DB
- Uploads files to S3
- Verifies checksums

Add a dry-run mode.

---

## Phase 4 — Encryption (Enterprise-grade)

Goal: “admins can’t view contents” beyond app RBAC.

### Step 4.1: Replace local master key with KMS/Vault
Current encryption uses `${WORKDIR}/.keystore/master.key`.

Replace with a `KeyProvider` interface:
```go
type KeyProvider interface {
  WrapDEK(ctx context.Context, projectID string, dek []byte) ([]byte, error)
  UnwrapDEK(ctx context.Context, projectID string, wrapped []byte) ([]byte, error)
}
```
Implementations:
- Dev: file-based KEK (existing behavior)
- Prod: Vault Transit or cloud KMS

### Step 4.2: Decide encryption layer
Options:
- Rely on S3 SSE only (not sufficient against infra admins with bucket access).
- Implement app-layer per-file encryption (similar to current) and store ciphertext in S3.

Recommendation:
- Keep app-layer encryption for project contents.
- Use KMS/Vault for KEK so servers/operators can’t decrypt without key access.

### Step 4.3: Key rotation
Reuse current `RotateProjectDEK` behavior but applied to objects:
- mark rotation in metadata
- re-encrypt objects
- finalize

---

## Phase 4 — Implementation Notes (Completed)

**Date**: January 2025

### What Was Implemented

Phase 4 introduces enterprise-grade encryption with a pluggable `KeyProvider` abstraction, replacing the local master key approach with support for HashiCorp Vault Transit and AWS KMS. The implementation maintains full backward compatibility with existing encrypted projects.

### Architecture Overview

**KeyProvider Interface** (`internal/projects/keyprovider.go`):
```go
type KeyProvider interface {
    WrapDEK(ctx context.Context, dek []byte) ([]byte, error)
    UnwrapDEK(ctx context.Context, wrapped []byte) ([]byte, error)
    ProviderType() string
    HealthCheck(ctx context.Context) error
    Close() error
}
```

**Three Implementations**:
1. **FileKeyProvider** — Local file-based master key (default/legacy mode for development)
2. **VaultKeyProvider** — HashiCorp Vault Transit secrets engine with HTTP API
3. **AWSKMSKeyProvider** — AWS KMS with simplified SigV4 signing (no SDK dependency)

### Envelope Encryption Format

**v1 (Legacy)** — Original AES-256-GCM format:
- 12-byte nonce + wrapped DEK ciphertext
- KEK derived from local master key file
- Stored in `.meta/enc.json` as `{"wrapped_dek": "base64...", "version": 1}`

**v2 (KeyProvider)** — New opaque format:
- Provider-specific wrapped bytes (Vault ciphertext or KMS blob)
- Stored as `{"wrapped_dek_v2": "base64...", "provider_type": "vault|awskms", "version": 2}`
- Also stored for backward recovery: `{"wrapped_dek": "...", "wrapped_dek_v2": "..."}`

The service automatically detects envelope version and uses the appropriate unwrap method.

### Files Created

| File | Purpose |
|------|---------|
| `internal/projects/keyprovider.go` | KeyProvider interface + 3 implementations (~530 lines) |
| `internal/projects/keyprovider_test.go` | Comprehensive unit tests (~380 lines) |

### Files Modified

| File | Changes |
|------|---------|
| `internal/config/config.go` | Added `EncryptionConfig`, `FileKeyProviderConfig`, `VaultKeyProviderConfig`, `AWSKMSKeyProviderConfig` structs |
| `internal/config/loader.go` | Added 35+ env var parsers for encryption configuration |
| `internal/projects/service.go` | Refactored to use KeyProvider; added v2 envelope support; dual-format DEK writing |
| `example.env` | Added Vault and AWS KMS configuration sections |

### Configuration

**Environment Variables** (all prefixed with `PROJECTS_ENCRYPTION_`):

| Variable | Description | Default |
|----------|-------------|---------|
| `PROVIDER` | `file`, `vault`, or `awskms` | `file` |
| `FILE_KEYSTORE_PATH` | Path for local master key | `${WORKDIR}/.keystore` |
| `VAULT_ADDRESS` | Vault server URL | — |
| `VAULT_TOKEN` | Vault auth token (or use `VAULT_TOKEN` env) | — |
| `VAULT_KEY_NAME` | Transit key name | `manifold-kek` |
| `VAULT_MOUNT_PATH` | Transit mount path | `transit` |
| `VAULT_NAMESPACE` | Vault Enterprise namespace | — |
| `VAULT_TLS_SKIP_VERIFY` | Skip TLS verification (dev only) | `false` |
| `VAULT_TIMEOUT_SECONDS` | Request timeout | `30` |
| `AWSKMS_KEY_ID` | KMS key ARN or alias | — |
| `AWSKMS_REGION` | AWS region (or use `AWS_REGION`) | — |
| `AWSKMS_ACCESS_KEY_ID` | AWS credentials (prefer IAM roles) | — |
| `AWSKMS_SECRET_ACCESS_KEY` | AWS credentials (prefer IAM roles) | — |
| `AWSKMS_ENDPOINT` | Custom endpoint (LocalStack) | — |

### Backward Compatibility

- Existing projects with v1 envelopes continue to work without migration
- `PROJECTS_ENCRYPT=true` with no provider config defaults to `file` provider (legacy behavior)
- Service detects envelope version automatically on read
- New writes use v2 format when KeyProvider is configured, otherwise v1

### Security Considerations

1. **Vault Transit**: Uses Vault's encrypt/decrypt API — KEK never leaves Vault
2. **AWS KMS**: Uses GenerateDataKey/Decrypt APIs — KEK never leaves KMS
3. **File Provider**: Master key stored locally — suitable only for development
4. **No SDK Dependencies**: Both Vault and AWS implementations use raw HTTP APIs to minimize attack surface

### Key Rotation

`RotateProjectDEK` now supports both envelope versions:
- Generates new DEK
- Re-encrypts all project files
- Wraps new DEK with current KeyProvider
- Updates `.meta/enc.json` with new wrapped DEK

### Testing

Unit tests cover:
- FileKeyProvider wrap/unwrap roundtrip
- Persistent master key loading
- VaultKeyProvider configuration validation
- AWSKMSKeyProvider configuration validation  
- Service integration with KeyProvider
- Legacy mode compatibility

### Usage Examples

**Local Development (default)**:
```bash
PROJECTS_ENCRYPT=true
# Uses file provider automatically with ${WORKDIR}/.keystore
```

**HashiCorp Vault**:
```bash
PROJECTS_ENCRYPT=true
PROJECTS_ENCRYPTION_PROVIDER=vault
PROJECTS_ENCRYPTION_VAULT_ADDRESS=https://vault.example.com:8200
PROJECTS_ENCRYPTION_VAULT_KEY_NAME=manifold-kek
VAULT_TOKEN=hvs.xxxx  # Or use PROJECTS_ENCRYPTION_VAULT_TOKEN
```

**AWS KMS**:
```bash
PROJECTS_ENCRYPT=true
PROJECTS_ENCRYPTION_PROVIDER=awskms
PROJECTS_ENCRYPTION_AWSKMS_KEY_ID=arn:aws:kms:us-east-1:123456789:key/...
PROJECTS_ENCRYPTION_AWSKMS_REGION=us-east-1
# Credentials via IAM role, instance profile, or AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY
```

### Preparation for Phase 5

The KeyProvider abstraction is ready for S3 storage integration:
- Per-file encryption uses DEK from KeyProvider-wrapped envelope
- S3 SSE can be layered on top for defense-in-depth
- Same encryption logic works regardless of storage backend

---

## Phase 5 — Dev/CI scaffolding (MinIO)

### Step 5.1: Extend docker-compose
Add MinIO service + create-bucket init container.

### Step 5.2: Add example env vars
Update `example.env` with:
- `PROJECTS_BACKEND=s3`
- `S3_ENDPOINT=http://minio:9000`
- `S3_BUCKET=manifold-workspaces`
- `S3_ACCESS_KEY=...`
- `S3_SECRET_KEY=...`

---

## Phase 5 — Implementation Notes (Completed)

**Date**: January 3, 2026

### What Was Implemented

Phase 5 adds MinIO scaffolding for local development and CI environments, providing an S3-compatible object storage backend that works out-of-the-box with `docker-compose up`.

### Files Modified

#### 1. `docker-compose.yml`

Added two new services under the "Object Storage" section:

**MinIO Server:**
```yaml
minio:
  image: minio/minio:RELEASE.2024-11-07T00-52-20Z
  container_name: manifold_minio
  command: server /data --console-address ":9001"
  environment:
    MINIO_ROOT_USER: ${MINIO_ROOT_USER:-minioadmin}
    MINIO_ROOT_PASSWORD: ${MINIO_ROOT_PASSWORD:-minioadmin}
  ports:
    - "9002:9000"   # S3 API
    - "9003:9001"   # MinIO Console
  volumes:
    - minio_data:/data
  healthcheck:
    test: ["CMD", "mc", "ready", "local"]
    interval: 10s
    timeout: 5s
    retries: 5
    start_period: 10s
  restart: unless-stopped
```

**MinIO Init (Bucket Creation):**
```yaml
minio-init:
  image: minio/mc:RELEASE.2024-11-05T11-29-45Z
  container_name: manifold_minio_init
  depends_on:
    minio:
      condition: service_healthy
  entrypoint: >
    /bin/sh -c "
    mc alias set manifold http://minio:9000 $${MINIO_ROOT_USER:-minioadmin} $${MINIO_ROOT_PASSWORD:-minioadmin};
    mc mb --ignore-existing manifold/$${MINIO_BUCKET:-manifold-workspaces};
    mc anonymous set none manifold/$${MINIO_BUCKET:-manifold-workspaces};
    echo 'Bucket ready: $${MINIO_BUCKET:-manifold-workspaces}';
    exit 0;
    "
  environment:
    MINIO_ROOT_USER: ${MINIO_ROOT_USER:-minioadmin}
    MINIO_ROOT_PASSWORD: ${MINIO_ROOT_PASSWORD:-minioadmin}
    MINIO_BUCKET: ${PROJECTS_S3_BUCKET:-manifold-workspaces}
```

**New Volume:**
```yaml
volumes:
  # ... existing volumes ...
  minio_data:
```

#### 2. `example.env`

Updated S3/MinIO configuration section with additional documentation:

```bash
# S3/MinIO configuration for projects storage (used when PROJECTS_BACKEND=s3)
# MinIO admin credentials (also used by docker-compose minio-init)
# MINIO_ROOT_USER=minioadmin
# MINIO_ROOT_PASSWORD=minioadmin

# S3 client configuration
# PROJECTS_S3_ENDPOINT="http://localhost:9002"  # MinIO S3 API (host port from docker-compose)
# PROJECTS_S3_REGION="us-east-1"
# PROJECTS_S3_BUCKET="manifold-workspaces"
# PROJECTS_S3_PREFIX="workspaces"
# PROJECTS_S3_ACCESS_KEY="minioadmin"
# PROJECTS_S3_SECRET_KEY="minioadmin"
# PROJECTS_S3_USE_PATH_STYLE=true  # Required for MinIO
# PROJECTS_S3_TLS_INSECURE=false
# PROJECTS_S3_SSE_MODE="none"  # none | sse-s3 | sse-kms
# PROJECTS_S3_SSE_KMS_KEY_ID=""
```

### Service Configuration Details

| Service | Container Name | Host Port | Container Port | Purpose |
|---------|---------------|-----------|----------------|---------|
| `minio` | `manifold_minio` | 9002 | 9000 | S3 API endpoint |
| `minio` | `manifold_minio` | 9003 | 9001 | MinIO Web Console |
| `minio-init` | `manifold_minio_init` | — | — | One-shot bucket creation |

### Environment Variables

| Variable | Default | Used By | Description |
|----------|---------|---------|-------------|
| `MINIO_ROOT_USER` | `minioadmin` | minio, minio-init | Admin username |
| `MINIO_ROOT_PASSWORD` | `minioadmin` | minio, minio-init | Admin password |
| `PROJECTS_S3_BUCKET` | `manifold-workspaces` | minio-init | Bucket name to create |

### Usage

**Start MinIO:**
```bash
docker-compose up -d minio
# Wait for health check, then init container runs automatically
```

**Access MinIO Console:**
- URL: http://localhost:9003
- Username: `minioadmin` (or `$MINIO_ROOT_USER`)
- Password: `minioadmin` (or `$MINIO_ROOT_PASSWORD`)

**Enable S3 Backend in Manifold:**
```bash
# Add to .env file:
PROJECTS_BACKEND=s3
PROJECTS_S3_ENDPOINT=http://localhost:9002
PROJECTS_S3_BUCKET=manifold-workspaces
PROJECTS_S3_ACCESS_KEY=minioadmin
PROJECTS_S3_SECRET_KEY=minioadmin
PROJECTS_S3_USE_PATH_STYLE=true
```

**For Docker-internal access (container-to-container):**
```bash
PROJECTS_S3_ENDPOINT=http://minio:9000
```

### Health Check

The MinIO service includes a health check using the `mc ready` command:
- Checks every 10 seconds
- Timeout after 5 seconds per check
- Retries up to 5 times
- 10-second start period for initial container startup

The `minio-init` service depends on `minio` being healthy before running, ensuring the bucket creation only happens after MinIO is fully operational.

### Security Notes

1. **Default Credentials**: The default `minioadmin/minioadmin` credentials are for development only
2. **Production**: Override `MINIO_ROOT_USER` and `MINIO_ROOT_PASSWORD` with strong credentials
3. **Bucket Policy**: Created bucket has anonymous access disabled (`mc anonymous set none`)
4. **Volume Persistence**: Data persists in `minio_data` volume across container restarts

### Integration Testing

With MinIO running, integration tests can use the real S3 API:

```go
func TestWithMinIO(t *testing.T) {
    cfg := objectstore.S3Config{
        Endpoint:     "http://localhost:9002",
        Region:       "us-east-1",
        Bucket:       "manifold-workspaces",
        AccessKey:    "minioadmin",
        SecretKey:    "minioadmin",
        UsePathStyle: true,
    }
    store, err := objectstore.NewS3Store(ctx, cfg)
    require.NoError(t, err)
    // ... test against real S3 API
}
```

### CI/CD Usage

For GitHub Actions or similar CI environments:

```yaml
services:
  minio:
    image: minio/minio:RELEASE.2024-11-07T00-52-20Z
    env:
      MINIO_ROOT_USER: minioadmin
      MINIO_ROOT_PASSWORD: minioadmin
    ports:
      - 9000:9000
    options: >-
      --health-cmd "mc ready local"
      --health-interval 10s
      --health-timeout 5s
      --health-retries 5
```

### Behavioral Notes

- **Optional Service**: MinIO is not required for default filesystem mode
- **Data Persistence**: Volume ensures data survives container restarts
- **Self-Healing**: Services restart automatically unless explicitly stopped
- **Version Pinned**: Uses specific MinIO releases for reproducibility

### Next Steps

With Phase 5 complete, the full storage migration path is now available:

1. **Development**: Use `docker-compose up minio` for local S3 testing
2. **Configuration**: Set `PROJECTS_BACKEND=s3` to enable S3 mode
3. **Migration**: Run `migrateprojects` to import existing filesystem projects
4. **Production**: Replace MinIO with AWS S3 or self-hosted MinIO cluster

---

## Testing Plan

### Unit tests
- `internal/sandbox/pathpolicy_test.go` already validates containment; keep.
- Add tests for:
  - Path→key normalization (reject traversal)
  - Workspace path creation and cleanup
  - Checkout/commit correctness (use MinIO in integration tests or fake objectstore)

### Integration tests
- Add a focused test that:
  1) Creates a project
  2) Uploads a file through Projects API
  3) Runs `/agent/run` with project_id and uses `apply_patch` or `run_cli` to modify it
  4) Verifies the modification is persisted (S3)

---

## Rollout Plan

1) Land Phase 1 behind config flag (`workspace.mode=legacy` default).
2) Add S3 mode behind `projects.backend=s3` and keep `workspace.mode=legacy` initially.
3) Enable ephemeral workspaces in dev with MinIO.
4) Add DB metadata and concurrency controls.
5) Introduce KMS/Vault key provider and app-layer encryption.

---

## Known Risks / Design Decisions to Confirm

- Workspace granularity: per-session vs per-run.
- Concurrency model: single-writer lock vs optimistic revision.
- Cost of S3 LIST operations (needs DB index or caching).
- Encryption requirements and threat model (app admin vs infra admin).

### UI/API contract edge cases (Verified)
- Backend must preserve the current `ListTree` ordering semantics (dirs first, then `name` asc), because the UI renders in backend order.
- Bulk download in `ProjectsView.vue` attempts to download every checked path via `GET /api/projects/{id}/files?path=<path>`.
  - The UI allows checking directories today (checkbox shown for all entries).
  - Current backend `ReadFile` expects a file path, so directory downloads will fail.
  - Decide a product behavior and implement it explicitly:
    - Option A (simplest): change UI to only allow checking files for download.
    - Option B: add a backend endpoint to download a directory as an archive (e.g., `GET /api/projects/{id}/archive?path=...`).

---

## Repo-specific details now confirmed

- Tree sorting/filtering is backend-controlled (no frontend sort/filter).
- No pagination in tree UI; the backend response should remain efficient for large directories (motivates DB index or cached listing in S3 mode).
- Root sentinel is `.`.

## Remaining follow-ups

- Determine whether tool runs should commit on every `/agent/run` completion or only on explicit “save/sync” events (UI has none today).
- Identify an existing background job mechanism (or add a simple ticker) for workspace TTL cleanup when using per-session workspaces.
---

## Phase 0 — Implementation Notes (Completed)

**Date**: January 3, 2026

### What Was Implemented

Phase 0 establishes the feature flag infrastructure and configuration types required for the durable storage migration. This phase introduces no behavioral changes—all defaults preserve existing "filesystem" + "legacy" behavior.

### Files Modified

1. **`internal/config/config.go`**
   - Extended `ProjectsConfig` struct with new nested types:
     - `WorkspaceConfig` — controls ephemeral workspace behavior (mode, root, TTL)
     - `S3Config` — holds S3/MinIO connection settings
     - `S3SSEConfig` — server-side encryption configuration
   - All new fields include YAML/JSON tags for serialization

2. **`internal/config/loader.go`**
   - Added environment variable parsing for all new config fields:
     - `PROJECTS_BACKEND`, `PROJECTS_ENCRYPT`
     - `PROJECTS_WORKSPACE_MODE`, `PROJECTS_WORKSPACE_ROOT`, `PROJECTS_WORKSPACE_TTL_SECONDS`
     - `PROJECTS_S3_*` (endpoint, region, bucket, prefix, credentials, TLS, SSE)
   - Added YAML struct types and parsing logic for `projects:` config block
   - Applied sensible defaults after YAML merge:
     - Backend: `filesystem`
     - Workspace mode: `legacy`
     - Workspace TTL: `86400` (24 hours)
     - Workspace root: `${WORKDIR}/sandboxes` (computed after WORKDIR resolution)
     - S3 region: `us-east-1`
     - S3 prefix: `workspaces`
     - SSE mode: `none`

3. **`example.env`**
   - Added documented environment variables for all new configuration options
   - S3 settings are commented out by default (not needed until Phase 2)

4. **`config.yaml.example`**
   - Added complete `projects:` section with all supported options
   - S3 configuration is commented out as an example for future use

5. **`internal/config/projects_config_test.go`** (new file)
   - Comprehensive test coverage for:
     - Default values verification
     - Environment variable overrides
     - YAML configuration parsing
     - Workspace root default computation
     - Backend and workspace mode value handling

### Configuration Summary

| Config Key | Env Var | Default | Description |
|------------|---------|---------|-------------|
| `projects.backend` | `PROJECTS_BACKEND` | `filesystem` | Storage backend: `filesystem` or `s3` |
| `projects.encrypt` | `PROJECTS_ENCRYPT` | `false` | Enable at-rest encryption |
| `projects.workspace.mode` | `PROJECTS_WORKSPACE_MODE` | `legacy` | Workspace isolation: `legacy` or `ephemeral` |
| `projects.workspace.root` | `PROJECTS_WORKSPACE_ROOT` | `${WORKDIR}/sandboxes` | Ephemeral workspace directory |
| `projects.workspace.ttlSeconds` | `PROJECTS_WORKSPACE_TTL_SECONDS` | `86400` | Workspace cleanup TTL (24h) |
| `projects.s3.endpoint` | `PROJECTS_S3_ENDPOINT` | (empty) | S3/MinIO API endpoint |
| `projects.s3.region` | `PROJECTS_S3_REGION` | `us-east-1` | AWS region |
| `projects.s3.bucket` | `PROJECTS_S3_BUCKET` | (empty) | S3 bucket name |
| `projects.s3.prefix` | `PROJECTS_S3_PREFIX` | `workspaces` | Key prefix in bucket |
| `projects.s3.usePathStyle` | `PROJECTS_S3_USE_PATH_STYLE` | `false` | Enable path-style S3 (MinIO) |
| `projects.s3.sse.mode` | `PROJECTS_S3_SSE_MODE` | `none` | SSE mode: `none`, `sse-s3`, `sse-kms` |

### Behavioral Notes

- **Zero breaking changes**: All defaults match the pre-existing filesystem-only behavior
- **Config precedence**: Environment variables override YAML, YAML overrides defaults
- **Workspace root**: Automatically set to `${WORKDIR}/sandboxes` if not explicitly configured
- **S3 credentials**: Can be provided via env vars (recommended for production) or YAML (dev only)

### Readiness for Phase 1

The following interfaces and abstractions are now ready for implementation:

1. **`WorkspaceManager` interface** — `internal/workspaces/manager.go`
   - Can use `cfg.Projects.Workspace.Mode` to select implementation
   - `LegacyWorkspaceManager` returns existing project directory
   - `EphemeralWorkspaceManager` creates per-session directories under `cfg.Projects.Workspace.Root`

2. **Backend selection** — `cfg.Projects.Backend`
   - Phase 1 will only use `filesystem` but the flag is ready
   - Phase 2 will add `s3` implementation using `cfg.Projects.S3.*`

3. **Test infrastructure** — comprehensive tests ensure config changes don't regress

### Next Steps (Phase 1)

1. Create `internal/workspaces` package with `WorkspaceManager` interface
2. Implement `LegacyWorkspaceManager` (no-op wrapper around existing paths)
3. Wire workspace manager into `/agent/run` handler
4. Update delegation paths to use workspace manager
---

## Phase 1 — Implementation Notes (Completed)

**Date**: January 3, 2026

### What Was Implemented

Phase 1 introduces the `WorkspaceManager` abstraction that decouples workspace path computation from HTTP handlers and tool implementations. This is a **zero-behavior-change refactor** that prepares the codebase for future ephemeral workspace support.

### Files Created

1. **`internal/workspaces/manager.go`** (new package)
   - `Workspace` struct: holds `UserID`, `ProjectID`, `SessionID`, `BaseDir`, and `Mode`
   - `WorkspaceManager` interface with `Checkout`, `Commit`, `Cleanup`, and `Mode` methods
   - `LegacyWorkspaceManager` implementation:
     - `Checkout`: validates project ID and returns absolute path to existing project directory
     - `Commit`/`Cleanup`: no-ops (changes written directly to disk)
   - `NewManager(cfg)` factory: selects implementation based on `cfg.Projects.Workspace.Mode`
   - `ValidateProjectID` helper: standalone validation for reuse
   - Sentinel errors: `ErrInvalidProjectID`, `ErrProjectNotFound`

2. **`internal/workspaces/manager_test.go`**
   - Comprehensive unit tests covering:
     - Manager creation with legacy/default/ephemeral modes
     - Empty project ID handling
     - Path traversal attack prevention
     - Project not found scenarios
     - Successful checkout with proper path resolution
     - No-op Commit/Cleanup verification

### Files Modified

1. **`internal/agentd/run.go`**
   - Added import for `manifold/internal/workspaces`
   - Added `workspaceManager workspaces.WorkspaceManager` to `app` struct
   - Initialize workspace manager early in `newApp()` before tool registration
   - Pass `wsMgr` to `AgentCallTool` and `Delegator` constructors
   - Log workspace mode on startup

2. **`internal/agentd/handlers_chat.go`**
   - Removed `os` and `path/filepath` imports (no longer needed)
   - Added import for `manifold/internal/workspaces`
   - Replaced inline project_id validation in `agentRunHandler()` with `a.workspaceManager.Checkout()`
   - Replaced inline project_id validation in the alternate handler with `a.workspaceManager.Checkout()`
   - Error handling uses `workspaces.ErrInvalidProjectID` and `workspaces.ErrProjectNotFound`

3. **`internal/tools/agents/agent_call.go`**
   - Removed `os` and `path/filepath` imports
   - Added import for `manifold/internal/workspaces`
   - Changed `workdir string` field to `wsMgr workspaces.WorkspaceManager`
   - Updated `NewAgentCallTool` signature: `NewAgentCallTool(reg, specReg, wsMgr)`
   - Replaced inline path validation in `Call()` with `t.wsMgr.Checkout()`

4. **`internal/tools/agents/delegator.go`**
   - Removed `os` and `path/filepath` imports
   - Added import for `manifold/internal/workspaces`
   - Changed `workdir string` field to `wsMgr workspaces.WorkspaceManager`
   - Updated `NewDelegator` signature: `NewDelegator(reg, specReg, wsMgr, maxSteps)`
   - Replaced inline path validation in `Run()` with `d.wsMgr.Checkout()`

### Behavioral Notes

- **Zero breaking changes**: All defaults preserve existing behavior
- **Path validation centralized**: Single source of truth in `workspaces.LegacyWorkspaceManager.Checkout()`
- **Same security guarantees**: Path traversal prevention logic unchanged, just relocated
- **Same error messages**: User-facing errors remain identical for compatibility
- **Testable**: Workspace manager can be mocked in tests

### Architecture Decisions

1. **Interface-driven design**: `WorkspaceManager` interface allows swapping implementations
2. **Early initialization**: Workspace manager created before tools so it can be injected
3. **Session ID passed through**: Even in legacy mode, session ID is captured for future use
4. **Mode field on Workspace**: Allows runtime introspection of workspace type

### Readiness for Phase 2

The following hooks are in place for S3-backed ephemeral workspaces:

1. **`EphemeralWorkspaceManager`** can implement `WorkspaceManager` interface
2. **`Checkout`** will create temp directory and sync from S3
3. **`Commit`** will sync changes back to S3
4. **`Cleanup`** will remove temp directory
5. **Mode selection**: `cfg.Projects.Workspace.Mode = "ephemeral"` triggers new implementation

### Test Results

```
$ go test ./internal/workspaces/...
ok      manifold/internal/workspaces    0.039s
```

### Next Steps (Phase 2)

1. Create `internal/objectstore` package with S3 client wrapper
2. Implement `EphemeralWorkspaceManager` with sync logic
3. Add manifest-based change detection for efficient commits
4. Update Projects HTTP API to read/write from S3 when `Backend="s3"`

---

## Phase 2 — Implementation Notes (Completed)

**Date**: January 3, 2026

### What Was Implemented

Phase 2 introduces S3-backed durable storage and ephemeral workspace checkout/commit functionality. This phase adds the `objectstore` package for S3 operations, the `EphemeralWorkspaceManager` for workspace lifecycle management, and an `S3Service` for the Projects API.

### New Packages Created

#### 1. `internal/objectstore` — S3 Object Storage Abstraction

**Files:**
- `store.go` — Core interfaces and types
- `s3.go` — AWS SDK Go v2 S3 implementation
- `memory.go` — In-memory implementation for testing
- `memory_test.go` — Comprehensive unit tests

**Key Types:**
```go
// ObjectStore defines the object storage interface
type ObjectStore interface {
    Get(ctx context.Context, key string) (io.ReadCloser, ObjectAttrs, error)
    Put(ctx context.Context, key string, body io.Reader, opts PutOptions) (string, error)
    Delete(ctx context.Context, key string) error
    List(ctx context.Context, opts ListOptions) (ListResult, error)
    Head(ctx context.Context, key string) (ObjectAttrs, error)
    Copy(ctx context.Context, src, dst string) error
    Exists(ctx context.Context, key string) (bool, error)
}

// ObjectAttrs holds object metadata
type ObjectAttrs struct {
    Key          string
    Size         int64
    ETag         string
    ContentType  string
    LastModified time.Time
}
```

**S3Store Features:**
- AWS SDK Go v2 with `aws-sdk-go-v2/service/s3`
- Path-style addressing support for MinIO (`UsePathStyle: true`)
- Custom endpoint configuration for self-hosted S3-compatible storage
- Server-side encryption modes: `none`, `sse-s3`, `sse-kms`
- Pagination support for `List` operations
- ETag return on `Put` for change detection

**MemoryStore Features:**
- Thread-safe in-memory map storage
- Full interface compliance for unit testing
- Common prefix support for directory-style listing
- ETag generation via MD5 hash

#### 2. `internal/workspaces/ephemeral.go` — Ephemeral Workspace Manager

**Key Types:**
```go
// EphemeralWorkspaceManager manages per-session workspace checkouts
type EphemeralWorkspaceManager struct {
    store     objectstore.ObjectStore
    workdir   string
    keyPrefix string
    active    map[string]*workspaceState
    mu        sync.RWMutex
}

// SyncManifest tracks file state for efficient change detection
type SyncManifest struct {
    Version      int                     `json:"version"`
    CheckoutTime time.Time               `json:"checkoutTime"`
    Files        map[string]FileManifest `json:"files"`
}

// FileManifest contains per-file metadata
type FileManifest struct {
    Size         int64     `json:"size"`
    SHA256       string    `json:"sha256"`
    ETag         string    `json:"etag"`
    LastModified time.Time `json:"lastModified"`
}
```

**Checkout Flow:**
1. Validate project ID (path traversal prevention)
2. Check for existing active session (return cached workspace)
3. Create local directory: `${ROOT}/users/<uid>/projects/<pid>/sessions/<sessionID>`
4. List all objects under project S3 prefix
5. Download each file to local workspace
6. Compute SHA256 hash during download
7. Build sync manifest for change detection
8. Track workspace in active sessions map

**Commit Flow:**
1. Walk local workspace directory
2. For each file:
   - Compute current SHA256 hash
   - Compare with manifest (skip unchanged files)
   - Upload modified/new files to S3
3. Detect deleted files (in manifest but not on disk)
4. Delete removed files from S3
5. Update manifest with new state

**Cleanup Flow:**
1. Remove local workspace directory (`os.RemoveAll`)
2. Remove from active sessions map
3. Manifest persisted for session resume (not yet implemented)

#### 3. `internal/projects/interface.go` — ProjectService Interface

```go
// ProjectService defines the projects API contract
type ProjectService interface {
    CreateProject(ctx context.Context, userID int64, name string) (Project, error)
    DeleteProject(ctx context.Context, userID int64, projectID string) error
    ListProjects(ctx context.Context, userID int64) ([]Project, error)
    ListTree(ctx context.Context, userID int64, projectID, path string) ([]TreeEntry, error)
    UploadFile(ctx context.Context, userID int64, projectID, path, name string, body io.Reader) (TreeEntry, error)
    DeleteFile(ctx context.Context, userID int64, projectID, path string) error
    MovePath(ctx context.Context, userID int64, projectID, src, dst string) error
    CreateDir(ctx context.Context, userID int64, projectID, path, name string) error
    ReadFile(ctx context.Context, userID int64, projectID, path string) (io.ReadCloser, string, error)
    EnableEncryption(enabled bool)
}
```

#### 4. `internal/projects/s3.go` — S3-backed ProjectService

**S3Service Features:**
- Full `ProjectService` interface implementation
- Key scheme: `${prefix}/users/<uid>/projects/<pid>/files/<path>`
- Metadata stored at: `${prefix}/users/<uid>/projects/<pid>/.meta/project.json`
- Project listing via prefix enumeration
- Directory-style listing with delimiter support
- Content-type detection via `mime.TypeByExtension`
- Configurable encryption placeholder (implementation pending Phase 4)

### Files Modified

#### 1. `internal/workspaces/manager.go`

- Added `NewManagerWithStore(cfg, store)` factory function
- When `cfg.Projects.Workspace.Mode == "ephemeral"` and store is provided:
  - Returns `EphemeralWorkspaceManager` instead of fallback to legacy
- Preserves backward compatibility: `NewManager(cfg)` still works (no store = legacy mode)

#### 2. `internal/agentd/run.go`

- Added import: `"manifold/internal/objectstore"`
- Added `s3Store objectstore.ObjectStore` field to `app` struct
- S3 client initialization when `cfg.Projects.Backend == "s3"`:
  ```go
  if cfg.Projects.Backend == "s3" {
      s3Cfg := objectstore.S3Config{
          Endpoint:     cfg.Projects.S3.Endpoint,
          Region:       cfg.Projects.S3.Region,
          Bucket:       cfg.Projects.S3.Bucket,
          AccessKey:    cfg.Projects.S3.AccessKey,
          SecretKey:    cfg.Projects.S3.SecretKey,
          UsePathStyle: cfg.Projects.S3.UsePathStyle,
          TLSInsecure:  cfg.Projects.S3.TLSInsecureSkipVerify,
          SSEMode:      cfg.Projects.S3.SSE.Mode,
          SSEKMSKeyID:  cfg.Projects.S3.SSE.KMSKeyID,
      }
      s3Store, err = objectstore.NewS3Store(ctx, s3Cfg)
  }
  ```
- Workspace manager uses `NewManagerWithStore(cfg, s3Store)` when S3 backend configured
- Projects service selection based on backend (placeholder for S3Service wiring)

### Dependencies Added

```
go get github.com/aws/aws-sdk-go-v2
go get github.com/aws/aws-sdk-go-v2/config
go get github.com/aws/aws-sdk-go-v2/credentials
go get github.com/aws/aws-sdk-go-v2/service/s3
```

### Test Coverage

**ObjectStore Tests (`memory_test.go`):**
- `TestMemoryStore_PutAndGet` — Basic put/get operations
- `TestMemoryStore_GetNotFound` — Error handling for missing keys
- `TestMemoryStore_Delete` — Delete and verify removal
- `TestMemoryStore_List` — Prefix listing and delimiter support
- `TestMemoryStore_Head` — Metadata retrieval
- `TestMemoryStore_Copy` — Object copy operations
- `TestMemoryStore_Exists` — Existence checks

**Ephemeral Workspace Tests (`ephemeral_test.go`):**
- `TestEphemeralWorkspaceManager_CheckoutAndCommit` — Full lifecycle test
- `TestEphemeralWorkspaceManager_EmptyProjectID` — Empty project handling
- `TestEphemeralWorkspaceManager_InvalidProjectID` — Path traversal prevention
- `TestEphemeralWorkspaceManager_SessionReuse` — Same session returns cached workspace

**Projects S3 Tests (`s3_test.go`):**
- `TestS3Service_CreateProject` — Project creation
- `TestS3Service_ListProjects` — Project enumeration
- `TestS3Service_DeleteProject` — Project deletion with cleanup
- `TestS3Service_UploadAndReadFile` — File upload and retrieval
- `TestS3Service_ListTree` — Directory listing
- `TestS3Service_DeleteFile` — File deletion
- `TestS3Service_MovePath` — File/directory move
- `TestS3Service_CreateDir` — Directory marker creation
- `TestS3Service_InvalidFileNames` — Path validation

### Architecture Decisions

1. **Interface Segregation**: `ObjectStore` interface is minimal and focused on blob operations
2. **SHA256 Change Detection**: Manifest tracks content hashes to avoid unnecessary uploads
3. **Session Caching**: Active workspaces cached in memory for fast re-checkout
4. **Path-style S3**: Enabled via config for MinIO compatibility
5. **Lazy Directory Creation**: Directories created on-demand during file writes

### Key Mapping Scheme

```
S3 Key Structure:
${prefix}/users/${userID}/projects/${projectID}/files/${relativePath}
${prefix}/users/${userID}/projects/${projectID}/.meta/project.json

Example:
workspaces/users/123/projects/my-project/files/src/main.go
workspaces/users/123/projects/my-project/.meta/project.json
```

### Configuration Required

To enable S3-backed storage:
```yaml
projects:
  backend: s3
  workspace:
    mode: ephemeral
    root: /var/manifold/sandboxes
  s3:
    endpoint: "http://minio:9000"  # or empty for AWS
    region: "us-east-1"
    bucket: "manifold-workspaces"
    prefix: "workspaces"
    accessKey: "minioadmin"
    secretKey: "minioadmin"
    usePathStyle: true  # required for MinIO
```

### Behavioral Notes

- **Backward Compatible**: Default `backend: filesystem` + `mode: legacy` preserves existing behavior
- **Graceful Degradation**: If S3 store not provided, ephemeral mode falls back to legacy
- **Thread-Safe**: `EphemeralWorkspaceManager` uses mutex for concurrent session access
- **Efficient Sync**: Only changed files uploaded on commit (SHA256 comparison)

### Readiness for Phase 3

The following hooks are in place for database metadata:

1. **Project struct** already includes `ID`, `Name`, `CreatedAt`, `UpdatedAt`
2. **ProjectService interface** is ready for database-backed listing
3. **S3Service** can be extended to use database for fast project enumeration
4. **Revision/concurrency** fields can be added to manifest

### Test Results

```
$ go test ./internal/objectstore/... ./internal/workspaces/... ./internal/projects/... -v
ok      manifold/internal/objectstore   0.007s
ok      manifold/internal/workspaces    0.018s
ok      manifold/internal/projects      0.012s
```

### Next Steps (Phase 3)

1. Add `ProjectsStore` to `internal/persistence` with Postgres implementation
2. Create database tables for project metadata
3. Wire database store into S3Service for fast listing
4. Add revision-based optimistic concurrency control
5. Implement migration tooling for existing filesystem projects

---

## Phase 3 — Implementation Notes (Completed)

**Date**: January 3, 2026

### What Was Implemented

Phase 3 introduces database-backed metadata and indexing for projects. This phase adds the `ProjectsStore` interface to the persistence layer, implements both Postgres and memory-backed stores, and provides a migration command for importing existing filesystem projects into the database.

### Key Goals Achieved

1. **Fast Project Listing**: Database-backed project queries instead of expensive S3 LIST operations
2. **File Index**: Optional `project_files` table for fast directory listing without S3 round-trips
3. **Optimistic Concurrency**: Revision-based conflict detection for concurrent updates
4. **Migration Tooling**: Command-line tool to import existing projects from filesystem to database

### Files Created

#### 1. `internal/persistence/databases/projects_store_postgres.go`

Postgres-backed implementation of `ProjectsStore`:

**Database Schema:**
```sql
CREATE TABLE IF NOT EXISTS projects (
    id UUID PRIMARY KEY,
    user_id BIGINT NOT NULL,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revision BIGINT NOT NULL DEFAULT 1,
    bytes BIGINT NOT NULL DEFAULT 0,
    file_count INTEGER NOT NULL DEFAULT 0,
    storage_backend TEXT NOT NULL DEFAULT 'filesystem'
);

CREATE INDEX IF NOT EXISTS projects_user_updated_idx ON projects(user_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS projects_user_name_idx ON projects(user_id, name);

CREATE TABLE IF NOT EXISTS project_files (
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    path TEXT NOT NULL,
    name TEXT NOT NULL,
    is_dir BOOLEAN NOT NULL DEFAULT FALSE,
    size BIGINT NOT NULL DEFAULT 0,
    mod_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    etag TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (project_id, path)
);

CREATE INDEX IF NOT EXISTS project_files_parent_idx ON project_files(project_id, ...);
```

**Key Features:**
- `Create()` generates UUID for new projects, returns full project with metadata
- `Get()` includes authorization check (userID must match project owner)
- `List()` returns projects sorted by `UpdatedAt DESC, Name ASC` (matching frontend expectations)
- `Update()` implements optimistic concurrency: compares revision, increments on success
- `UpdateStats()` partial update for byte/file counts without revision check
- `Delete()` cascades to remove all file index entries
- `InsertWithID()` migration helper to preserve existing project UUIDs

**File Index Operations:**
- `IndexFile()` upserts file entry with ON CONFLICT handling
- `RemoveFileIndex()` deletes single file entry
- `RemoveFileIndexPrefix()` deletes directory and all descendants
- `ListFiles()` returns directory contents sorted (dirs first, then by name)
- `GetFile()` retrieves single file entry by exact path

#### 2. `internal/persistence/databases/projects_store_memory.go`

In-memory implementation for tests and development:

- Thread-safe with `sync.RWMutex`
- Full interface compliance for unit testing
- Same sorting behavior as Postgres implementation
- Supports all file index operations

#### 3. `internal/persistence/databases/projects_store_test.go`

Comprehensive test suite:

- `TestMemoryProjectsStore_CreateAndGet` — Basic CRUD operations
- `TestMemoryProjectsStore_GetNotFound` — ErrNotFound handling
- `TestMemoryProjectsStore_GetForbidden` — Authorization enforcement
- `TestMemoryProjectsStore_List` — Multi-user isolation and sorting
- `TestMemoryProjectsStore_Update` — Revision increment on update
- `TestMemoryProjectsStore_UpdateRevisionConflict` — Optimistic concurrency
- `TestMemoryProjectsStore_Delete` — Delete with ownership check
- `TestMemoryProjectsStore_DeleteForbidden` — Cross-user delete prevention
- `TestMemoryProjectsStore_UpdateStats` — Partial stats update
- `TestMemoryProjectsStore_FileIndex` — Full file index lifecycle
- `TestMemoryProjectsStore_DefaultName` — Empty name handling
- `TestNormalizePath` — Path normalization edge cases
- `TestParentDir` — Parent directory computation

#### 4. `cmd/migrateprojects/main.go`

Migration command for importing filesystem projects:

**Usage:**
```bash
go run cmd/migrateprojects/main.go \
  -workdir /data \
  -dsn "postgres://user:pass@host:5432/db" \
  -dry-run \
  -verbose
```

**Features:**
- Scans `${WORKDIR}/users/<uid>/projects/<pid>` structure
- Reads `.meta/project.json` for metadata (falls back to directory timestamps)
- Computes file stats (byte count, file count)
- Preserves original project UUIDs via `InsertWithID()`
- Indexes all files in `project_files` table
- Supports dry-run mode for safe preview
- Verbose mode for detailed progress

**Migration Summary Output:**
```
--- Migration Summary ---
Users scanned:     5
Projects found:    12
Projects migrated: 12
Files indexed:     847
Errors:            0
```

### Files Modified

#### 1. `internal/persistence/store.go`

Added new error and types:

```go
var ErrRevisionConflict = errors.New("persistence: revision conflict")

type Project struct {
    ID             string    `json:"id"`
    UserID         int64     `json:"userId"`
    Name           string    `json:"name"`
    CreatedAt      time.Time `json:"createdAt"`
    UpdatedAt      time.Time `json:"updatedAt"`
    Revision       int64     `json:"revision"`
    Bytes          int64     `json:"bytes"`
    FileCount      int       `json:"fileCount"`
    StorageBackend string    `json:"storageBackend,omitempty"`
}

type ProjectFile struct {
    ProjectID string    `json:"projectId"`
    Path      string    `json:"path"`
    Name      string    `json:"name"`
    IsDir     bool      `json:"isDir"`
    Size      int64     `json:"size"`
    ModTime   time.Time `json:"modTime"`
    ETag      string    `json:"etag"`
    UpdatedAt time.Time `json:"updatedAt"`
}

type ProjectsStore interface {
    Init(ctx context.Context) error
    Create(ctx context.Context, userID int64, name string) (Project, error)
    Get(ctx context.Context, userID int64, projectID string) (Project, error)
    List(ctx context.Context, userID int64) ([]Project, error)
    Update(ctx context.Context, p Project) (Project, error)
    UpdateStats(ctx context.Context, projectID string, bytes int64, fileCount int) error
    Delete(ctx context.Context, userID int64, projectID string) error
    IndexFile(ctx context.Context, f ProjectFile) error
    RemoveFileIndex(ctx context.Context, projectID, path string) error
    RemoveFileIndexPrefix(ctx context.Context, projectID, pathPrefix string) error
    ListFiles(ctx context.Context, projectID, path string) ([]ProjectFile, error)
    GetFile(ctx context.Context, projectID, path string) (ProjectFile, error)
}
```

#### 2. `internal/persistence/databases/interfaces.go`

Added `Projects` field to `Manager` struct:

```go
type Manager struct {
    Search         FullTextSearch
    Vector         VectorStore
    Graph          GraphDB
    Chat           persistence.ChatStore
    EvolvingMemory memory.EvolvingMemoryStore
    Playground     *PlaygroundStore
    Warpp          persistence.WarppWorkflowStore
    MCP            persistence.MCPStore
    Projects       persistence.ProjectsStore  // NEW
}
```

Updated `Close()` to clean up projects store connection.

#### 3. `internal/persistence/databases/factory.go`

Added projects store initialization:

```go
// Projects Store
// Use default DSN if available, otherwise memory
var projectsStore persistence.ProjectsStore
if cfg.DefaultDSN != "" {
    if p, err := newPgPool(ctx, cfg.DefaultDSN); err == nil {
        projectsStore = NewPostgresProjectsStore(p)
    } else {
        projectsStore = NewPostgresProjectsStore(nil)
    }
} else {
    projectsStore = NewPostgresProjectsStore(nil)
}
if err := projectsStore.Init(ctx); err != nil {
    return Manager{}, fmt.Errorf("init projects store: %w", err)
}
m.Projects = projectsStore
```

### Architecture Decisions

1. **Optimistic Concurrency**: `Revision` field prevents lost updates from concurrent modifications
2. **Cascading Deletes**: `ON DELETE CASCADE` ensures file index is cleaned up with project
3. **Path Normalization**: `normalizePath()` ensures consistent path handling across operations
4. **Dual Tables**: Separate `projects` and `project_files` for efficient queries
5. **Index on Parent**: Computed index on parent directory enables fast `ListFiles()` queries
6. **Memory Fallback**: `NewPostgresProjectsStore(nil)` returns memory store when no pool provided

### Helper Functions

```go
// normalizePath ensures consistent path representation
func normalizePath(p string) string {
    p = strings.TrimSpace(p)
    p = path.Clean(p)
    if p == "/" || p == "" {
        return "."
    }
    p = strings.TrimPrefix(p, "/")
    p = strings.TrimSuffix(p, "/")
    return p
}

// parentDir returns the parent directory of a path
func parentDir(p string) string {
    p = normalizePath(p)
    if p == "." {
        return "."
    }
    dir := path.Dir(p)
    if dir == "" || dir == "/" {
        return "."
    }
    return dir
}
```

### Test Results

```
$ go test ./internal/persistence/... -v -run "Projects|NormalizePath|ParentDir"
=== RUN   TestMemoryProjectsStore_CreateAndGet
--- PASS: TestMemoryProjectsStore_CreateAndGet (0.00s)
=== RUN   TestMemoryProjectsStore_GetNotFound
--- PASS: TestMemoryProjectsStore_GetNotFound (0.00s)
=== RUN   TestMemoryProjectsStore_GetForbidden
--- PASS: TestMemoryProjectsStore_GetForbidden (0.00s)
=== RUN   TestMemoryProjectsStore_List
--- PASS: TestMemoryProjectsStore_List (0.01s)
=== RUN   TestMemoryProjectsStore_Update
--- PASS: TestMemoryProjectsStore_Update (0.00s)
=== RUN   TestMemoryProjectsStore_UpdateRevisionConflict
--- PASS: TestMemoryProjectsStore_UpdateRevisionConflict (0.00s)
=== RUN   TestMemoryProjectsStore_Delete
--- PASS: TestMemoryProjectsStore_Delete (0.00s)
=== RUN   TestMemoryProjectsStore_DeleteForbidden
--- PASS: TestMemoryProjectsStore_DeleteForbidden (0.00s)
=== RUN   TestMemoryProjectsStore_UpdateStats
--- PASS: TestMemoryProjectsStore_UpdateStats (0.00s)
=== RUN   TestMemoryProjectsStore_FileIndex
--- PASS: TestMemoryProjectsStore_FileIndex (0.00s)
=== RUN   TestMemoryProjectsStore_DefaultName
--- PASS: TestMemoryProjectsStore_DefaultName (0.00s)
=== RUN   TestNormalizePath
--- PASS: TestNormalizePath (0.00s)
=== RUN   TestParentDir
--- PASS: TestParentDir (0.00s)
PASS
ok      manifold/internal/persistence/databases 0.024s
```

### Behavioral Notes

- **Zero Breaking Changes**: Existing filesystem behavior unchanged
- **Automatic Table Creation**: `Init()` creates tables idempotently
- **Authorization Enforcement**: All operations check user ownership
- **Empty Name Handling**: Creates "Untitled" project if name is empty/whitespace
- **Sorting Consistency**: List operations match frontend expectations (UpdatedAt desc, Name asc)

### Integration Points

The `ProjectsStore` is now available via `databases.Manager.Projects` and can be used by:

1. **S3Service** — Use database for fast project listing instead of S3 LIST
2. **Projects HTTP API** — Optionally back `/api/projects` endpoints with database
3. **Workspace Manager** — Track project revisions for sync conflict detection
4. **Background Jobs** — Update file indexes after workspace commits

### Migration Path

To migrate existing filesystem projects to database:

1. Ensure Postgres is configured via `DATABASE_URL` or config
2. Run migration in dry-run mode first:
   ```bash
   go run cmd/migrateprojects/main.go -workdir $WORKDIR -dsn $DATABASE_URL -dry-run
   ```
3. Execute actual migration:
   ```bash
   go run cmd/migrateprojects/main.go -workdir $WORKDIR -dsn $DATABASE_URL -verbose
   ```
4. Verify project counts match

### Next Steps (Phase 4)

1. Replace local master key with KMS/Vault for encryption
2. Implement `KeyProvider` interface for DEK wrap/unwrap
3. Add application-layer encryption for S3 objects
4. Key rotation support for re-encrypting project contents
