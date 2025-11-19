package ingest

import (
	"context"

	"manifold/internal/persistence/databases"
)

const (
	labelDoc         = "Doc"
	labelChunk       = "Chunk"
	labelExternalRef = "ExternalRef"
	relHasChunk      = "HAS_CHUNK"
	relRefersTo      = "REFERS_TO"
)

// UpsertDocAndChunksGraph upserts the Doc node, all Chunk nodes, and HAS_CHUNK edges.
// It returns the list of chunk IDs created (same order as chunks slice).
func UpsertDocAndChunksGraph(ctx context.Context, g databases.GraphDB, docID string, pre PreprocessedDoc, in IngestRequest, chunks []ChunkRecord, version int) ([]string, error) {
	if g == nil {
		return nil, nil
	}

	// Upsert Doc node with basic props aligned to the data model.
	dprops := map[string]any{
		"title":    in.Title,
		"url":      in.URL,
		"source":   in.Source,
		"tenant":   in.Tenant,
		"lang":     pre.Language,
		"doc_hash": pre.Hash,
		"version":  version,
	}
	// Attach provided metadata/ACL if any for future policy/filters.
	if in.Metadata != nil {
		dprops["metadata"] = in.Metadata
	}
	if in.ACL != nil {
		dprops["acl"] = in.ACL
	}
	if err := g.UpsertNode(ctx, docID, []string{labelDoc}, dprops); err != nil {
		return nil, err
	}

	// Upsert Chunk nodes and HAS_CHUNK edges in order.
	ids := make([]string, len(chunks))
	for i, c := range chunks {
		cid := chunkID(docID, c.Index)
		ids[i] = cid
		cprops := map[string]any{
			"doc_id":  docID,
			"idx":     c.Index,
			"tenant":  in.Tenant,
			"lang":    pre.Language,
			"version": version,
		}
		if in.Source != "" {
			cprops["source"] = in.Source
		}
		if in.URL != "" {
			cprops["url"] = in.URL
		}
		if err := g.UpsertNode(ctx, cid, []string{labelChunk}, cprops); err != nil {
			return ids[:i], err
		}
		// Edge carries idx for convenience; UpsertEdge is idempotent when backed by a unique constraint.
		eprops := map[string]any{"idx": c.Index}
		if err := g.UpsertEdge(ctx, docID, relHasChunk, cid, eprops); err != nil {
			return ids[:i+1], err
		}
	}

	// Optional: external references as nodes with REFERS_TO edges from Doc.
	if in.Options.Graph.ExternalRefs != nil {
		for src, key := range in.Options.Graph.ExternalRefs {
			refID := "ref:" + src + ":" + key
			rprops := map[string]any{"source": src, "key": key}
			// If key looks like a URL and URL is empty, also set url.
			if in.URL == "" {
				// simple heuristic; keep it minimal
				if len(key) > 8 && (key[:7] == "http://" || key[:8] == "https://") {
					rprops["url"] = key
				}
			}
			if err := g.UpsertNode(ctx, refID, []string{labelExternalRef}, rprops); err != nil {
				return ids, err
			}
			if err := g.UpsertEdge(ctx, docID, relRefersTo, refID, nil); err != nil {
				return ids, err
			}
		}
	}
	return ids, nil
}

// Entity and link extraction scaffolding (no-op defaults)

// Entity represents a detected named-entity mention.
type Entity struct {
	ID    string
	Type  string
	Value string
	Meta  map[string]any
}

// EntityExtractor extracts entities from text.
type EntityExtractor interface {
	Extract(ctx context.Context, text, lang string) ([]Entity, error)
}

// Link represents an external reference discovered in text.
type Link struct {
	Source string
	Key    string
	URL    string
	Meta   map[string]any
}

// LinkExtractor extracts external references from text.
type LinkExtractor interface {
	ExtractLinks(ctx context.Context, text string) ([]Link, error)
}
