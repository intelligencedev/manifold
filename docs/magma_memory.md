# MAGMA Memory

MAGMA is Manifold's optional multi-graph memory layer for RAG and long-running agent memory. It stores each memory event once, then links it through four typed graph views:

- `semantic`: similarity edges between related events.
- `temporal`: `BEFORE`, `AFTER`, and `CONCURRENT` event ordering.
- `causal`: `CAUSES` edges extracted from grounded event text or LLM consolidation.
- `entity`: entity-to-event `MENTIONS` edges and entity-to-entity `RELATED_TO` co-reference edges.

When MAGMA is disabled, existing RAG behavior is unchanged. Tool calls can still opt in per request with `options.magma.enabled`.

## Configuration

```yaml
magma:
  enabled: false
  consolidation:
    model: gpt-4o-mini
    batchSize: 10
    maxQueueSize: 1000
    workerCount: 2
    prompts:
      consolidationExtraction: ""
      intentClassification: ""
  graphs:
    semantic:
      enabled: true
      topK: 20
      similarityThreshold: 0.7
    temporal:
      enabled: true
      dateResolution: auto
    causal:
      enabled: true
      llmThreshold: 0.8
    entity:
      enabled: true
      coReference: true
  retrieval:
    defaultHops: 2
    defaultMaxNodes: 10
    intentClassification: hybrid
    contextFormat: structured
  lifecycle:
    pruneIntervalMinutes: 0
    eventTTLHours: 0
    maxEdgesPerSourceRel: 0
    minSemanticWeight: 0
    lowConfidenceThreshold: 0.6
    requireReviewApproval: false
    requireCausalGrounding: false
```

Lifecycle defaults are conservative:

- `pruneIntervalMinutes: 0` disables the background lifecycle worker.
- `eventTTLHours: 0` disables durable event expiration.
- `maxEdgesPerSourceRel: 0` disables fanout simplification.
- `minSemanticWeight: 0` disables semantic edge pruning by weight.
- `lowConfidenceThreshold` controls which edges are flagged as `needs_review` during lifecycle pruning.
- `requireReviewApproval: true` makes retrieval ignore review-flagged edges until they are approved.
- `requireCausalGrounding: true` marks ungrounded LLM causal edges as `needs_review`.

## Causal Edge Provenance

Causal edges standardize these `Edge.Props` keys:

- `origin`: `llm_consolidation`, `operator`, `distiller`, or `import`.
- `model`: the model name when the edge came from an LLM.
- `confidence`: extraction confidence.
- `evidence`: a list of `{sourceKind, sourceId}` references.
- `approved_at` and `approved_by`: operator approval metadata.

`GroundEdge` appends evidence references and clears `review_state=needs_review`
when the only review reason was `ungrounded_causal`.

## Write Path

`rag_ingest` stores the normal RAG document and, when MAGMA is enabled, writes a MAGMA event on the fast path. The fast path embeds the raw event, stores the event node, mirrors the vector, and queues the event for consolidation.

Consolidation runs asynchronously through MAGMA workers, or synchronously in tests via `DrainConsolidation`. Consolidation resolves temporal attributes, entity mentions, semantic links, temporal links, and causal links. LLM extraction is optional and is controlled by the configured MAGMA LLM provider and prompts.

## Read Path

`rag_retrieve` can use MAGMA directly when `options.magma.enabled` is true or when global MAGMA retrieval is enabled. Query processing:

1. Classifies intent as temporal, entity, semantic, causal, or a mixed intent.
2. Selects graph views and traversal budgets.
3. Finds anchors through entity links and/or vector search.
4. Traverses typed graph views.
5. Builds structured context with timelines, entity profiles, causal chains, semantic clusters, and raw events.

When normal RAG retrieval uses `graphAugment` and dedicated MAGMA retrieval is not enabled, MAGMA graph context can still add related memory events to the result set.

## Lifecycle Operations

The MAGMA service exposes lifecycle hardening APIs:

- `Prune(ctx, policy)` expires old events, deletes low-weight semantic edges, simplifies high-fanout edge groups, and flags low-confidence edges for review.
- `StartLifecycleWorker(ctx, interval)` runs `Prune` periodically with the configured lifecycle policy.
- `ReviewEdges(ctx)` lists edges whose `review_state` is not approved.
- `ApproveEdge(ctx, selector, reviewer)` marks an edge as approved and records review metadata.
- `RetractEdge(ctx, selector, reason)` deletes an incorrect edge from the graph.

Manual lifecycle testing is available through the `magma_lifecycle` tool:

```json
{"action":"prune","policy":{"low_confidence_threshold":0.6}}
{"action":"review_edges"}
{"action":"approve_edge","selector":{"source":"event:t1:example","graph_type":"causal","rel":"CAUSES","target":"event:t1:example"},"reviewer":"manual-test"}
{"action":"retract_edge","selector":{"source":"event:t1:example","graph_type":"causal","rel":"CAUSES","target":"event:t1:example"},"reason":"manual-test"}
```

In-memory and Postgres graph backends implement the optional maintenance interface used by these APIs. Backends that do not implement it still support normal MAGMA ingest and retrieval, but lifecycle operations return `MaintenanceDisabled`.

## Observability

MAGMA emits metrics through the RAG metrics sink:

- `magma_ingestion_fast_ms`
- `magma_ingestion_consolidation_ms`
- `magma_events_total`
- `magma_query_ms`
- `magma_traversal_hops`
- `magma_context_tokens`
- `magma_intent_distribution`
- `magma_consolidation_success_rate`
- `magma_entity_resolution_accuracy`
- `magma_lifecycle_events_deleted`
- `magma_lifecycle_edges_deleted`
- `magma_lifecycle_edges_flagged_review`

MAGMA also emits OpenTelemetry spans for ingest, consolidation, query classification, anchor selection, traversal, pruning, approval, and retraction. See [observability.md](observability.md) for the full telemetry setup.

## Known Remaining Work

The implementation covers the runtime feature path and lifecycle hardening. Formal benchmark harnesses for LoCoMo, LongMemEval, and p99 latency validation remain separate validation work.
