# Evolving Memory System

This document describes the evolving memory system integrated into the agent engine, based on the paper ["Evo-Memory: Benchmarking LLM Agent Test-time Learning with Self-Evolving Memory"](https://arxiv.org/abs/2511.20857).

## Overview

The evolving memory system implements several key capabilities from the paper:

1. **Search → Synthesis → Evolve Loop**: Self-evolving memory that continuously improves through experience
2. **ExpRAG (Experience Retrieval & Aggregation)**: Similarity-based retrieval of past successful/failed experiences  
3. **ExpRecent**: Sliding window of recent task experiences for sequential learning
4. **ReMem (Think-Act-Refine)**: Advanced controller that actively edits and maintains memory quality
5. **Memory Type Classification**: Distinguishes between factual, procedural, and episodic memories
6. **Smart Pruning**: Similarity-based deduplication and relevance-based memory management
7. **Strategy Cards**: Reusable strategy patterns extracted from successful experiences

## Architecture

```
      +-----------+        +-----------+        +-----------+
x_t ->|  Search R |--R_t-> | Synthesis |--C_t-> |  LLM  F   |--> ŷ_t
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
EMBED_API_KEY="your_api_key"
EMBED_API_HEADER="Authorization"
EMBED_PATH="/v1/embeddings"
EMBED_TIMEOUT=30

# Evolving Memory configuration
EVOLVING_MEMORY_ENABLED=true           # Enable the evolving memory system
EVOLVING_MEMORY_MAX_SIZE=1000          # Maximum memory entries to retain
EVOLVING_MEMORY_TOP_K=4                # Number of similar experiences to retrieve
EVOLVING_MEMORY_WINDOW_SIZE=20         # Sliding window size for ExpRecent mode
EVOLVING_MEMORY_ENABLE_RAG=true        # Enable ExpRAG similarity-based retrieval
EVOLVING_MEMORY_REMEM_ENABLED=false    # Enable Think-Act-Refine mode (advanced)
EVOLVING_MEMORY_MAX_INNER_STEPS=5      # Maximum THINK/REFINE loops in ReMem mode
EVOLVING_MEMORY_MODEL=gpt-4o-mini      # Model for memory summarization

# Smart pruning (advanced)
EVOLVING_MEMORY_ENABLE_SMART_PRUNE=true  # Enable similarity-based dedup & relevance pruning
EVOLVING_MEMORY_PRUNE_THRESHOLD=0.95     # Similarity threshold for duplicate detection (0.0-1.0)
EVOLVING_MEMORY_RELEVANCE_DECAY=0.99     # Daily decay factor for relevance scores (0.0-1.0)
EVOLVING_MEMORY_MIN_RELEVANCE=0.1        # Minimum relevance to avoid pruning (0.0-1.0)
```

### Via config.yaml

```yaml
# Evolving memory configuration
evolvingMemory:
  enabled: true              # Enable the evolving memory system
  maxSize: 1000              # Maximum number of memory entries to retain
  topK: 4                    # Number of similar experiences to retrieve
  windowSize: 20             # Size of sliding window for ExpRecent
  enableRAG: true            # Enable ExpRAG similarity-based retrieval
  reMemEnabled: false        # Enable Think-Act-Refine mode (advanced)
  maxInnerSteps: 5           # Maximum THINK/REFINE loops in ReMem mode
  model: "gpt-4o-mini"       # Model for memory summarization
  # Smart pruning (advanced)
  enableSmartPrune: true     # Enable similarity-based dedup & relevance pruning
  pruneThreshold: 0.95       # Similarity threshold for duplicate detection (0.0-1.0)
  relevanceDecay: 0.99       # Daily decay factor for relevance scores (0.0-1.0)
  minRelevance: 0.1          # Minimum relevance to avoid pruning (0.0-1.0)

# Embedding service configuration (required for evolving memory)
embedding:
  baseURL: "http://localhost:11434"
  model: "nomic-embed-text"
  path: "/api/embeddings"
  timeoutSeconds: 30
  # For OpenAI-compatible services:
  # apiKey: "${OPENAI_API_KEY}"
  # apiHeader: "Authorization"
  # Optional: provide additional headers (map)
  # headers:
  #   x-trace-id: "abc123"
  #   x-algo: "v2"
```

## Memory Entry Structure

Each memory entry contains comprehensive information about task experiences:

```go
type MemoryEntry struct {
    ID                 string                 // Unique identifier
    Input              string                 // x_i: Original task/query
    Output             string                 // ŷ_i: Model's response
    Feedback           string                 // f_i: Success/failure signal (legacy)
    Summary            string                 // LLM-generated key lesson
    RawTrace           string                 // Detailed reasoning trace
    Embedding          []float32              // Vector for similarity search
    Metadata           map[string]interface{} // Timestamp, domain, tags
    CreatedAt          time.Time

    // Enhanced fields (from paper review)
    StructuredFeedback *StructuredFeedback    // Detailed feedback metrics
    MemoryType         MemoryType             // factual | procedural | episodic
    StrategyCard       string                 // Reusable strategy pattern
    AccessCount        int                    // For relevance-based pruning
    LastAccessedAt     time.Time              // For recency-based pruning
    RelevanceScore     float64                // Cumulative relevance metric
}
```

### Structured Feedback

The system supports detailed feedback beyond simple success/failure:

```go
type StructuredFeedback struct {
    Type         FeedbackType  // success | failure | partial | in_progress
    Correct      bool          // Binary correctness flag
    ProgressRate float64       // 0.0-1.0 for multi-turn tasks
    StepsUsed    int           // Actual steps taken
    StepsOptimal int           // Optimal steps (if known)
    Message      string        // Human-readable feedback
}
```

### Memory Types

The paper emphasizes distinguishing between different types of memory:

| Type | Description | Example |
|------|-------------|---------|
| `factual` | Facts, data, static knowledge ("What") | "The capital of France is Paris" |
| `procedural` | Strategies, workflows, how-to ("How") | "When solving quadratic equations, use the quadratic formula" |
| `episodic` | Specific task episodes | "User asked about weather, I used the API tool" |

## Usage Modes

### 1. ExpRecent (Lightweight)

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
- No embedding service required

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
- Tracks access patterns for relevance scoring

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
- **THINK**: Internal reasoning and task decomposition
- **REFINE_MEMORY**: Prune, merge, or reorganize memories
- **ACT**: Final output (ends the inner loop)

**Memory Operations:**
- `PRUNE`: Remove low-quality or irrelevant entries
- `MERGE`: Combine similar experiences with new summary
- `UPDATE_TAG`: Add metadata tags for filtering

## Strategy Cards

Strategy cards are reusable patterns extracted from successful task executions:

```
When confronted with <pattern>, do <strategy>. Avoid <mistakes>.
```

Example:
```
When confronted with a quadratic equation, apply the quadratic formula 
(-b ± √(b²-4ac))/2a. Avoid forgetting to check for complex roots when 
the discriminant is negative.
```

Strategy cards are automatically generated by the ReMem controller and stored with each memory entry for future reference.

## Smart Pruning

The system includes intelligent memory management:

### Similarity-Based Deduplication
- Before adding new entries, checks for near-duplicates (similarity > 0.95)
- Automatically merges duplicate experiences
- Preserves unique insights while reducing redundancy

### Relevance-Based Pruning
When memory exceeds `maxSize`, entries are pruned based on:
1. **Access frequency**: More accessed memories score higher
2. **Recency**: Recently accessed memories get priority
3. **Time decay**: Relevance scores decay over time (default: 1% daily)

Configuration defaults (programmatic):
```go
PruneThreshold:   0.95   // Similarity threshold for duplicate detection
RelevanceDecay:   0.99   // Daily decay factor for relevance scores
MinRelevance:     0.1    // Minimum relevance to avoid pruning
EnableSmartPrune: false  // Enable via code (not yet in config.yaml)
```

## Task Similarity Metrics

The system can analyze memory for task similarity patterns:

```go
metrics, _ := evolvingMem.ComputeTaskSimilarityMetrics(ctx)
// Returns:
// - AverageSimilarity: Mean pairwise similarity (0.0-1.0)
// - ClusterRatio: Higher = more similar/clustered tasks
// - DomainDistribution: Count by domain
// - TypeDistribution: Count by memory type
// - PruningRecommendation: Actionable advice
```

Higher task similarity correlates with better memory effectiveness, as similar tasks benefit more from experience reuse.

## Integration

The memory system integrates seamlessly with the existing agent engine and **automatically uses your existing embedding service configuration**:

```go
import "manifold/internal/agent/memory"

// Initialize evolving memory - uses cfg.Embedding from your .env file
memCfg := memory.EvolvingMemoryConfig{
    EmbeddingConfig:  cfg.Embedding,  // Reuses EMBED_* environment variables
    LLM:              llmProvider,
    Model:            cfg.EvolvingMemory.Model,
    MaxSize:          cfg.EvolvingMemory.MaxSize,
    TopK:             cfg.EvolvingMemory.TopK,
    WindowSize:       cfg.EvolvingMemory.WindowSize,
    EnableRAG:        cfg.EvolvingMemory.EnableRAG,
    // Smart pruning options (programmatic)
    EnableSmartPrune: true,
    PruneThreshold:   0.95,
    RelevanceDecay:   0.99,
    MinRelevance:     0.1,
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
- Access metrics are tracked automatically for relevance scoring

## ReMem Controller Usage

For advanced Think-Act-Refine mode with strategy card generation:

```go
// Initialize ReMem controller
reMemCtrl := memory.NewReMemController(memory.ReMemConfig{
    Memory:        evolvingMem,
    LLM:           llmProvider,
    Model:         "gpt-4",
    MaxInnerSteps: 5,
})

// Execute with Think-Act-Refine loop
result, trace, err := reMemCtrl.Execute(ctx, task, tools)

// Store experience with structured feedback and strategy card
reMemCtrl.StoreExperienceEnhanced(ctx, task, result, "success", &memory.StructuredFeedback{
    Type:         memory.FeedbackSuccess,
    Correct:      true,
    ProgressRate: 1.0,
    StepsUsed:    3,
    StepsOptimal: 3,
    Message:      "Task completed efficiently",
}, trace)
```

## Filtering by Memory Type

Retrieve memories by type for targeted recall:

```go
// Get only procedural memories (strategies, how-to)
procedural := evolvingMem.GetProceduralMemories()

// Get only factual memories (facts, data)
factual := evolvingMem.GetFactualMemories()

// Search filtered by type
results, _ := evolvingMem.SearchByType(ctx, "solve equation", memory.MemoryProcedural)
```

## Memory Statistics

Get aggregate statistics about the memory store:

```go
stats := evolvingMem.GetMemoryStats()
// Returns map with:
// - total_entries: int
// - max_size: int
// - type_distribution: map[MemoryType]int
// - total_accesses: int
// - avg_accesses_per_entry: float64
// - entries_with_strategy_card: int
// - entries_with_structured_feedback: int
```

## Benefits

Based on the paper's empirical results:

1. **Improved Success Rate**: 15-25% higher success on multi-turn tasks (Table 2)
2. **Reduced Steps**: Fewer iterations needed to solve tasks (Figure 5)
3. **Better Generalization**: Learns patterns from past experiences
4. **Self-Improvement**: Memory quality evolves over time
5. **Task Similarity Correlation**: Higher gains on similar task clusters (Figure 4)

## Performance Considerations

1. **Embedding Latency**: Each memory operation requires embedding calls
   - Uses your existing embedding service (same as pgvector/vector search)
   - Embedding dimensions are determined by your configured model
   - Access tracking is async to not block searches

2. **Memory Size**: Keep maxSize reasonable (100-1000 entries)
   - Larger memory = slower retrieval (O(n) similarity comparison)
   - Smart pruning helps maintain quality over quantity
   - Relevance-based pruning keeps frequently-used memories

3. **Vector Dimensions**: The system is dimension-agnostic
   - Works with any embedding dimension (384, 768, 1024, 1536, etc.)
   - Evolving memory stores embeddings as `[]float32` of variable length

4. **ReMem Overhead**: Think-Act-Refine adds 2-5 extra LLM calls per task
   - Use for complex reasoning tasks only
   - Start with ExpRAG, upgrade to ReMem if needed
   - Strategy card generation adds one additional LLM call

## Example Workflow

1. User submits task
2. Agent searches memory for similar past experiences (top-k=4)
3. Access counts updated for retrieved memories (async)
4. Memory type classification applied (factual/procedural/episodic)
5. Synthesizes context from retrieved memories with strategy cards
6. LLM generates response using augmented context
7. Agent stores new experience with:
   - LLM-generated summary
   - Strategy card (if ReMem enabled)
   - Structured feedback (if provided)
   - Automatic memory type classification
8. Smart pruning: near-duplicates merged, low-relevance entries removed

## Debugging

Enable detailed logging:

```yaml
logLevel: "debug"
```

Look for these log messages:
- `evolving_memory_search`: Retrieval operation with candidate count
- `evolving_memory_entry_added`: New experience stored (includes memory_type, has_strategy_card)
- `evolving_memory_smart_merged`: Duplicate detection merged entries
- `evolving_memory_relevance_pruned`: Entries removed by relevance scoring
- `evolving_memory_fifo_pruned`: Fallback FIFO pruning when smart prune disabled
- `remem_action`: ReMem controller action (THINK/REFINE_MEMORY/ACT)
- `remem_think`: Internal reasoning trace
- `remem_refine`: Memory edit operations applied

## API Reference

### EvolvingMemory Methods

| Method | Description |
|--------|-------------|
| `Search(ctx, query)` | Retrieve top-k similar memories |
| `SearchWithScores(ctx, query)` | Search with similarity scores |
| `SearchByType(ctx, query, type)` | Search filtered by memory type |
| `Synthesize(ctx, task, memories)` | Build context from memories |
| `Evolve(ctx, input, output, feedback)` | Store new experience (basic) |
| `EvolveEnhanced(ctx, ...)` | Store with structured feedback & strategy |
| `GetRecentWindow()` | Get ExpRecent sliding window |
| `GetProceduralMemories()` | Filter procedural memories |
| `GetFactualMemories()` | Filter factual memories |
| `ComputeTaskSimilarityMetrics(ctx)` | Analyze task similarity |
| `GetMemoryStats()` | Get aggregate statistics |
| `ApplyEdits(ctx, ops)` | Apply PRUNE/MERGE/UPDATE_TAG |
| `ExportMemories()` | Export all entries |
| `ImportMemories(entries)` | Import entries |

### ReMemController Methods

| Method | Description |
|--------|-------------|
| `Execute(ctx, task, tools)` | Run Think-Act-Refine loop |
| `StoreExperience(ctx, ...)` | Store with basic feedback |
| `StoreExperienceEnhanced(ctx, ...)` | Store with structured feedback |

## Persistence

Memory entries can be persisted via the `EvolvingMemoryStore` interface:

```go
type EvolvingMemoryStore interface {
    Load(ctx context.Context, userID int64) ([]*MemoryEntry, error)
    Save(ctx context.Context, userID int64, entries []*MemoryEntry) error
}
```

A PostgreSQL implementation is available in `internal/persistence/databases/evolving_memory_store_postgres.go`.

## References

- Paper: [Evo-Memory: Benchmarking LLM Agent Test-time Learning with Self-Evolving Memory](https://arxiv.org/abs/2511.20857)
- Implementation follows the paper's Search → Synthesis → Evolve pattern
- ExpRAG and ReMem baselines from Section 3.2 and 3.3
- Memory type distinction from paper's "conversational recall vs experience reuse" (Figure 1)
