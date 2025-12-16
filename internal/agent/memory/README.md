# Evolving Memory Implementation

This directory contains the implementation of the **Evolving Memory** system based on the paper ["Evolving Memory: Adaptive Reasoning and Learning in Multi-Turn Conversations"](https://arxiv.org/pdf/2511.20857).

## Files

- **evolving.go**: Core memory system with Search → Synthesis → Evolve loop
  - `MemoryEntry`: Structured experience storage
  - `EvolvingMemory`: Main memory manager with retrieval and evolution
  - ExpRAG and ExpRecent implementations
  
- **remem.go**: Think-Act-Refine controller
  - `ReMemController`: Advanced reasoning loop with active memory editing
  - THINK, ACT, REFINE action modes
  - Memory pruning, merging, and reorganization

- **manager.go**: Original chat memory manager (preserved for compatibility)

- **evolving_test.go**: Comprehensive test suite

## Quick Start

### 1. Basic ExpRAG Mode

```go
import "manifold/internal/agent/memory"

// Create evolving memory
em := memory.NewEvolvingMemory(memory.EvolvingMemoryConfig{
    EmbeddingConfig: cfg.Embedding,
    LLM:             llmProvider,
    Model:           "gpt-4o-mini",
    MaxSize:         1000,
    TopK:            4,
    WindowSize:      20,
    EnableRAG:       true,
})

// Use with agent engine
engine := &agent.Engine{
    LLM:            llmProvider,
    Tools:          toolRegistry,
    EvolvingMemory: em,
}
```

### 2. Advanced ReMem Mode

```go
// Create ReMem controller
reMemCtrl := memory.NewReMemController(memory.ReMemConfig{
    Memory:        em,
    LLM:           llmProvider,
    Model:         "gpt-4",
    MaxInnerSteps: 5,
})

engine := &agent.Engine{
    LLM:             llmProvider,
    Tools:           toolRegistry,
    EvolvingMemory:  em,
    ReMemEnabled:    true,
    ReMemController: reMemCtrl,
}
```

## How It Works

### Search Phase (R)
1. Embed incoming query using configured embedding service
2. Compute cosine similarity against all stored memory entries
3. Return top-k most similar experiences

### Synthesis Phase (C)
1. Format retrieved experiences into structured template
2. Inject into LLM context alongside current task
3. Include both successful and failed past attempts

### Evolve Phase (U)
1. After task completion, store new experience
2. Generate LLM summary of key lessons learned
3. Embed and index for future retrieval
4. Prune oldest entries if exceeding maxSize

### ReMem Loop
1. **THINK**: Model decomposes task internally (private reasoning)
2. **REFINE**: Model prunes/merges/reorganizes memory entries
3. **ACT**: Model produces final output (terminates loop)

## Configuration

See `config.yaml.example` for full configuration options.

## Testing

```bash
# Run all tests
go test ./internal/agent/memory/...

# Run with live embedding service
go test -v ./internal/agent/memory/... -run TestEvolvingMemory

# Benchmark cosine similarity
go test -bench=. ./internal/agent/memory/...
```

## Performance

- **ExpRecent**: Minimal overhead, sliding window only
- **ExpRAG**: Adds embedding latency (~50-100ms per query with local model)
- **ReMem**: Adds 2-5 extra LLM calls per task (use for complex reasoning only)

## Limitations

Current implementation:
- Memory is in-memory only (not persisted across restarts)
- Single-agent memory (not shared across instances)
- Requires external embedding service

Future enhancements planned:
- Persistent storage backend (Postgres/Redis)
- Distributed memory via vector database
- Advanced pruning strategies (importance scoring, time decay)
- Memory visualization tools

## References

- Paper: https://arxiv.org/pdf/2511.20857
- Embedding model: BGE-base or nomic-embed-text (recommended)
- Similarity metric: Cosine similarity (standard for RAG systems)
