# Evolving Memory System

This document describes the evolving memory system integrated into the agent engine, based on the paper ["Evolving Memory: Adaptive Reasoning and Learning in Multi-Turn Conversations"](https://arxiv.org/pdf/2511.20857).

## Overview

The evolving memory system implements three key capabilities:

1. **Search → Synthesis → Evolve Loop**: Self-evolving memory that continuously improves through experience
2. **ExpRAG (Experience Retrieval & Aggregation)**: Similarity-based retrieval of past successful/failed experiences
3. **ReMem (Think-Act-Refine)**: Advanced controller that actively edits and maintains memory quality

## Architecture

```
      +-----------+        +-----------+        +-----------+
x_t ->|  Search R |--R_t-> | Synthesis |--C_t-> |  LLM  F   |--> y_hat_t
      +-----------+        +-----------+        +-----------+
             ^                                         |
             |                                         v
             |             +-----------------------------+
             +-------------|      Evolve U: M_t -> M_{t+1}|
                           +-----------------------------+
```

## Configuration

The evolving memory system uses your existing embedding service configuration automatically. No separate embedding setup is needed - it leverages the same `EMBED_*` environment variables used by pgvector and the vector search tool.

### Via Environment Variables (.env file)

```bash
# Embedding service (already configured for other features)
EMBED_BASE_URL="http://192.168.1.244:32184"
EMBED_MODEL="path/to/model.gguf"
EMBED_API_HEADER="Authorization"
EMBED_PATH="/v1/embeddings"
EMBED_TIMEOUT=30

# Vector dimensions (for pgvector) - evolving memory adapts automatically
VECTOR_DIMENSIONS=1024

# Evolving Memory configuration
EVOLVING_MEMORY_ENABLED=true
EVOLVING_MEMORY_MAX_SIZE=1000
EVOLVING_MEMORY_TOP_K=4
EVOLVING_MEMORY_WINDOW_SIZE=20
EVOLVING_MEMORY_ENABLE_RAG=true
EVOLVING_MEMORY_REMEM_ENABLED=false
EVOLVING_MEMORY_MAX_INNER_STEPS=5
EVOLVING_MEMORY_MODEL=gpt-4o-mini
```

### Via config.yaml

```yaml
# Evolving memory configuration
evolvingMemory:
  enabled: true              # Enable the evolving memory system
  maxSize: 1000             # Maximum number of memory entries to retain
  topK: 4                   # Number of similar experiences to retrieve
  windowSize: 20            # Size of sliding window for ExpRecent
  enableRAG: true           # Enable ExpRAG similarity-based retrieval
  reMemEnabled: false       # Enable Think-Act-Refine mode (advanced)
  maxInnerSteps: 5          # Maximum THINK/REFINE loops in ReMem mode
  model: "gpt-4o-mini"      # Model for memory summarization

# Note: embedding service uses your existing EMBED_* environment variables
# No additional embedding configuration needed
```

## Memory Entry Structure

Each memory entry contains:

```go
type MemoryEntry struct {
    ID        string                 // Unique identifier
    Input     string                 // Original task/query
    Output    string                 // Model's response
    Feedback  string                 // Success/failure signal
    Summary   string                 // LLM-generated key lesson
    RawTrace  string                 // Detailed reasoning trace
    Embedding []float32              // Vector for similarity search
    Metadata  map[string]interface{} // Timestamp, domain, tags
    CreatedAt time.Time
}
```

## Usage Modes

### 1. ExpRecent (Default)

Maintains a sliding window of recent task experiences:

```yaml
evolvingMemory:
  enabled: true
  windowSize: 20
  enableRAG: false
```

- Keeps the most recent N tasks in memory
- Injects compressed summaries into context
- Minimal overhead, good for sequential tasks

### 2. ExpRAG (Recommended)

Retrieves similar past experiences via embedding similarity:

```yaml
evolvingMemory:
  enabled: true
  enableRAG: true
  topK: 4
  maxSize: 1000
```

- Embeds each task and stores in memory
- Retrieves top-k similar experiences at runtime
- Learns from both successes and failures
- Requires embedding service

### 3. ReMem (Advanced)

Full Think-Act-Refine controller with active memory editing:

```yaml
evolvingMemory:
  enabled: true
  enableRAG: true
  reMemEnabled: true
  maxInnerSteps: 5
```

**Actions:**
- **THINK**: Internal reasoning decomposition
- **REFINE**: Prune, merge, or reorganize memories
- **ACT**: Final output

**Memory Operations:**
- `PRUNE`: Remove low-quality entries
- `MERGE`: Combine similar experiences
- `UPDATE_TAG`: Add metadata for filtering

## Integration

The memory system integrates seamlessly with the existing agent engine and **automatically uses your existing embedding service configuration**:

```go
import "manifold/internal/agent/memory"

// Initialize evolving memory - uses cfg.Embedding from your .env file
memCfg := memory.EvolvingMemoryConfig{
    EmbeddingConfig: cfg.Embedding,  // Reuses EMBED_* environment variables
    LLM:             llmProvider,
    Model:           cfg.EvolvingMemory.Model,
    MaxSize:         cfg.EvolvingMemory.MaxSize,
    TopK:            cfg.EvolvingMemory.TopK,
    WindowSize:      cfg.EvolvingMemory.WindowSize,
    EnableRAG:       cfg.EvolvingMemory.EnableRAG,
}
evolvingMem := memory.NewEvolvingMemory(memCfg)

// Create agent engine with memory
engine := &agent.Engine{
    LLM:            llmProvider,
    Tools:          toolRegistry,
    EvolvingMemory: evolvingMem,
    // ... other config
}

// Memory is automatically used during Run()
response, err := engine.Run(ctx, userInput, history)
```

**Key Points:**
- Uses the same embedding service as pgvector and vector search
- No separate embedding setup required
- Works with any embedding dimension (384, 768, 1024, 1536, etc.)
- Embeddings are cached in memory with each entry

## ReMem Controller Usage

For advanced Think-Act-Refine mode:

```go
// Initialize ReMem controller
reMemCtrl := memory.NewReMemController(memory.ReMemConfig{
    Memory:        evolvingMem,
    LLM:           llmProvider,
    Model:         "gpt-4",
    MaxInnerSteps: 5,
})

engine := &agent.Engine{
    LLM:             llmProvider,
    Tools:           toolRegistry,
    EvolvingMemory:  evolvingMem,
    ReMemEnabled:    true,
    ReMemController: reMemCtrl,
}
```

## Benefits

Based on the paper's empirical results:

1. **Improved Success Rate**: 15-25% higher success on multi-turn tasks
2. **Reduced Steps**: Fewer iterations needed to solve tasks
3. **Better Generalization**: Learns patterns from past experiences
4. **Self-Improvement**: Memory quality evolves over time

## Performance Considerations

1. **Embedding Latency**: Each memory operation requires embedding calls
   - Uses your existing embedding service (same as pgvector/vector search)
   - Embedding dimensions are determined by your configured model (e.g., 1024 for Qwen3-Embedding)
   - Consider batch embedding for efficiency

2. **Memory Size**: Keep maxSize reasonable (100-1000 entries)
   - Larger memory = slower retrieval (O(n) similarity comparison)
   - Quality matters more than quantity

3. **Vector Dimensions**: The system is dimension-agnostic
   - Works with any embedding dimension (384, 768, 1024, 1536, etc.)
   - Your `VECTOR_DIMENSIONS` setting affects pgvector storage, not evolving memory
   - Evolving memory stores embeddings as `[]float32` of variable length

4. **ReMem Overhead**: Think-Act-Refine adds 2-5 extra LLM calls per task
   - Use for complex reasoning tasks only
   - Start with ExpRAG, upgrade to ReMem if needed

## Example Workflow

1. User submits task
2. Agent searches memory for similar past experiences (top-k=4)
3. Synthesizes context from retrieved memories
4. LLM generates response using augmented context
5. Agent stores new experience with feedback and LLM-generated summary
6. Memory evolves: embeddings updated, old entries pruned if needed

## Debugging

Enable detailed logging:

```yaml
logLevel: "debug"
```

Look for these log messages:
- `evolving_memory_search`: Retrieval operation
- `evolving_memory_exprag`: ExpRAG mode activated
- `evolving_memory_exprecent`: ExpRecent fallback
- `evolving_memory_entry_added`: New experience stored
- `remem_action`: ReMem controller action (THINK/REFINE/ACT)

## Future Enhancements

- Persistence backend for memory entries (currently in-memory only)
- Advanced pruning strategies (importance-based, time-decay)
- Multi-domain memory partitioning
- Distributed memory sharing across agent instances
- Memory visualization tools

## References

- Paper: [Evolving Memory: Adaptive Reasoning and Learning](https://arxiv.org/pdf/2511.20857)
- Implementation follows the paper's Search → Synthesis → Evolve pattern
- Uses BGE-base embeddings as recommended (or equivalent)
