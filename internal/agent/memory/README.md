# Manifold Memory Systems

This directory contains the agent memory systems that are used to shorten chat
history, retrieve useful long-term context, record run episodes, distill durable
beliefs, and expose graph memory for observability. The current implementation
is not a single memory store. It is a set of coordinated lanes with different
trust levels and lifecycles.

## Mental Model

| System | Main files | Purpose | Typical prompt role |
| --- | --- | --- | --- |
| Chat transcript memory | `manager*.go` | Persistence-backed session history with rolling summaries and OpenAI Responses compaction fallback support. | Keeps the conversation coherent without resending every prior message. |
| Unified memory runtime | `runtime.go` | Concurrently gathers policy, belief, MAGMA, evolving, and recent context under one prompt budget. | Adds a bounded runtime context block to the current user message. |
| Evolving memory | `evolving*.go`, `remem.go` | Stores task episodes as reusable lessons, summaries, strategy cards, embeddings, and feedback. | Recalls prior task experience and operational strategies. |
| Belief memory | `belief/*.go` | Durable scoped claims with evidence, confidence, lifecycle, promotion, and enforcement metadata. | Surfaces higher-trust facts, preferences, procedures, constraints, and capabilities. |
| MAGMA graph memory | `magma/*.go` | Agentic event graph over semantic, temporal, causal, and entity views. | Retrieves structured graph context and supports graph observability. |

Transit is adjacent to this package rather than implemented here. It provides
durable key-value memory for objective state and can be mirrored into MAGMA when
records are embeddable.

## Runtime Flow

For normal agentd runs, prefer the unified runtime path:

1. `agentd` builds a per-user engine in `internal/agentd/engine_runtime.go`.
2. `configureUnifiedMemoryRuntime` attaches `*memory.Runtime` when
   `memory.enabled` and per-run memory settings are enabled.
3. `Engine.augmentWithUnifiedMemory` calls `Runtime.PrepareContext` before the
   model request.
4. `Runtime` starts independent lanes for policy, belief, MAGMA, evolving, and
   optional recent context. Each lane has its own timeout.
5. `Runtime` joins returned sections in this prompt order: policy, belief,
   MAGMA, evolving, recent.
6. `buildRuntimeContext` enforces `memory.retrieval.maxTokensPerPrompt` and
   truncates with an omission marker when needed.
7. After the run, `Engine.recordMemoryEpisode` calls `Runtime.RecordEpisode`.
   That records evolving experience first, then belief-memory episode and
   candidates using the evolving entry ID as linkage.

`Engine.augmentWithMemory` and `Engine.augmentWithBeliefMemory` still exist as
direct legacy paths. The current coordinated agentd path is `Runtime`.

## Chat Transcript Memory

`Manager` is the session transcript compactor, not semantic long-term memory.
It uses `persistence.ChatStore` to load prior messages and session summary
state, then returns the prompt history for a target model/provider.

Key behavior:

- `BuildContextForProvider` loads persisted messages, ensures any needed
  summary exists, appends a compatible summary, and keeps a raw recent tail.
- Summarization is token-budget driven with a reserve buffer for output and
  reasoning tokens.
- Tool-call chains are preserved when choosing the raw tail so a kept tool
  result is not separated from its assistant tool call.
- Summaries are dual-format when Responses compaction is available:
  encrypted OpenAI compaction plus plain text fallback for non-OpenAI models.
- If compaction is unavailable, the manager falls back to plain text summaries.

Use this when you need prior chat turns. Do not use it as a source of factual
or procedural memory outside the session transcript.

## Evolving Memory

`EvolvingMemory` implements the Search -> Synthesis -> Evolve loop for task
experience. It stores `MemoryEntry` records with the original input, output,
feedback, generated summary, optional reasoning trace, memory type, strategy
card, scope, access stats, relevance score, and optional embedding.

Retrieval behavior:

- ExpRAG uses embeddings when `EnableRAG` is true.
- Dense vector search can run in-process or through a store implementing
  `EvolvingMemorySearchStore`.
- Keyword search can be blended through `EvolvingMemoryKeywordStore` for exact
  identifiers, paths, and error codes.
- Results are fused and ranked with similarity, recency decay, feedback quality,
  access count, MMR diversity, and optional reranking.
- `BuildExpRecentContext` provides a sliding recent window when configured.
- `SynthesizeScored` formats successful examples and cautions separately.

Write behavior:

- `EvolveEnhanced` redacts common PII/secrets, caps stored field sizes, creates
  an LLM summary, classifies entries as factual, procedural, or episodic, and
  embeds retrieval text when RAG is enabled.
- Smart pruning can merge near-duplicates and prune by relevance when capacity
  is exceeded. Otherwise old entries are pruned FIFO.
- Successful procedural entries can be promoted from session scope to user
  scope after repeated access when the store supports promotion.
- Entries are persisted asynchronously when a store is configured.
- Entries can be mirrored into MAGMA through `EvolvingMemoryMagmaSink`.

Persistence:

- `EvolvingMemoryStore` supports load/save snapshots.
- Optional extensions support delta upserts/deletes, access touches,
  server-side vector search, keyword search, user-scope promotion, and expired
  row cleanup.
- agentd caches one `EvolvingMemory` per user/session and evicts idle session
  instances by `evolvingMemory.sessionTTLMinutes`.
- Postgres-backed implementations live under `internal/persistence/databases`.

### ReMem

`ReMemController` is an optional Think/Act/Refine controller over evolving
memory. It is enabled by `memory.evolving.reMemEnabled` or the legacy
`evolvingMemory.reMemEnabled` setting.

The controller:

- searches evolving memory for the task,
- asks the configured memory model for JSON actions,
- handles `THINK`, `REFINE_MEMORY`, and `ACT`,
- applies memory edit operations such as pruning or tag updates,
- generates strategy cards only when the trace has enough substance.

ReMem is a memory-preparation step. It does not receive tool schemas and does
not dispatch tools.

## Belief Memory

Belief memory stores durable scoped claims and evidence. It is intended for
facts and constraints that should survive beyond a single chat transcript.

Core records:

- `Scope`: org, project, objective, session, user, and role hierarchy.
- `Episode`: run envelope linked to project/objective/session and optionally an
  evolving memory entry.
- `Belief`: normalized statement with kind, confidence, evidence counts,
  status, enforcement mode, review state, optional embedding, and metadata.
- `Evidence`: supporting or contradicting source records.
- `Promotion`: audit trail for scope widening.
- `CandidateRecord`: reviewable/auditable distillation output.

Distillation:

- `SimpleDistiller` conservatively creates low-confidence candidates from run
  outcome and summary text.
- `LLMDistiller` can produce multiple validated candidates, audit rejected or
  queued outputs, and auto-apply only above configured confidence thresholds.
- `ApplyCandidate` merges candidates into existing scoped beliefs by normalized
  statement hash and records evidence.

Retrieval:

- `StoreRetriever` walks nearby scopes in this order when available: session,
  objective, project, user, role, org.
- Results are ranked by store score, scope proximity, confidence, evidence
  balance, recency, and lexical match.
- `GraphEnrichedRetriever` can add related superseding, contradicting, and
  promoted beliefs through the belief graph.
- RAG evidence can be blended into the belief lane as clearly delimited
  untrusted evidence when `enableRAGEvidence` is enabled.

Lifecycle and enforcement:

- `LifecycleService` promotes eligible beliefs to broader scopes, decays stale
  beliefs, retracts beliefs, and supersedes outdated beliefs.
- Constraint beliefs can be compiled into policy records through a
  `PolicySink` when enforcement is enabled.
- Beliefs can be mirrored into MAGMA through `BeliefMagmaSink`.

Belief episode recording currently requires an objective ID. Retrieval can still
use broader scopes when those scopes exist.

## MAGMA Graph Memory

MAGMA stores memory events as a graph with multiple views:

- semantic similarity,
- temporal relationships,
- causal relationships,
- entity mentions and co-reference.

`magma.Service` ingests events, embeds them, stores event nodes, writes vector
records, and queues consolidation work. Consolidation can use rules and an
optional LLM to extract temporal attributes, entities, and causal edges. Workers
are started lazily by sinks and can also be drained by observability actions.

`magma.QueryEngine` classifies query intent with rules, LLM, or hybrid mode,
chooses a traversal policy, anchors by entity and/or vector search, traverses
selected graph views, and builds either structured or text context.

MAGMA accepts events from:

- evolving memory entries,
- belief memory records,
- Transit records with embedding enabled.

Lifecycle settings can prune old events, cap edges by source/relation, flag
low-confidence edges for review, and require review approval.

## Configuration

Prefer the unified `memory:` block. The loader mirrors it into the legacy
top-level subsystem configs so older code paths still see `cfg.EvolvingMemory`,
`cfg.BeliefMemory`, and `cfg.Magma`.

```yaml
memory:
  enabled: true
  retrieval:
    maxTokensPerPrompt: 2200
    timeoutMs: 700
    includeRecent: true
  llmClients:
    evolving: {}
    beliefDistillation: {}
    magmaConsolidation: {}
  evolving:
    topK: 4
    windowSize: 20
    enableRAG: true
  belief:
    maxBeliefsPerPrompt: 5
    retrieval:
      minConfidence: 0.35
      maxTokensPerPrompt: 700
      includeContradictions: true
  magma:
    retrieval:
      defaultHops: 2
      defaultMaxNodes: 10
      intentClassification: hybrid
      contextFormat: structured
```

Important notes:

- With a top-level `memory:` block, `memory.enabled` is the master switch for
  the coordinated memory stack.
- `memory.retrieval.*` controls the unified runtime block, not transcript
  compaction.
- `memory.llmClients.evolving`, `memory.llmClients.beliefDistillation`, and
  `memory.llmClients.magmaConsolidation` can override the main LLM client for
  memory jobs.
- Legacy `evolvingMemory:`, `beliefMemory:`, and `magma:` blocks still exist,
  but `config.yaml.example` marks `evolvingMemory` as deprecated in favor of
  `memory.evolving`.
- Chat transcript summarization is configured separately through the chat memory
  manager settings.

## Observability and Maintenance

Agentd exposes several memory-oriented endpoints:

- `/debug/memory` and `/api/debug/memory`: chat/evolving memory sessions,
  entries, plans, explanations, and evolving-entry maintenance.
- `/api/observability/memory`: unified memory/MAGMA overview, graph, timeline,
  review edges, retrieval explain, pruning, edge approval/retraction, node
  deletion, consolidation drain, and evolving embedding rebuild.
- `/api/debug/beliefs` and `/api/beliefs`: belief search, evidence,
  promotions, candidate review, and retraction.
- `/api/metrics/memory`: local memory telemetry snapshots.

`MemoryCallbacks`, `MemoryMetrics`, local telemetry, logs, and OpenTelemetry
spans provide lower-level hooks for search, synthesis, evolve, MAGMA ingest,
consolidation, and graph query behavior.

## File Map

- `runtime.go`: unified memory runtime and lane diagnostics.
- `manager*.go`: chat transcript compaction and context reconstruction.
- `evolving*.go`: evolving memory retrieval, synthesis, writes, persistence,
  scoring, pruning, edits, and recent-window support.
- `remem.go`: optional Think/Act/Refine controller for active memory editing.
- `metrics.go`: memory telemetry and OpenTelemetry metrics.
- `belief/`: belief store interfaces, retrieval, distillation, lifecycle,
  promotion, RAG evidence blending, and objective Transit key helpers.
- `magma/`: graph memory service, query engine, graph views, consolidation,
  lifecycle, intent classification, tracing, and tests.
- `testdata/evals/unified_memory_quality.json`: evaluation fixture for unified
  memory quality.

## Testing

Run package tests after changing memory behavior:

```bash
go test ./internal/agent/memory/...
```

When changes affect agentd wiring, persistence stores, or debug endpoints, also
run the relevant adjacent packages:

```bash
go test ./internal/agent/... ./internal/agentd/... ./internal/persistence/databases/...
```
