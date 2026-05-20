# Evolving Memory

Evolving memory is Manifold's experience-reuse system for agent runs. It stores compact lessons from completed tasks, retrieves relevant past experiences for new tasks, and maintains the memory corpus over time.

It is separate from chat summarization. Chat summarization keeps conversation history small enough for the model context. Evolving memory stores reusable task experience so future runs can avoid repeated mistakes and reuse successful strategies.

## Runtime Flow

At runtime the engine uses a Search -> Synthesis -> Evolve loop:

1. **Search**: embed the current task and retrieve relevant memories.
2. **Synthesis**: format retrieved memories into context for the main agent.
3. **Evolve**: after the run, summarize the experience and store it with feedback, type, scope, strategy-card data, and embedding.

When ReMem is enabled, a short memory-preparation loop runs before the main agent answers. ReMem may inspect retrieved memories and request safe maintenance edits, but it is not the final answering agent.

## Current Capabilities

- ExpRecent: recent in-process task window.
- ExpRAG: embedding-based experience retrieval.
- Hybrid retrieval: pgvector nearest-neighbor search plus PostgreSQL full-text keyword search when the Postgres store is available.
- Ranking: dense similarity blended with recency decay, structured feedback quality, access-count boost, and MMR diversification.
- Memory types: `factual`, `procedural`, and `episodic`.
- Strategy cards: compact reusable strategies generated for non-trivial ReMem traces.
- Scope promotion: successful procedural memories can be promoted from `session` scope to `user` scope after repeated retrieval.
- Smart pruning: duplicate merge and relevance-based pruning, while protecting frequently reused successful memories.
- Persistence: optional Postgres-backed `evolving_memories` table with pgvector and full-text indexes.
- Observability: debug endpoints, UI inspector, score explanations, local process metrics, and optional ClickHouse-backed historical metrics.

## Recommended Configuration

For an active local development agent with Postgres persistence and embeddings enabled:

```yaml
evolvingMemory:
  enabled: true

  # Persistence and session lifecycle.
  persistDebounceMs: 250
  sessionTTLMinutes: 120
  janitorIntervalMinutes: 15
  storeJanitorIntervalMinutes: 60

  # Retrieval.
  maxSize: 2000
  topK: 4
  windowSize: 20
  enableRAG: true

  # ReMem memory-preparation loop.
  reMemEnabled: true
  maxInnerSteps: 3

  # Dedicated memory LLM. Use an OpenAI-compatible local endpoint if desired.
  llmClient:
    provider: local
    openai:
      apiKey: ""
      baseURL: http://localhost:11434/v1
      model: qwen-memory
      summaryBaseURL: http://localhost:11434/v1
      summaryModel: qwen-memory
      api: completions
      extraHeaders: {}
      extraParams: {}

  # Legacy shorthand still works, but llmClient is preferred when present.
  model: gpt-4o-mini

  # Smart pruning.
  enableSmartPrune: true
  pruneThreshold: 0.97
  relevanceDecay: 0.99
  minRelevance: 0.05
  pruneQualityFloor: 3
  promotionAccessThreshold: 5
```

The example config keeps `enabled: false` and `reMemEnabled: false` so new installs stay opt-in. Enable them deliberately after embeddings, database connectivity, and the memory model are working.

## Required Supporting Config

### Embeddings

`enableRAG: true` requires the shared `embedding` block. The same embedding service is used by vector features and evolving memory.

```yaml
embedding:
  baseURL: https://api.openai.com
  model: text-embedding-3-small
  apiKey: "${EMBED_API_KEY}"
  apiHeader: Authorization
  headers: {}
  path: /v1/embeddings
  timeoutSeconds: 30
```

The Postgres memory store creates an `embedding_vec vector(N)` column when `databases.vector.dimensions` is set. Keep that dimension aligned with the configured embedding model.

### Postgres Persistence

Evolving memory uses the default database DSN when available:

```yaml
databases:
  defaultDSN: "${DATABASE_URL}"
  vector:
    backend: postgres
    dsn: "${DATABASE_URL}"
    dimensions: 1536
    metric: cosine
```

If no default Postgres DSN is available, evolving memory still works in process, but memories will not survive restarts.

Apply the SQL migrations under `deploy/postgres/migrations/` before production use. Agentd has defensive table/index initialization, but that initialization is not a replacement for managing schema migrations in deployed environments.

Relevant migration files:

- `20260427_evolving_memory_pgvector.sql`
- `20260427_evolving_memory_phase7.sql`
- `20260427_evolving_memory_hybrid_search.sql`

## Configuration Reference

| Key | Default | Purpose |
| --- | --- | --- |
| `enabled` | `false` | Enables evolving memory for agent runs. |
| `llmClient` | unset | Dedicated provider configuration for memory summarization, ReMem, and strategy-card generation. Preferred over legacy shorthand fields. |
| `provider` | unset | Legacy provider override. Use `llmClient.provider` for new configs. |
| `model` | inherited/resolved | Legacy model shorthand for memory summarization/ReMem when `llmClient` does not provide one. |
| `persistDebounceMs` | `250` | Debounces async persistence writes after memory changes. Lower values persist sooner; higher values reduce write churn. |
| `sessionTTLMinutes` | `60` | Evicts idle in-process per-session memory objects after this many minutes. This does not delete durable rows. |
| `janitorIntervalMinutes` | `15` | How often agentd scans idle in-process session memories for eviction. |
| `storeJanitorIntervalMinutes` | `60` | How often the durable store janitor removes rows whose `expires_at` has passed. |
| `maxSize` | `1000` | Maximum entries retained per in-memory memory instance. Smart pruning or FIFO pruning runs when this is exceeded. |
| `topK` | `4` | Number of memories injected for a task after ranking and MMR diversification. |
| `windowSize` | `20` | Size of the ExpRecent sliding window used for recent experience context. |
| `enableRAG` | `false` unless configured | Enables embedding-based retrieval. Requires a working embedding service. |
| `reMemEnabled` | `false` | Enables the ReMem memory-preparation loop before the main agent run. |
| `maxInnerSteps` | `5` | Maximum ReMem THINK/REFINE iterations before forcing ACT. `3` is a good operational default. |
| `enableSmartPrune` | `false` | Enables duplicate merging and relevance-based pruning. |
| `pruneThreshold` | `0.95` | Cosine similarity threshold above which entries are treated as near-duplicates for merge/prune decisions. Higher values are less aggressive. |
| `relevanceDecay` | `0.99` | Daily relevance decay factor used during pruning. Lower values age out old memories faster. |
| `minRelevance` | `0.1` | Entries below this relevance can be pruned when smart pruning is active. Lower values retain more old memories. |
| `pruneQualityFloor` | `3` | Protects successful memories with at least this many accesses from relevance pruning. |
| `promotionAccessThreshold` | `5` | Promotes successful procedural session memories to user scope after this many accesses. |

Internally, retrieval also uses ranking weights and an MMR lambda. These are currently code-level defaults, not YAML configuration keys.

## Retrieval and Ranking

When `enableRAG` is disabled, the engine does not call the embedding service for evolving memory. It still stores text memories and can inject the ExpRecent sliding window, but semantic/vector retrieval is skipped.

When `enableRAG` is enabled, the engine embeds the current task and searches memory. New memories are embedded from retrieval-oriented text that combines the task, outcome, distilled summary, strategy card, and result text so the reusable lesson can be found even when the future request is phrased differently.

With Postgres persistence, search uses:

- pgvector cosine search against `embedding_vec`
- full-text keyword search over `input`, `output`, `feedback`, `summary`, and `strategy_card`
- reciprocal-rank fusion when both search modes return candidates

If query embedding fails but keyword candidates are available, search falls back to keyword results. If write-time embedding fails, the text memory is still stored with embedding diagnostics in metadata and can be found by keyword search or included through ExpRecent.

Candidates are then rescored using:

- embedding similarity
- recency decay based on `last_accessed_at` or `created_at`
- structured feedback quality (`success` boosts, `failure` lowers)
- access-count boost for frequently reused memories
- MMR diversification to avoid returning several near-identical memories

Search updates access counters asynchronously. Repeated successful procedural memories can move from `session` scope to `user` scope so they are available across that user's future sessions.

Existing rows can be backfilled with the current retrieval-text recipe through `EvolvingMemory.RebuildEmbeddings`.

## Memory Entry Shape

Each memory stores the task, result, distilled lesson, retrieval data, feedback, and lifecycle metadata:

```go
type MemoryEntry struct {
    ID                 string
    Input              string
    Output             string
    Feedback           string
    Summary            string
    RawTrace           string
    Embedding          []float32
    Metadata           map[string]interface{}
    CreatedAt          time.Time

    StructuredFeedback *StructuredFeedback
    MemoryType         MemoryType
    StrategyCard       string
    Scope              MemoryScope
    ExpiresAt          *time.Time
    AccessCount        int
    LastAccessedAt     time.Time
    RelevanceScore     float64
}
```

Stored text is bounded before persistence to avoid saving very large payloads. The summarizer and strategy-card prompts also instruct the model not to include secrets, credentials, private user data, or transient one-off details.

## ReMem

ReMem is a memory controller, not the final user-facing answer path. It runs before the main agent and returns one of these JSON actions:

- `THINK`: short operational notes about the task and retrieved memories.
- `REFINE_MEMORY`: maintenance edits for retrieved memory IDs.
- `ACT`: finish memory preparation and hand off to the main agent.

Supported edit operations are:

- `PRUNE`: remove an obsolete, misleading, or unsafe memory.
- `MERGE`: combine redundant memories that share the same reusable lesson.
- `UPDATE_TAG`: add metadata tags when a memory looks useful but should not be removed.

Use `reMemEnabled: true` only with a model that reliably emits valid JSON. If logs show repeated `remem_json_parse_failed_fallback_to_act`, disable ReMem or use a stronger memory model.

## Pruning and Retention

When smart pruning is enabled, the memory system:

1. Merges near-duplicates using `pruneThreshold`.
2. Computes relevance from recency and access count.
3. Removes low-relevance entries when above `maxSize`.
4. Protects successful frequently accessed memories according to `pruneQualityFloor`.

When smart pruning is disabled, the fallback behavior is FIFO pruning once `maxSize` is exceeded.

`sessionTTLMinutes` only evicts idle in-process memory objects. It does not delete durable memories. Durable deletion only happens for entries with an expired `expires_at`, through the store janitor.

## Observability

### Debug Endpoints

The debug handler is available at both `/debug/memory` and `/api/debug/memory`.

| Endpoint | Purpose |
| --- | --- |
| `GET /api/debug/memory` | Lists available debug subroutes. |
| `GET /api/debug/memory/sessions` | Lists chat sessions plus sessions that only have evolving memory. |
| `GET /api/debug/memory/sessions/{id}` | Shows chat summary, context messages, and memory plan for a session. |
| `GET /api/debug/memory/entries` | Lists chat memory entries with filtering and pagination. |
| `GET /api/debug/memory/plan` | Shows the derived chat memory budget plan. |
| `GET /api/debug/memory/evolving?session_id=...&query=...` | Shows evolving-memory state, recent window, and optional retrieved memories. |
| `GET /api/debug/memory/explain?session_id=...&query=...` | Shows ranking components for retrieved memories. |

The evolving debug response includes `enableRAG` plus search diagnostics such as retrieval mode (`vector`, `keyword`, `hybrid`, `recent`, or `disabled`), candidate counts, server-side vector usage, keyword-store usage, and embedding errors.

When auth is enabled, these endpoints require the current authenticated user and use that user's memory namespace.

### Metrics

The memory metrics endpoint is:

```text
GET /api/metrics/memory?window=24h
GET /api/metrics/memory?windowSeconds=3600
```

It reports:

- search count
- retrieved hit count and average hits per search
- average search latency
- evolve/write attempts by result
- evolve errors
- latest memory size by user/session
- smart merge count
- pruned entries by reason

Metrics are emitted via OpenTelemetry instruments and are also mirrored into bounded process-local telemetry when `obs.local.enabled` is true. The endpoint prefers ClickHouse when `obs.clickhouse.dsn` is configured and falls back to `source: "process"` when running without ClickHouse. If both local telemetry and ClickHouse are disabled, it returns `source: "none"`.

Metric names:

- `evolving_memory_search_latency_seconds`
- `evolving_memory_search_total`
- `evolving_memory_search_hits`
- `evolving_memory_evolve_total`
- `evolving_memory_size`
- `evolving_memory_smart_merge_total`
- `evolving_memory_pruned_total`

The Agentd UI Overview page includes a memory metrics panel and a memory inspector panel.

## Troubleshooting

Set `logLevel: debug` while diagnosing memory behavior.

Useful log messages:

- `evolving_memory_search`: retrieval ran and returned candidates.
- `evolving_memory_embed_query_failed`: the embedding service failed or returned invalid data.
- `evolving_memory_entry_added`: a new memory was stored.
- `evolving_memory_smart_merged`: duplicate memories were merged.
- `evolving_memory_relevance_pruned`: relevance pruning removed entries.
- `evolving_memory_fifo_pruned`: fallback FIFO pruning removed entries.
- `remem_action`: ReMem selected THINK, REFINE_MEMORY, or ACT.
- `remem_json_parse_failed_fallback_to_act`: the ReMem model did not return valid JSON.

Common fixes:

- If searches fail, verify `embedding.baseURL`, `embedding.path`, `embedding.model`, and API credentials.
- If Postgres vector search fails, confirm `pgvector` is installed and the migrations were applied with a dimensioned `vector(N)` column.
- If memory is lost after restart, verify `databases.defaultDSN` points to Postgres and agentd can connect during startup.
- If ReMem is slow or noisy, reduce `maxInnerSteps` to `3`, disable ReMem, or use a stronger instruction-following memory model.
- If too many duplicates accumulate, enable smart pruning and keep `pruneThreshold` around `0.95` to `0.97`.
- If useful old memories disappear too quickly, lower pruning aggressiveness by reducing `minRelevance` or increasing `maxSize`.

## Related Files

- Runtime implementation: `internal/agent/memory/evolving.go`
- ReMem controller: `internal/agent/memory/remem.go`
- Metrics instruments: `internal/agent/memory/metrics.go`
- Agentd wiring: `internal/agentd/run.go`
- Debug endpoints: `internal/agentd/handlers_memory.go`
- Metrics endpoint: `internal/agentd/memory_metrics_clickhouse.go` and `internal/agentd/process_metrics.go`
- Postgres store: `internal/persistence/databases/evolving_memory_store_postgres.go`
- Example config: `config.yaml.example`
