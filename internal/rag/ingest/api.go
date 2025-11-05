package ingest

import "time"

// IngestRequest describes a single document ingestion operation.
// The service is responsible for chunking, indexing into FTS/vector stores,
// and attaching graph relationships according to options.
type IngestRequest struct {
	// ID is the unified document ID (e.g., doc:<namespace>:<slug|hash>).
	ID string
	// Title is an optional document title for display and ranking features.
	Title string
	// URL is an optional canonical location for the document.
	URL string
	// Source describes where the document came from (e.g., github, web, file).
	Source string
	// Text is the raw, full document content to be chunked.
	Text string
	// Metadata holds arbitrary key/value metadata. Values should be JSON-serializable.
	Metadata map[string]any
	// Language preferred tokenizer configuration (e.g., "english"). If empty, auto-detect or default.
	Language string
	// Tenant for multi-tenant isolation. When empty, defaults are applied by the service.
	Tenant string
	// ACL is an optional access-control payload to apply consistently across stores.
	ACL map[string]any
	// Options drives how the ingestion should behave.
	Options IngestOptions
}

// IngestOptions controls chunking, embeddings, and graph handling.
type IngestOptions struct {
	// Chunking controls how the input text is split into chunks.
	Chunking ChunkingOptions
	// Embedding controls whether/how to generate and store embeddings.
	Embedding EmbeddingOptions
	// Graph controls whether/how to upsert nodes and edges.
	Graph GraphOptions
	// ReingestPolicy determines behavior when the document already exists.
	ReingestPolicy ReingestPolicy
	// Version allows callers to set or bump a document version explicitly.
	Version int
	// IdempotencyKey allows callers to de-duplicate repeated ingestion attempts.
	IdempotencyKey string
}

// ChunkingOptions describes the chunking strategy.
type ChunkingOptions struct {
	// Strategy name (e.g., "tokens", "sentences", "markdown").
	Strategy string
	// MaxTokens per chunk (semantic; implementation may map to characters when tokenization is unavailable).
	MaxTokens int
	// Overlap tokens between sequential chunks.
	Overlap int
}

// EmbeddingOptions controls vector embedding generation.
type EmbeddingOptions struct {
	// Enabled toggles vector embedding upsert.
	Enabled bool
	// Model is a hint or identifier for the embedding model to use.
	Model string
	// Dimensions is optional; when zero, derive from configured backend.
	Dimensions int
}

// GraphOptions controls creation of Doc/Chunk/Entity/ExternalRef nodes and edges.
type GraphOptions struct {
	// Enabled toggles graph augmentation.
	Enabled bool
	// ExtractEntities toggles named-entity extraction to populate Entity nodes and MENTIONS edges.
	ExtractEntities bool
	// ExternalRefs optional external references to attach via REFERS_TO.
	ExternalRefs map[string]string
}

// ReingestPolicy determines how to handle existing documents.
type ReingestPolicy string

const (
	// ReingestSkipIfUnchanged skips re-index when doc_hash/metadata unchanged.
	ReingestSkipIfUnchanged ReingestPolicy = "skip_if_unchanged"
	// ReingestOverwrite overwrites existing chunks/embeddings in-place.
	ReingestOverwrite ReingestPolicy = "overwrite"
	// ReingestNewVersion creates a new logical version and rewires VERSION_OF edges.
	ReingestNewVersion ReingestPolicy = "new_version"
)

// IngestResponse summarizes the mutation performed.
type IngestResponse struct {
	DocID    string
	Version  int
	ChunkIDs []string
	// Stats captures operational metrics for the ingestion.
	Stats IngestStats
	// Warnings captures non-fatal issues encountered.
	Warnings []string
}

// IngestStats captures ingestion-time statistics for observability and evaluation.
type IngestStats struct {
	NumChunks     int
	TotalTokens   int
	VectorUpserts int
	Duration      time.Duration
}
