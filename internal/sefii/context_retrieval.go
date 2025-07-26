package sefii

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// ContextualChunk extends the basic Chunk with additional context information
type ContextualChunk struct {
	Chunk
	NeighborChunks []Chunk        `json:"neighbor_chunks,omitempty"`
	FullDocument   string         `json:"full_document,omitempty"`
	DocumentStats  *DocumentStats `json:"document_stats,omitempty"`
}

// DocumentStats provides information about the source document
type DocumentStats struct {
	TotalChunks   int    `json:"total_chunks"`
	FileSize      int    `json:"file_size_bytes"`
	Language      string `json:"language"`
	DocumentTitle string `json:"document_title"`
}

// RetrieveWithContext retrieves chunks with surrounding context
func (e *Engine) RetrieveWithContext(ctx context.Context, chunkIDs []int64, contextWindow int, includeFullDoc bool) ([]ContextualChunk, error) {
	var results []ContextualChunk

	for _, chunkID := range chunkIDs {
		// Get the main chunk
		mainChunk, err := e.getChunkByID(ctx, chunkID)
		if err != nil {
			continue
		}

		contextChunk := ContextualChunk{
			Chunk: *mainChunk,
		}

		// Get document stats
		stats, err := e.getDocumentStats(ctx, mainChunk.FilePath)
		if err == nil {
			contextChunk.DocumentStats = stats
		}

		// Get neighboring chunks if context window is specified
		if contextWindow > 0 {
			neighbors, err := e.getNeighboringChunks(ctx, chunkID, mainChunk.FilePath, contextWindow)
			if err == nil {
				contextChunk.NeighborChunks = neighbors
			}
		}

		// Get full document if requested
		if includeFullDoc {
			fullDoc, err := e.getFullDocument(ctx, mainChunk.FilePath)
			if err == nil {
				contextChunk.FullDocument = fullDoc
			}
		}

		results = append(results, contextChunk)
	}

	return results, nil
}

// getChunkByID retrieves a single chunk by its ID
func (e *Engine) getChunkByID(ctx context.Context, chunkID int64) (*Chunk, error) {
	var chunk Chunk
	var mdBytes []byte
	var summary *string

	err := e.DB.QueryRow(ctx, `
		SELECT id, content, summary, file_path, metadata 
		FROM documents 
		WHERE id = $1
	`, chunkID).Scan(&chunk.ID, &chunk.Content, &summary, &chunk.FilePath, &mdBytes)

	if err != nil {
		return nil, fmt.Errorf("failed to retrieve chunk %d: %w", chunkID, err)
	}

	if summary != nil {
		chunk.Summary = *summary
	}

	if len(mdBytes) > 0 {
		meta := make(map[string]string)
		_ = json.Unmarshal(mdBytes, &meta)
		chunk.Metadata = meta
	}

	return &chunk, nil
}

// getNeighboringChunks retrieves chunks that are adjacent to the given chunk in the same document
func (e *Engine) getNeighboringChunks(ctx context.Context, chunkID int64, filePath string, contextWindow int) ([]Chunk, error) {
	// Get the chunk index of the current chunk
	mainChunk, err := e.getChunkByID(ctx, chunkID)
	if err != nil {
		return nil, err
	}

	currentIndex := -1
	if mainChunk.Metadata != nil {
		if idxStr, exists := mainChunk.Metadata["chunkIndex"]; exists {
			if idx, err := strconv.Atoi(idxStr); err == nil {
				currentIndex = idx
			}
		}
	}

	if currentIndex == -1 {
		// Fallback: get chunks by ID proximity (assuming sequential insertion)
		return e.getChunksByIDProximity(ctx, chunkID, filePath, contextWindow)
	}

	// Calculate range of chunk indices to retrieve
	startIndex := currentIndex - contextWindow
	endIndex := currentIndex + contextWindow
	if startIndex < 0 {
		startIndex = 0
	}

	// Build query to get chunks by index range
	rows, err := e.DB.Query(ctx, `
		SELECT id, content, summary, file_path, metadata
		FROM documents 
		WHERE file_path = $1 
		  AND metadata->>'chunkIndex' IS NOT NULL
		  AND CAST(metadata->>'chunkIndex' AS INTEGER) BETWEEN $2 AND $3
		  AND id != $4
		ORDER BY CAST(metadata->>'chunkIndex' AS INTEGER)
	`, filePath, startIndex, endIndex, chunkID)

	if err != nil {
		return nil, fmt.Errorf("failed to retrieve neighboring chunks: %w", err)
	}
	defer rows.Close()

	var neighbors []Chunk
	for rows.Next() {
		var chunk Chunk
		var mdBytes []byte
		var summary *string

		if err := rows.Scan(&chunk.ID, &chunk.Content, &summary, &chunk.FilePath, &mdBytes); err != nil {
			continue
		}

		if summary != nil {
			chunk.Summary = *summary
		}

		if len(mdBytes) > 0 {
			meta := make(map[string]string)
			_ = json.Unmarshal(mdBytes, &meta)
			chunk.Metadata = meta
		}

		neighbors = append(neighbors, chunk)
	}

	return neighbors, nil
}

// getChunksByIDProximity is a fallback method that uses chunk ID proximity when indices aren't available
func (e *Engine) getChunksByIDProximity(ctx context.Context, chunkID int64, filePath string, contextWindow int) ([]Chunk, error) {
	rows, err := e.DB.Query(ctx, `
		SELECT id, content, summary, file_path, metadata
		FROM documents 
		WHERE file_path = $1 
		  AND id BETWEEN $2 AND $3
		  AND id != $4
		ORDER BY id
	`, filePath, chunkID-int64(contextWindow), chunkID+int64(contextWindow), chunkID)

	if err != nil {
		return nil, fmt.Errorf("failed to retrieve chunks by ID proximity: %w", err)
	}
	defer rows.Close()

	var neighbors []Chunk
	for rows.Next() {
		var chunk Chunk
		var mdBytes []byte
		var summary *string

		if err := rows.Scan(&chunk.ID, &chunk.Content, &summary, &chunk.FilePath, &mdBytes); err != nil {
			continue
		}

		if summary != nil {
			chunk.Summary = *summary
		}

		if len(mdBytes) > 0 {
			meta := make(map[string]string)
			_ = json.Unmarshal(mdBytes, &meta)
			chunk.Metadata = meta
		}

		neighbors = append(neighbors, chunk)
	}

	return neighbors, nil
}

// getDocumentStats retrieves statistics about the source document
func (e *Engine) getDocumentStats(ctx context.Context, filePath string) (*DocumentStats, error) {
	// Get basic stats from chunks table
	var totalChunks int
	var language, docTitle string

	err := e.DB.QueryRow(ctx, `
		SELECT 
			COUNT(*) as total_chunks,
			COALESCE(metadata->>'language', 'unknown') as language,
			COALESCE(metadata->>'docTitle', '') as doc_title
		FROM documents 
		WHERE file_path = $1
		GROUP BY metadata->>'language', metadata->>'docTitle'
		LIMIT 1
	`, filePath).Scan(&totalChunks, &language, &docTitle)

	if err != nil {
		return nil, fmt.Errorf("failed to get document stats: %w", err)
	}

	// Get additional metadata if available
	var mdBytes []byte
	err = e.DB.QueryRow(ctx, `
		SELECT metadata 
		FROM document_metadata 
		WHERE file_path = $1
	`, filePath).Scan(&mdBytes)

	stats := &DocumentStats{
		TotalChunks:   totalChunks,
		Language:      language,
		DocumentTitle: docTitle,
	}

	// If we have document metadata, extract additional information
	if err == nil && len(mdBytes) > 0 {
		var docMeta map[string]interface{}
		if json.Unmarshal(mdBytes, &docMeta) == nil {
			if title, ok := docMeta["docTitle"].(string); ok && title != "" {
				stats.DocumentTitle = title
			}
			if lang, ok := docMeta["language"].(string); ok && lang != "" {
				stats.Language = lang
			}
		}
	}

	return stats, nil
}

// getFullDocument reconstructs the complete document from all its chunks
func (e *Engine) getFullDocument(ctx context.Context, filePath string) (string, error) {
	rows, err := e.DB.Query(ctx, `
		SELECT content, metadata
		FROM documents 
		WHERE file_path = $1 
		ORDER BY CASE 
			WHEN metadata->>'chunkIndex' IS NOT NULL 
			THEN CAST(metadata->>'chunkIndex' AS INTEGER) 
			ELSE id 
		END
	`, filePath)

	if err != nil {
		return "", fmt.Errorf("failed to retrieve document chunks: %w", err)
	}
	defer rows.Close()

	var builder strings.Builder
	for rows.Next() {
		var content string
		var mdBytes []byte

		if err := rows.Scan(&content, &mdBytes); err != nil {
			continue
		}

		// Remove embedding prefix if present
		content = strings.TrimPrefix(content, "search_document: ")

		builder.WriteString(content)
		if !strings.HasSuffix(content, "\n") {
			builder.WriteString("\n")
		}
	}

	return builder.String(), nil
}

// SearchWithExpandedContext performs search and automatically includes relevant context
func (e *Engine) SearchWithExpandedContext(ctx context.Context, query, filePathFilter string, limit int, contextWindow int, includeFullDoc bool, embeddingsHost, apiKey string) ([]ContextualChunk, error) {
	// First, perform the regular search
	chunks, err := e.SearchChunks(ctx, query, filePathFilter, limit, embeddingsHost, apiKey)
	if err != nil {
		return nil, err
	}

	// Extract chunk IDs
	var chunkIDs []int64
	for _, chunk := range chunks {
		chunkIDs = append(chunkIDs, chunk.ID)
	}

	// Get chunks with context
	return e.RetrieveWithContext(ctx, chunkIDs, contextWindow, includeFullDoc)
}
