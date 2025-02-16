# Merge Modes in SearchRelevantChunks

When searching for relevant chunks, two different search methods can be used simultaneously:

## Vector Search
Uses semantic similarity (via embeddings) to rank chunks based on how close their embeddings are to the query embedding.

## Inverted Index Search
Uses a simple token-based lookup (using an in-memory or persisted inverted index) to find chunks that contain any of the query tokens.

The `mergeMode` parameter determines how the results of these two methods are combined:

### Union Mode (`merge_mode = "union"`)
**What it does:**
The union mode gathers all chunk IDs that are returned by either the vector search or the inverted index search. In other words, a chunk is included in the final results if it appears in either of the individual search result sets.

**Pros:**
- Maximizes recall by returning any chunk that is deemed relevant by either method
- Useful when you want a broad set of results and you don't mind some extra noise

**Cons:**
- May return some less relevant results since a chunk only needs to pass one of the two criteria

### Intersect Mode (`merge_mode = "intersect"`)
**What it does:**
The intersect mode returns only those chunk IDs that appear in both the vector search results and the inverted index search results. A chunk must be retrieved by both search methods to be considered relevant.

**Pros:**
- Increases precision by ensuring that only chunks that match both the semantic and the token-based criteria are returned
- Helps filter out noise when you expect that truly relevant content will be captured by both methods

**Cons:**
- May yield fewer results (i.e., lower recall) because it is more restrictive