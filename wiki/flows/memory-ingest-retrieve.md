# Runtime Flow: Memory Ingest and Retrieval

This page focuses on document/memory indexing flows, not chat summarization.

## Transit Record Flow

```mermaid
sequenceDiagram
    participant Caller
    participant Service as transit.Service
    participant Store as Transit store
    participant Search as Search index
    participant Emb as Embedding service
    participant Vector as Vector store

    Caller->>Service: Create/update record
    Service->>Service: Validate key and defaults
    Service->>Store: Persist versioned record
    Service->>Search: Index description/value text
    alt vector enabled and record embed=true
        Service->>Emb: Generate embedding
        Emb-->>Service: vector
        Service->>Vector: Upsert vector metadata
    end
    Service-->>Caller: Record
```

## RAG Ingest/Retrieve Flow

```mermaid
flowchart TD
    Ingest[IngestRequest] --> Preprocess[Preprocess text, metadata, language, hash]
    Preprocess --> Chunk[Chunk document]
    Chunk --> SearchDoc[Index document]
    Chunk --> SearchChunks[Index chunks]
    Chunk --> Embed{Embedding enabled?}
    Embed -->|yes| Vector[Upsert chunk vectors]
    Embed -->|no| SkipVector[Skip vector]
    Chunk --> Graph{Graph enabled?}
    Graph -->|yes| GraphStore[Upsert document/chunk graph]
    Graph -->|no| SkipGraph[Skip graph]

    Query[Retrieve query] --> Plan[Build query plan]
    Plan --> FTS[Full-text candidates]
    Plan --> VEC[Vector candidates]
    FTS --> Fuse[Fuse/diversify]
    VEC --> Fuse
    Fuse --> Enrich[Graph enrich/rerank/snippets]
    Enrich --> Results[Retrieved items]
```

## Evidence

- `internal/transit/service.go` validates, stores, indexes, and searches Transit records.
- `internal/rag/service/service.go` implements ingestion and retrieval stages.
- `internal/rag/ingest/*`, `internal/rag/retrieve/*`, `internal/rag/chunker/*`, `internal/rag/embedder/*` support the service.
- `internal/tools/rag/tool.go` and `internal/tools/transit/tools.go` expose agent tools.
