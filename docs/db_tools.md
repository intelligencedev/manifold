# Using the local DB & Graph tools for research agents

This document explains how a research agent can use the local indexing, vector, and graph tools to ingest documents and web pages, organize them around a topic, and then iteratively construct a research paper while preserving provenance and supporting retrieval.

Target audience: implementers building agents that need to store, search, embed, and relate content (documents, web pages, notes, citations).

## Quick tool reference

- `search_index` — index a full-text document for keyword/full-text search.
- `search_query` — query the full-text index for matching documents.
- `search_remove` — remove a document from the full-text index.
- `vector_upsert` — upsert an embedding vector (or provide text to embed) with metadata.
- `vector_query` — nearest-neighbor search against stored vectors.
- `vector_delete` — delete a vector by id.
- `graph_upsert_node` — create or update a node in the knowledge graph.
- `graph_upsert_edge` — create or update a relationship between two nodes.
- `graph_neighbors` — list outbound neighbors for a node filtered by relationship type.
- `graph_get_node` — fetch a node by ID.

Refer to the tool reference for the exact parameter names. The examples below follow those parameter shapes.

## Contract (tiny)

- Inputs: raw text or web page content + metadata (title, url, authors, date, sourceId, topic).
- Outputs: stored search/doc IDs, vector IDs, graph node IDs, and query results (documents/vectors/neighbor lists).
- Error modes: embedding provider failure, duplicate IDs, oversized payload, rate-limit/timeout.

## Recommended data model and metadata

- Document node (graph): id = `doc:<sha1(url|title|date)>` or `doc:<uuid>`
  - labels: ["Document"]
  - props: { title, url, authors, fetched_at, source, language, content_hash }
- Topic node (graph): id = `topic:<normalized-topic-name>`
  - labels: ["Topic"]
  - props: { name, created_at }
- Section/Claim node: id = `claim:<uuid>`
   - labels: ["Claim","Section"]
   - props: { text, confidence, anchor_doc: `doc:<id>` }
- Edges
  - `ABOUT` (doc -> topic)
  - `CITES` (doc -> doc)
  - `CONTAINS` (doc -> claim)
  - `RELATED_TO` (claim -> claim or topic)

Keep canonical metadata fields for filtering (e.g., `source`, `fetched_at`, `url`, `content_hash`). Use short, consistent keys to make vector filters practical.

## Ingest pipeline (high level)

1. Discover/fetch web pages or documents (web crawler or browser tool).
2. Normalize metadata (title, url, authors, fetched_at, language). Compute content hash.
3. Chunk long content into passages (recommend ~200–600 tokens per chunk; overlap 20–50 tokens).
4. For each chunk:
   - `search_index` the chunk text with metadata (id: `search:<doc-id>:<chunk-index>`).
   - `vector_upsert` either supply `text` to be embedded by the configured provider or supply a precomputed `vector`. Use id: `vec:<doc-id>:<chunk-index>` and include metadata pointing to the doc node id and chunk offsets.
5. `graph_upsert_node` for the Document node and for the Topic node(s). Then `graph_upsert_edge` to link the document to its topic(s) with `ABOUT`, and to create `CITES` edges for discovered references.

This hybrid approach (full-text index + vector index + graph) gives a lot of flexibility: keyword search (search_query) for exact matches and metadata filters, semantic retrieval (vector_query) for meaning-based results, and structured relationships via the graph.

## Concrete examples (JSON-like payloads)

Index a chunk for full-text search:

```json
{ "id": "search:doc-123:0", "text": "<chunk text>", "metadata": { "doc_id": "doc-123", "url": "https://...", "title": "Example" } }
```

Upsert a vector (let the backend embed the text):

```json
{ "id": "vec:doc-123:0", "text": "<chunk text>", "metadata": { "doc_id": "doc-123", "chunk_index": 0, "url": "https://..." } }
```

Or upsert a precomputed vector:

```json
{ "id": "vec:doc-123:0", "vector": [0.001, -0.02, ...], "metadata": { "doc_id": "doc-123" } }
```

Query vectors (semantic retrieval):

```json
{ "vector": [0.1, -0.2, ...], "k": 10, "filter": { "doc_id": "doc-123" } }
```

Create a document node and link to a topic node:

```json
{ "id": "doc-123", "labels": ["Document"], "props": { "title":"...", "url":"...", "fetched_at":"2025-09-27T12:00:00Z" } }
```

```json
{ "src": "doc-123", "rel": "ABOUT", "dst": "topic:long-term-ai-safety", "props": { "confidence": 0.9 } }
```

Find neighbors (what a document says about a topic):

```json
{ "id": "topic:long-term-ai-safety", "rel": "ABOUT" }
```

Fetch a node:

```json
{ "id": "doc-123" }
```

Delete a vector or search entry when content is removed or replaced:

```json
// remove vector
{ "id": "vec:doc-123:0" }

// remove search index entry
{ "id": "search:doc-123:0" }
```

## Building a research paper (practical agent workflow)

This is a step-by-step pattern the agent can follow to construct a paper on a topic.

1. Topic bootstrap
   - Create a `Topic` node with `graph_upsert_node`.
   - Seed discovery: run web searches and feeds; fetch candidate pages.

2. Ingest & canonicalize
   - For each fetched page: normalize, chunk, `search_index` & `vector_upsert` chunks, create a `Document` graph node, and add `ABOUT` edge to the Topic.
   - Add provenance metadata: `source`, `fetched_at`, `url`, `content_hash`, and `agent_version`.

3. Map claims and evidence
   - Use an LLM to extract candidate claims and short summaries from chunks. For each claim:
     - Create a `Claim` node (`graph_upsert_node`).
     - Create `CONTAINS` edge from Document -> Claim with chunk location props.
     - Link claim -> topic with `RELATED_TO`.

4. Gather evidence
   - For each claim, run `vector_query` with the claim embedding to fetch top-k supporting chunks across the corpus.
   - Validate via `search_query` for exact phrase matches or quoted evidence.

5. Drafting sections
   - For each section, collect supporting chunks (vector_query + search_query) and neighbor claims (graph_neighbors).
   - Use `llm_transform` (or your LLM subsystem) to synthesize a section draft from the retrieved excerpts, including inline citations that reference `doc:<id>` nodes.

6. Cite and attribute
   - For every citation added, attach it as an edge `CITES` docA -> docB and record a `citation_text` prop.
   - Keep a citation index node (or a list in the paper node) so the agent can later render a bibliography.

7. Iterate, prune, and finalize
   - If a document is found to be retracted or superseded, use `search_remove` and `vector_delete` to remove its entries and mark the graph node with `deprecated=true`.
   - Run final retrieval passes to ensure each claim has supporting evidence; fail the claim or mark as speculative if support is weak.

8. Produce final artifacts
   - The agent can materialize the paper text and include a structured appendix that maps each inline citation to the `doc` node metadata (url, authors, fetched_at).

## Filtering and metadata strategies

- Use metadata filters in `vector_query` so you can restrict to peer-reviewed sources or to a date range: e.g., `filter: { "source": "arxiv", "year": "2023" }`.
- Keep a per-vector `doc_id` and `chunk_index` for quick lookup/provenance.
- Include `content_hash` on the Document node to detect duplicate or updated content. When content_hash changes, delete old vectors/search entries and re-ingest.

## Edge cases & best practices

- Long documents: always chunk. Avoid embedding > 8k token passages; split and keep chunk metadata.
- Deduping: compute a content hash and dedupe before indexing. If duplicates are acceptable, ensure metadata includes canonical doc id.
- Consistency: use deterministic IDs where appropriate (e.g., doc id derived from URL or canonical DOI) so updates are easy.
- Embedding model drift: store which embedding model and configuration was used as metadata so you can re-embed later and invalidate old vectors if needed.
- Security & privacy: never index secrets. Sanitize personally identifiable information according to policy.
- Deletion: when deleting by `vector_delete` and `search_remove`, also update the graph to mark node deprecated and keep a tombstone so provenance trails are intact.

## Example mini-workflow (summary)

1. fetch page -> compute id `doc:sha1(url)`
2. chunk -> for each chunk call `search_index` and `vector_upsert`
3. create `Document` node and `ABOUT` edge to `topic:...`
4. extract claims -> create `Claim` nodes and `CONTAINS` edges
5. for each claim call `vector_query` -> collect evidence -> synthesize section

## Closing notes

Using the three-layer approach (full-text search + vector embeddings + graph relationships) gives an agent strong retrieval and reasoning primitives: exact matches when needed, semantic similarity for relevance, and structured relationships for organization and provenance. The procedures above are intentionally small and composable — mix-and-match them to fit your agent's architecture, scale, and embedding provider.

If you want, I can also:

- Provide a small helper library sketch (Go or Node) that wraps these tools with the recommended schemas.
- Generate a sample ingestion script that fetches a single web page and runs the entire ingest pipeline.
