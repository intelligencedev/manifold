# Manifold Workspace Architecture Implementation Plan

## Overview

This document describes the implementation plan for Manifold's workspace management system, designed to support both simple single-node deployments and enterprise multi-tenant deployments at scale.

### Goals

1. **Backward Compatibility**: Simple deployments continue to work with only Postgres + local WORKDIR (no S3, no encryption, no auth).
2. **Enterprise Scale**: Support millions of users with multi-tenant isolation, encryption-at-rest, and minimal latency.
3. **Security**: Ciphertext-only disk cache with KMS/Vault key controls; admins cannot read tenant data on disk.
4. **Performance**: Minimize S3 calls; sub-second chat turn latency; fast agent workflows.
5. **Tool Compatibility**: Agents perceive a standard filesystem for running tools (grep, compilers, linters, etc.).

---

## Deployment Modes

### Mode 1: Simple (Current Default)

```
┌─────────────┐     ┌──────────────┐
│   agentd    │────▶│   Postgres   │
└─────────────┘     └──────────────┘
       │
       ▼
┌─────────────────────┐
│  Local WORKDIR      │
│  (plaintext files)  │
└─────────────────────┘
```

- No S3, no Redis, no Kafka
- Projects stored as plaintext in `WORKDIR/users/{userID}/projects/{projectID}/`
- No encryption
- Single-node only
- **No changes required** to current behavior

### Mode 2: Enterprise

```
┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│   agentd    │────▶│   Postgres   │     │  KMS/Vault  │
│  (N nodes)  │     └──────────────┘     └─────────────┘
└─────────────┘            │                    │
       │                   │                    │
       ▼                   ▼                    ▼
┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│    Redis    │     │     S3       │     │    Kafka    │
│ (coord/cache)│     │  (durable)   │     │  (events)   │
└─────────────┘     └──────────────┘     └─────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────┐
│              Local Workspace Cache                   │
│  ┌─────────────────┐    ┌─────────────────────────┐ │
│  │ Encrypted Cache │    │  tmpfs Working Set      │ │
│  │  (ciphertext)   │───▶│  (plaintext, ephemeral) │ │
│  └─────────────────┘    └─────────────────────────┘ │
└─────────────────────────────────────────────────────┘
```

- S3 as durable source of truth (ciphertext)
- Redis for coordination, generation cache, invalidation pub/sub, locks
- Kafka for durable change events, async commit pipeline, audit trail
- Local encrypted cache + tmpfs plaintext working set
- Multi-node, multi-tenant

---

## Core Concepts

### Project Generation

A monotonically increasing version number per project, stored in:
- **S3**: `.meta/project.json` (field: `generation`)
- **Redis**: `project:{tenantID}:{projectID}:generation` (hot cache)
- **Postgres**: `projects.generation` column (source of truth for coordination)

Updated on every successful commit. Used for cheap staleness detection.

### Skills Generation

Separate generation for the `.manifold/skills/` subtree:
- Allows refreshing skills without full workspace sync
- Stored alongside project generation

### Workspace Lifecycle

```
┌─────────────────────────────────────────────────────────────────┐
│                      Workspace States                            │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  [Missing] ──checkout──▶ [Cached] ──materialize──▶ [Active]     │
│                              │                         │         │
│                              │◀───────unmaterialize────┘         │
│                              │                                   │
│                              │◀───────checkin (if dirty)         │
│                              │                                   │
│                         [Evicted] ◀──────TTL/LRU────────         │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

- **Missing**: No local cache exists
- **Cached**: Encrypted files on local disk; not currently in use
- **Active**: Materialized into tmpfs for tool execution
- **Evicted**: Cleaned up due to TTL or memory pressure

---

## Component Design

### 1. WorkspaceManager Interface (Unchanged API)

```go
type WorkspaceManager interface {
    // Checkout ensures workspace is available locally and returns handle
    Checkout(ctx context.Context, userID int64, projectID, sessionID string) (Workspace, error)
    
    // Commit persists local changes to durable storage
    Commit(ctx context.Context, ws Workspace) error
    
    // Cleanup releases workspace resources
    Cleanup(ctx context.Context, ws Workspace) error
    
    // Mode returns the manager mode ("legacy", "ephemeral", "enterprise")
    Mode() string
}
```

### 2. New: EnterpriseWorkspaceManager

Implements `WorkspaceManager` with:
- Encrypted local cache
- tmpfs materialization
- Generation-based staleness
- Event-driven invalidation

```go
type EnterpriseWorkspaceManager struct {
    // Storage backends
    store        objectstore.ObjectStore  // S3
    redis        *redis.Client            // Coordination
    kafka        *kafka.Producer          // Events
    
    // Encryption
    keyProvider  KeyProvider              // KMS/Vault
    
    // Local cache config
    cacheDir     string                   // Encrypted cache root
    tmpfsDir     string                   // tmpfs mount for plaintext
    
    // State tracking
    mu           sync.RWMutex
    sessions     map[string]*SessionState
    
    // Background workers
    evictionTicker *time.Ticker
    kafkaConsumer  *kafka.Consumer
}

type SessionState struct {
    Workspace       Workspace
    LocalGeneration int64           // Last known generation
    Dirty           bool            // Has uncommitted changes
    MaterializedAt  time.Time       // When tmpfs was populated
    DirtyPaths      map[string]bool // Changed files
}
```

### 3. Local Cache Structure

```
{cacheDir}/
├── tenants/
│   └── {tenantID}/
│       └── projects/
│           └── {projectID}/
│               ├── .meta/
│               │   ├── cache-manifest.json   # Local cache state
│               │   └── wrapped-dek.bin       # Wrapped DEK for this cache
│               └── files/
│                   └── ... (encrypted files, mirrors S3 structure)

{tmpfsDir}/
├── sessions/
│   └── {sessionID}/
│       └── workspace/
│           └── ... (plaintext files for active session)
```

### 4. Generation Tracking (Redis)

```
Keys:
  project:{tenantID}:{projectID}:generation     -> int64
  project:{tenantID}:{projectID}:skills_gen     -> int64
  project:{tenantID}:{projectID}:lock           -> session_id (with TTL)

Pub/Sub Channels:
  project:{tenantID}:{projectID}:invalidate     -> {generation, skills_gen, changed_paths}
```

### 5. Kafka Events

```
Topic: manifold.project.commits

Event Schema:
{
  "tenant_id": "string",
  "project_id": "string",
  "user_id": int64,
  "session_id": "string",
  "generation": int64,
  "skills_generation": int64,
  "changed_paths": ["string"],
  "timestamp": "ISO8601",
  "commit_id": "uuid"
}
```

---

## Implementation Phases

### Phase 1: Foundation (Weeks 1-2)

**Goal**: Introduce generation tracking and session-stable workspaces without breaking existing behavior.

#### 1.1 Add Generation to Project Metadata

```go
// internal/projects/model.go
type Project struct {
    ID              string    `json:"id"`
    Name            string    `json:"name"`
    Generation      int64     `json:"generation"`       // NEW
    SkillsGeneration int64    `json:"skills_generation"` // NEW
    CreatedAt       time.Time `json:"created_at"`
    UpdatedAt       time.Time `json:"updated_at"`
}
```

- Update `S3Service` and `Service` to increment generation on file writes
- Add `projects.generation` column to Postgres schema
- Backward compatible: default generation=0 for existing projects

#### 1.2 Session-Stable Workspace Reuse

Modify `EphemeralWorkspaceManager.Checkout`:
- If session already has a workspace and generation matches, return existing (no S3 calls)
- Only hydrate on first checkout or when stale

```go
func (m *EphemeralWorkspaceManager) Checkout(ctx context.Context, userID int64, projectID, sessionID string) (Workspace, error) {
    key := sessionKey(userID, projectID, sessionID)
    
    m.mu.RLock()
    existing := m.active[key]
    m.mu.RUnlock()
    
    if existing != nil {
        // Check if still fresh
        remoteGen, err := m.getRemoteGeneration(ctx, userID, projectID)
        if err == nil && existing.generation >= remoteGen {
            return existing.ws, nil  // Reuse - no S3 calls!
        }
    }
    
    // Full checkout or incremental refresh...
}
```

#### 1.3 Dirty Tracking and Conditional Commit

```go
func (m *EphemeralWorkspaceManager) MarkDirty(ws Workspace, paths []string) {
    // Track which files changed
}

func (m *EphemeralWorkspaceManager) Commit(ctx context.Context, ws Workspace) error {
    state := m.getState(ws)
    if !state.Dirty {
        return nil  // Nothing to do
    }
    
    // Upload only changed files
    for path := range state.DirtyPaths {
        // ...
    }
    
    // Increment generation
    // Clear dirty state
}
```

#### 1.4 Separate Skills Loading

Create `internal/skills/cache.go`:

```go
type SkillsCache struct {
    mu    sync.RWMutex
    cache map[string]*CachedSkills  // key: "{projectID}:{skillsGen}"
}

type CachedSkills struct {
    Generation   int64
    Skills       []Skill
    RenderedPrompt string
    CachedAt     time.Time
}

func (c *SkillsCache) GetOrLoad(ctx context.Context, projectID string, skillsGen int64, loader func() ([]Skill, error)) (*CachedSkills, error) {
    key := fmt.Sprintf("%s:%d", projectID, skillsGen)
    
    c.mu.RLock()
    if cached, ok := c.cache[key]; ok {
        c.mu.RUnlock()
        return cached, nil
    }
    c.mu.RUnlock()
    
    // Load and cache
    skills, err := loader()
    // ...
}
```

**Deliverables**:
- [x] Generation field in project metadata
- [x] Session-stable workspace reuse
- [x] Dirty tracking + conditional commit
- [x] Skills cache separate from workspace

---

### Phase 2: Redis Integration (Weeks 3-4)

**Goal**: Add Redis for fast generation checks and cross-node invalidation.

#### 2.1 Redis Client Configuration

```go
// internal/config/config.go
type RedisConfig struct {
    Enabled  bool   `yaml:"enabled"`
    Addr     string `yaml:"addr"`
    Password string `yaml:"password"`
    DB       int    `yaml:"db"`
}
```

#### 2.2 Generation Cache in Redis

```go
// internal/workspaces/redis_cache.go
type RedisGenerationCache struct {
    client *redis.Client
}

func (c *RedisGenerationCache) GetGeneration(ctx context.Context, tenantID, projectID string) (int64, error) {
    key := fmt.Sprintf("project:%s:%s:generation", tenantID, projectID)
    return c.client.Get(ctx, key).Int64()
}

func (c *RedisGenerationCache) SetGeneration(ctx context.Context, tenantID, projectID string, gen int64) error {
    key := fmt.Sprintf("project:%s:%s:generation", tenantID, projectID)
    return c.client.Set(ctx, key, gen, 0).Err()
}
```

#### 2.3 Pub/Sub Invalidation

```go
func (c *RedisGenerationCache) PublishInvalidation(ctx context.Context, tenantID, projectID string, event InvalidationEvent) error {
    channel := fmt.Sprintf("project:%s:%s:invalidate", tenantID, projectID)
    data, _ := json.Marshal(event)
    return c.client.Publish(ctx, channel, data).Err()
}

func (c *RedisGenerationCache) SubscribeInvalidations(ctx context.Context, tenantID, projectID string) <-chan InvalidationEvent {
    // Returns channel that receives invalidation events
}
```

#### 2.4 Distributed Locks for Commits

```go
func (c *RedisGenerationCache) AcquireCommitLock(ctx context.Context, tenantID, projectID, sessionID string, ttl time.Duration) (bool, error) {
    key := fmt.Sprintf("project:%s:%s:lock", tenantID, projectID)
    return c.client.SetNX(ctx, key, sessionID, ttl).Result()
}
```

**Deliverables**:
- [x] Redis client integration (optional, disabled by default)
- [x] Generation cache with Redis backend
- [x] Pub/Sub invalidation
- [x] Distributed commit locks

---

### Phase 3: Encrypted Local Cache (Weeks 5-7)

**Goal**: Store only ciphertext on local disk; decrypt to tmpfs for tool execution.

#### 3.1 Encrypted Cache Manager

```go
// internal/workspaces/encrypted_cache.go
type EncryptedCacheManager struct {
    cacheDir    string
    keyProvider KeyProvider
    
    // DEK cache (in-memory only, never persisted)
    dekCache    sync.Map  // projectID -> *cachedDEK
}

type cachedDEK struct {
    dek       []byte
    expiresAt time.Time
}

func (m *EncryptedCacheManager) WriteFile(ctx context.Context, tenantID, projectID, path string, plaintext []byte) error {
    dek, err := m.getDEK(ctx, tenantID, projectID)
    if err != nil {
        return err
    }
    
    ciphertext, err := encrypt(dek, plaintext)
    if err != nil {
        return err
    }
    
    cachePath := m.cachePath(tenantID, projectID, path)
    return os.WriteFile(cachePath, ciphertext, 0600)
}

func (m *EncryptedCacheManager) ReadFile(ctx context.Context, tenantID, projectID, path string) ([]byte, error) {
    cachePath := m.cachePath(tenantID, projectID, path)
    ciphertext, err := os.ReadFile(cachePath)
    if err != nil {
        return nil, err
    }
    
    dek, err := m.getDEK(ctx, tenantID, projectID)
    if err != nil {
        return err
    }
    
    return decrypt(dek, ciphertext)
}
```

#### 3.2 tmpfs Materialization

```go
// internal/workspaces/tmpfs_materializer.go
type TmpfsMaterializer struct {
    tmpfsRoot string  // e.g., /dev/shm/manifold or mounted tmpfs
    cache     *EncryptedCacheManager
}

func (m *TmpfsMaterializer) Materialize(ctx context.Context, tenantID, projectID, sessionID string) (string, error) {
    workspacePath := filepath.Join(m.tmpfsRoot, "sessions", sessionID, "workspace")
    
    if err := os.MkdirAll(workspacePath, 0700); err != nil {
        return "", err
    }
    
    // Decrypt all files from cache to tmpfs
    err := m.cache.WalkFiles(ctx, tenantID, projectID, func(relPath string) error {
        plaintext, err := m.cache.ReadFile(ctx, tenantID, projectID, relPath)
        if err != nil {
            return err
        }
        
        destPath := filepath.Join(workspacePath, relPath)
        os.MkdirAll(filepath.Dir(destPath), 0700)
        return os.WriteFile(destPath, plaintext, 0600)
    })
    
    return workspacePath, err
}

func (m *TmpfsMaterializer) Unmaterialize(ctx context.Context, sessionID string) error {
    workspacePath := filepath.Join(m.tmpfsRoot, "sessions", sessionID)
    return os.RemoveAll(workspacePath)  // Secure: tmpfs is RAM, no residue on disk
}
```

#### 3.3 Sync Changed Files Back

```go
func (m *TmpfsMaterializer) SyncBack(ctx context.Context, tenantID, projectID, sessionID string, changedPaths []string) error {
    workspacePath := filepath.Join(m.tmpfsRoot, "sessions", sessionID, "workspace")
    
    for _, relPath := range changedPaths {
        srcPath := filepath.Join(workspacePath, relPath)
        plaintext, err := os.ReadFile(srcPath)
        if err != nil {
            if os.IsNotExist(err) {
                // File deleted - handle accordingly
                continue
            }
            return err
        }
        
        // Write encrypted to cache
        if err := m.cache.WriteFile(ctx, tenantID, projectID, relPath, plaintext); err != nil {
            return err
        }
    }
    
    return nil
}
```

#### 3.4 EnterpriseWorkspaceManager Integration

```go
type EnterpriseWorkspaceManager struct {
    cache        *EncryptedCacheManager
    materializer *TmpfsMaterializer
    // ...
}

func (m *EnterpriseWorkspaceManager) Checkout(ctx context.Context, userID int64, projectID, sessionID string) (Workspace, error) {
    tenantID := m.getTenantID(ctx, userID)
    
    // 1. Check generation (Redis first, then S3 metadata)
    localGen := m.getLocalGeneration(tenantID, projectID)
    remoteGen, _ := m.getRemoteGeneration(ctx, tenantID, projectID)
    
    // 2. Sync encrypted cache if stale
    if localGen < remoteGen {
        if err := m.syncCache(ctx, tenantID, projectID); err != nil {
            return Workspace{}, err
        }
    }
    
    // 3. Materialize to tmpfs
    workspacePath, err := m.materializer.Materialize(ctx, tenantID, projectID, sessionID)
    if err != nil {
        return Workspace{}, err
    }
    
    return Workspace{
        BaseDir:   workspacePath,
        SessionID: sessionID,
        // ...
    }, nil
}
```

**Deliverables**:
- [x] Encrypted cache manager with in-memory DEK cache
- [x] tmpfs materializer
- [x] Sync-back for changed files
- [x] EnterpriseWorkspaceManager integration (materialize/syncback)

---

### Phase 4: Kafka Integration (Weeks 8-9)

**Goal**: Durable event stream for commit events, async commit pipeline, audit trail.

#### 4.1 Kafka Producer for Commit Events

```go
// internal/workspaces/kafka_events.go
type KafkaEventProducer struct {
    producer *kafka.Writer
    topic    string
}

type CommitEvent struct {
    TenantID        string    `json:"tenant_id"`
    ProjectID       string    `json:"project_id"`
    UserID          int64     `json:"user_id"`
    SessionID       string    `json:"session_id"`
    Generation      int64     `json:"generation"`
    SkillsGeneration int64    `json:"skills_generation"`
    ChangedPaths    []string  `json:"changed_paths"`
    Timestamp       time.Time `json:"timestamp"`
    CommitID        string    `json:"commit_id"`
}

func (p *KafkaEventProducer) PublishCommit(ctx context.Context, event CommitEvent) error {
    data, _ := json.Marshal(event)
    return p.producer.WriteMessages(ctx, kafka.Message{
        Key:   []byte(fmt.Sprintf("%s:%s", event.TenantID, event.ProjectID)),
        Value: data,
    })
}
```

#### 4.2 Async Commit Worker (Optional)

For high-write workloads, commits can be async:

```go
// internal/workspaces/commit_worker.go
type CommitWorker struct {
    queue  chan CommitJob
    store  objectstore.ObjectStore
    redis  *RedisGenerationCache
    kafka  *KafkaEventProducer
}

type CommitJob struct {
    TenantID    string
    ProjectID   string
    SessionID   string
    ChangedFiles map[string][]byte  // path -> ciphertext
    BaseGeneration int64
}

func (w *CommitWorker) Run(ctx context.Context) {
    for job := range w.queue {
        w.processCommit(ctx, job)
    }
}

func (w *CommitWorker) processCommit(ctx context.Context, job CommitJob) {
    // 1. Acquire lock
    // 2. Check generation (optimistic concurrency)
    // 3. Upload changed files to S3
    // 4. Update generation in S3 + Redis
    // 5. Publish Kafka event
    // 6. Publish Redis invalidation
    // 7. Release lock
}
```

#### 4.3 Kafka Consumer for Invalidation

```go
// internal/workspaces/invalidation_consumer.go
type InvalidationConsumer struct {
    consumer *kafka.Reader
    manager  *EnterpriseWorkspaceManager
}

func (c *InvalidationConsumer) Run(ctx context.Context) {
    for {
        msg, err := c.consumer.ReadMessage(ctx)
        if err != nil {
            continue
        }
        
        var event CommitEvent
        json.Unmarshal(msg.Value, &event)
        
        // Notify manager to invalidate local cache for this project
        c.manager.InvalidateProject(event.TenantID, event.ProjectID, event.Generation)
    }
}
```

**Deliverables**:
- [x] Kafka producer for commit events
- [x] Async commit worker (optional mode)
- [x] Kafka consumer for cross-node invalidation

---

### Phase 5: Skills Optimization (Week 10)

**Goal**: Skills are fetched and cached independently from full workspace.

#### 5.1 Skills-Only Fetch

```go
// internal/skills/loader.go
func (l *Loader) LoadSkillsOnly(ctx context.Context, projectSvc ProjectService, userID int64, projectID string) ([]Skill, error) {
    // Only fetch .manifold/skills/** from S3
    // Don't hydrate entire workspace
    
    skillsPrefix := fmt.Sprintf(".manifold/skills/")
    files, err := projectSvc.ListTree(ctx, userID, projectID, skillsPrefix)
    if err != nil {
        return nil, err
    }
    
    var skills []Skill
    for _, f := range files {
        if !strings.HasSuffix(f.Name, "SKILL.md") {
            continue
        }
        
        content, err := projectSvc.ReadFile(ctx, userID, projectID, f.Path)
        if err != nil {
            continue
        }
        
        skill, err := parseSkill(content)
        if err != nil {
            continue
        }
        skills = append(skills, skill)
    }
    
    return skills, nil
}
```

#### 5.2 Redis Skills Cache

```go
// internal/skills/redis_cache.go
type RedisSkillsCache struct {
    client *redis.Client
    ttl    time.Duration
}

func (c *RedisSkillsCache) GetRenderedPrompt(ctx context.Context, tenantID, projectID string, skillsGen int64) (string, bool) {
    key := fmt.Sprintf("skills:%s:%s:%d:prompt", tenantID, projectID, skillsGen)
    val, err := c.client.Get(ctx, key).Result()
    if err != nil {
        return "", false
    }
    return val, true
}

func (c *RedisSkillsCache) SetRenderedPrompt(ctx context.Context, tenantID, projectID string, skillsGen int64, prompt string) error {
    key := fmt.Sprintf("skills:%s:%s:%d:prompt", tenantID, projectID, skillsGen)
    return c.client.Set(ctx, key, prompt, c.ttl).Err()
}
```

**Deliverables**:
- [x] Skills-only fetch (no full workspace hydration)
- [x] Redis-backed skills prompt cache

---

### Phase 6: Configuration & Deployment (Week 11)

#### 6.1 Configuration Schema

```yaml
# config.yaml

# Simple mode (default)
projects:
  backend: filesystem  # or "s3"
  workspace:
    mode: legacy       # or "ephemeral" or "enterprise"
    root: ""           # defaults to WORKDIR/sandboxes

# Enterprise mode
projects:
  backend: s3
  encrypt: true
  workspace:
    mode: enterprise
    cache_dir: /var/lib/manifold/cache      # Encrypted cache
    tmpfs_dir: /dev/shm/manifold            # tmpfs for plaintext
    cache_ttl: 24h
    session_idle_timeout: 30m
  encryption:
    provider: vault  # or "awskms" or "file"
    vault:
      address: https://vault.example.com
      key_name: manifold-tenant-keys

redis:
  enabled: true
  addr: redis:6379

kafka:
  enabled: true
  brokers:
    - kafka:9092
  topics:
    commits: manifold.project.commits
```

#### 6.2 Docker Compose (Enterprise)

```yaml
# docker-compose.enterprise.yml
services:
  agentd:
    image: manifold/agentd:latest
    volumes:
      - /dev/shm/manifold:/dev/shm/manifold:rw  # tmpfs
      - manifold-cache:/var/lib/manifold/cache   # encrypted cache
    environment:
      - MANIFOLD_PROJECTS_BACKEND=s3
      - MANIFOLD_PROJECTS_WORKSPACE_MODE=enterprise
    depends_on:
      - postgres
      - redis
      - kafka
      - minio

  redis:
    image: redis:7-alpine
    volumes:
      - redis-data:/data

  kafka:
    image: confluentinc/cp-kafka:7.5.0
    # ... standard Kafka config

  zookeeper:
    image: confluentinc/cp-zookeeper:7.5.0
    # ...

  minio:
    image: minio/minio:latest
    # ...

  vault:
    image: hashicorp/vault:1.15
    # ...

volumes:
  redis-data:
  manifold-cache:
```

**Deliverables**:
- [x] Updated configuration schema
- [x] Enterprise docker-compose
- [x] Deployment documentation

---

### Phase 7: Testing & Hardening (Week 12)

#### 7.1 Test Cases

- [x] **Backward compatibility**: Simple mode works unchanged
- [x] **Session stability**: Multiple turns reuse workspace (0 S3 calls)
- [x] **Generation detection**: Stale workspace triggers refresh
- [x] **Cross-session visibility**: Skills updated in session A visible in session B (via generation)
- [ ] **Encryption**: No plaintext on disk in enterprise mode
- [x] **Conflict handling**: Concurrent commits handled gracefully
- [ ] **Recovery**: Node restart recovers from encrypted cache
- [x] **Eviction**: Cache cleanup under memory pressure

#### 7.2 Performance Benchmarks

- [x] Chat turn latency (warm vs cold workspace)
- [x] S3 request count per turn
- [x] Concurrent users per node
- [x] Cache eviction overhead

**Deliverables**:
- [x] Integration test suite
- [x] Performance benchmarks
- [x] Security audit checklist

---

## Summary: What Happens on Each Chat Turn

### Simple Mode (No Change)
```
Turn N:
  1. Checkout workspace (full hydrate every time - current behavior)
  2. Run workflow
  3. Commit changes
```

### Enterprise Mode (Optimized)
```
Turn N (warm):
  1. Check Redis: generation unchanged?
     → YES: Use existing materialized workspace (0 S3 calls)
     → NO: Incremental sync changed files only
  2. Check skills_generation unchanged?
     → YES: Use cached skills prompt (0 S3 calls)
     → NO: Fetch only .manifold/skills/**, cache result
  3. Run workflow against tmpfs workspace
  4. If dirty:
     a. Sync changed files: tmpfs → encrypted cache
     b. Debounce/batch upload to S3
     c. Increment generation
     d. Publish Kafka event
     e. Publish Redis invalidation
```

---

## Risk Mitigation

| Risk | Mitigation |
|------|------------|
| tmpfs size limits | Monitor usage; evict idle sessions; alert on pressure |
| DEK cache in memory | Short TTL; secure memory handling; no swap |
| Kafka lag | Monitor consumer lag; Redis pub/sub as fast path |
| Redis unavailable | Fallback to S3 metadata HEAD; degrade gracefully |
| Conflict on concurrent commit | Optimistic concurrency with generation; retry with refresh |
| Node crash with dirty data | Periodic safety flush (dirty sessions only) |

---

## Migration Path

1. **Phase 1-2**: Deploy with Redis optional; existing deployments unaffected
2. **Phase 3-4**: Introduce enterprise mode as opt-in; requires config change
3. **Phase 5-7**: Stabilize; recommend enterprise for multi-tenant deployments

Existing simple deployments continue working with no changes required.
