# `/api/sefii/combined-retrieve` Endpoint Documentation

## Overview

The `/api/sefii/combined-retrieve` endpoint is designed for a flexible Retrieval Augmented Generation (RAG) pipeline. It integrates both semantic (vector-based) search and keyword-based (inverted index) search, allowing users to merge results based on their desired strategy. In addition, the endpoint can either return individual text chunks with metadata or reassemble full documents (grouped by file path) from the retrieved chunks.

## HTTP Request

```
POST /api/sefii/combined-retrieve HTTP/1.1
Host: <your-server-host>
Content-Type: application/json
```

## Request Body

The request body must be a JSON object with the following properties:

| Property              | Type    | Required | Default  | Description |
|-----------------------|---------|----------|----------|-------------|
| `query`               | string  | Yes      | N/A      | The search query text. |
| `file_path_filter`    | string  | No       | `""`     | Optional filter to restrict results to documents whose file path matches the provided pattern. |
| `limit`               | number  | No       | `10`     | Maximum number of results (chunks or documents) to return. |
| `use_inverted_index`  | boolean | No       | `false`  | Enable keyword-based search using the inverted index. |
| `use_vector_search`   | boolean | No       | `false`  | Enable semantic search using vector embeddings. |
| `merge_mode`          | string  | No       | `"union"`| Merge strategy to combine results from vector and inverted index searches. Acceptable values: `"union"` or `"intersect"`. <br><br>**Union:** Returns any chunk that appears in either result set.<br>**Intersect:** Returns only those chunks that appear in both result sets. |
| `return_full_docs`    | boolean | No       | `false`  | If `true`, the endpoint will reassemble and return entire documents (grouped by file path) instead of individual chunks. |

### Example Request Body for Chunk-Level Retrieval

```json
{
  "query": "optimize query performance",
  "file_path_filter": "docs/",
  "limit": 5,
  "use_inverted_index": true,
  "use_vector_search": true,
  "merge_mode": "union",
  "return_full_docs": false
}
```

### Example Request Body for Full Document Retrieval

```json
{
  "query": "improve API latency",
  "file_path_filter": "",
  "limit": 3,
  "use_inverted_index": true,
  "use_vector_search": true,
  "merge_mode": "intersect",
  "return_full_docs": true
}
```

## Response

The response is returned as JSON. Its structure depends on the value of `return_full_docs`:

### When `return_full_docs` is `false` (Chunk-Level Results)

The response contains an array of individual chunks. Each chunk includes:
- `id`: Unique identifier of the chunk.
- `content`: The text snippet extracted from the document.
- `file_path`: The source file path of the chunk.

#### Example Response

```json
{
  "chunks": [
    {
      "id": 21556,
      "content": "snippet of text extracted from the document...",
      "file_path": "frontend/src/components/WebSearchNode.vue"
    },
    {
      "id": 21558,
      "content": "another snippet from a different file...",
      "file_path": "frontend/src/components/WebSearchNode.vue"
    }
    // ... additional chunks
  ]
}
```

### When `return_full_docs` is `true` (Full Document Retrieval)

The response reassembles entire documents by grouping chunks by their `file_path`. The response is a JSON object where each key is a file path and the corresponding value is the full concatenated document content.

#### Example Response

```json
{
  "documents": {
    "frontend/src/components/WebSearchNode.vue": "Full document content reassembled from all chunks...\n\n",
    "frontend/src/components/DatadogNode.vue": "Full document content reassembled from all chunks...\n\n"
  }
}
```

## Error Responses

If an error occurs, the endpoint will return a JSON object containing an `error` field with a descriptive message, along with an appropriate HTTP status code (e.g., 400 for bad request, 500 for server error).

#### Example Error Response

```json
{
  "error": "Failed to connect to database"
}
```

## Usage Notes

- **Hybrid Retrieval:**  
  Enable both `use_vector_search` and `use_inverted_index` to combine semantic and keyword-based searches. Use `merge_mode` to control the merging:
  - **Union:** Broader recall by returning any chunk from either search.
  - **Intersect:** More precise results by returning only chunks present in both searches.

- **File Path Filtering:**  
  The `file_path_filter` parameter is optional and, if provided, restricts the search to documents with matching file paths.

- **Full Document Reconstruction:**  
  When `return_full_docs` is `true`, the endpoint queries all chunks for each unique file path from the selected results and concatenates them to reconstruct the full document. Ensure that documents are ingested with consistent `file_path` metadata for accurate reassembly.

- **Performance Considerations:**  
  Retrieving full documents may lead to larger payloads. Consider appropriate limits and pagination for production use.

## Example curl Commands

### 1. Retrieve Chunk-Level Results (Using Union Merge Mode)

```bash
curl -X POST http://localhost:8080/api/sefii/combined-retrieve \
  -H "Content-Type: application/json" \
  -d '{
        "query": "optimize query performance",
        "file_path_filter": "docs/",
        "limit": 5,
        "use_inverted_index": true,
        "use_vector_search": true,
        "merge_mode": "union",
        "return_full_docs": false
      }'
```

### 2. Retrieve Full Documents (Using Intersect Merge Mode)

```bash
curl -X POST http://localhost:8080/api/sefii/combined-retrieve \
  -H "Content-Type: application/json" \
  -d '{
        "query": "improve API latency",
        "file_path_filter": "",
        "limit": 3,
        "use_inverted_index": true,
        "use_vector_search": true,
        "merge_mode": "intersect",
        "return_full_docs": true
      }'
```

## Conclusion

The `/api/sefii/combined-retrieve` endpoint offers a robust and flexible mechanism for retrieving semantically relevant content from ingested documents. By supporting both vector-based and keyword-based retrieval methods, customizable merge strategies, and full document reassembly, it serves as a key component in building an advanced RAG pipeline.