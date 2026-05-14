# Data: Memory Systems

This page maps memory-related persistence and retrieval structures.

## Memory Entity Map

```mermaid
erDiagram
    TRANSIT_MEMORIES {
      uuid id
      bigint tenant_id
      text key_name
      text description
      text value
      int version
    }
    EVOLVING_MEMORIES {
      text id
      bigint user_id
      text session_id
      text summary
      jsonb metadata
    }
    DOCUMENTS {
      text id
      text text
      jsonb metadata
    }
    EMBEDDINGS {
      text id
      vector embedding
      jsonb metadata
    }
    NODES {
      text id
      text labels
      jsonb props
    }
    EDGES {
      text src
      text rel
      text dst
      jsonb props
    }
    BELIEF_SCOPES ||--o{ BELIEF_EPISODES : contains
    BELIEF_SCOPES ||--o{ BELIEFS : contains
    BELIEFS ||--o{ BELIEF_EVIDENCE : has
    BELIEFS ||--o{ BELIEF_PROMOTIONS : promotes
```

## Transit Fields

Transit records include key, description, value, base64 flag, embed flag, embed source, version, tenant/user audit fields, and timestamps.

## Belief Fields

Beliefs include statement, statement hash, confidence, evidence counts, status, optional expiry, embedding vector, and metadata. Evidence records link to episodes or external sources and record polarity/weight.

## Retrieval Paths

- Transit search can combine store/search/vector results depending on config.
- RAG retrieval fuses full-text and vector candidates, then optionally uses graph and reranking.
- Belief retrieval searches scoped beliefs and can blend RAG evidence into prompt context.
- Evolving memory retrieves and updates session-scoped experience entries.

## Evidence

- `internal/transit/types.go`.
- `internal/rag/service/service.go`.
- `internal/agent/belief/types.go`.
- `internal/persistence/databases/transit_store_postgres.go`, `evolving_memory_schema_postgres.go`, `belief_store_postgres.go`, `postgres_search.go`, `postgres_vector.go`, `postgres_graph.go`.
