package ingest

import (
	"context"
	"fmt"
	"strings"

	"manifold/internal/persistence/databases"
)

// chunkTableChecker is an optional capability of a FullTextSearch backend.
type chunkTableChecker interface {
	HasChunksTable(ctx context.Context) (bool, error)
}

// chunkUpserter is an optional capability of a FullTextSearch backend.
type chunkUpserter interface {
	UpsertChunk(ctx context.Context, chunkID, docID string, idx int, text string, metadata map[string]string, lang string) error
}

// UpsertDocumentToSearch writes/overwrites the document row in the FTS backend.
// Metadata is flattened to strings for compatibility with databases.FullTextSearch.
func UpsertDocumentToSearch(ctx context.Context, s databases.FullTextSearch, docID string, in IngestRequest, pre PreprocessedDoc, version int) error {
	md := flattenMetadata(in.Metadata)
	// mandatory fields for observability and filtering
	md["type"] = "doc"
	if in.Title != "" {
		md["title"] = in.Title
	}
	if in.URL != "" {
		md["url"] = in.URL
	}
	if in.Source != "" {
		md["source"] = in.Source
	}
	if in.Tenant != "" {
		md["tenant"] = in.Tenant
	}
	if pre.Language != "" {
		md["lang"] = pre.Language
	}
	if pre.Hash != "" {
		md["doc_hash"] = pre.Hash
	}
	if version > 0 {
		md["version"] = fmt.Sprintf("%d", version)
	}
	return s.Index(ctx, docID, pre.Text, md)
}

// ChunkRecord is a minimal representation of a chunk used for indexing.
type ChunkRecord struct {
	Index int
	Text  string
}

// UpsertChunksToSearch persists chunks. When the backend exposes a real chunks
// table, it is used; otherwise it falls back to separate documents with id prefix
// "chunk:" and metadata.type="chunk".
func UpsertChunksToSearch(ctx context.Context, s databases.FullTextSearch, docID string, lang string, chunks []ChunkRecord, in IngestRequest, version int) ([]string, error) {
	// Determine capability
	hasTable := false
	if chk, ok := s.(chunkTableChecker); ok {
		exists, err := chk.HasChunksTable(ctx)
		if err != nil {
			return nil, err
		}
		hasTable = exists
	}

	ids := make([]string, 0, len(chunks))
	if hasTable {
		up, ok := s.(chunkUpserter)
		if !ok {
			// Should not happen: table exists but backend cannot upsert; fall back
			hasTable = false
		} else {
			md := baseChunkMetadata(in, version)
			for _, c := range chunks {
				chunkID := fmt.Sprintf("chunk:%s:%d", docID, c.Index)
				if err := up.UpsertChunk(ctx, chunkID, docID, c.Index, c.Text, md, lang); err != nil {
					return nil, err
				}
				ids = append(ids, chunkID)
			}
			return ids, nil
		}
	}

	// Fallback: index chunks as individual documents
	md := baseChunkMetadata(in, version)
	md["lang"] = lang
	for _, c := range chunks {
		chunkID := fmt.Sprintf("chunk:%s:%d", docID, c.Index)
		if err := s.Index(ctx, chunkID, c.Text, md); err != nil {
			return nil, err
		}
		ids = append(ids, chunkID)
	}
	return ids, nil
}

func baseChunkMetadata(in IngestRequest, version int) map[string]string {
	md := flattenMetadata(in.Metadata)
	md["type"] = "chunk"
	if in.Source != "" {
		md["source"] = in.Source
	}
	if in.Tenant != "" {
		md["tenant"] = in.Tenant
	}
	if version > 0 {
		md["version"] = fmt.Sprintf("%d", version)
	}
	if in.ID != "" {
		md["doc_id"] = in.ID
	}
	if in.URL != "" {
		md["url"] = in.URL
	}
	return md
}

// flattenMetadata converts map[string]any into map[string]string by formatting
// scalars; non-scalar values are JSON-like stringified via fmt.%v.
func flattenMetadata(in map[string]any) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		switch t := v.(type) {
		case string:
			out[k] = t
		case fmt.Stringer:
			out[k] = t.String()
		case int, int32, int64, uint, uint32, uint64, float32, float64, bool:
			out[k] = fmt.Sprintf("%v", t)
		default:
			out[k] = fmt.Sprintf("%v", t)
		}
	}
	// Ensure keys are safe
	cleaned := make(map[string]string, len(out))
	for k, v := range out {
		cleaned[strings.ToLower(k)] = v
	}
	return cleaned
}
