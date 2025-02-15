// internal/sefii/engine.go
package sefii

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"manifold/internal/documents"
	"manifold/internal/embeddings" // Updated import for embedding functions

	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
)

// Chunk represents a text chunk with associated metadata.
type Chunk struct {
	ID       int64  `json:"id"`
	Content  string `json:"content"`
	FilePath string `json:"file_path"`
	// Future extensions: commit IDs, line numbers, etc.
}

// InvertedIndex is a simple in-memory mapping from token → set of chunk IDs.
type InvertedIndex struct {
	Index map[string]map[int64]bool // token → set of chunk IDs
	Mutex sync.RWMutex
}

// NewInvertedIndex creates a new inverted index.
func NewInvertedIndex() *InvertedIndex {
	return &InvertedIndex{
		Index: make(map[string]map[int64]bool),
	}
}

// Add inserts a token mapping for a given chunk ID.
func (ii *InvertedIndex) Add(token string, chunkID int64) {
	ii.Mutex.Lock()
	defer ii.Mutex.Unlock()
	if _, exists := ii.Index[token]; !exists {
		ii.Index[token] = make(map[int64]bool)
	}
	ii.Index[token][chunkID] = true
}

// Get returns all chunk IDs associated with a given token.
func (ii *InvertedIndex) Get(token string) []int64 {
	ii.Mutex.RLock()
	defer ii.Mutex.RUnlock()
	var ids []int64
	for id := range ii.Index[token] {
		ids = append(ids, id)
	}
	return ids
}

// Engine is the SEFII engine that handles ingestion and search.
type Engine struct {
	DB            *pgx.Conn
	InvertedIndex *InvertedIndex
}

// NewEngine returns a new SEFII engine.
func NewEngine(db *pgx.Conn) *Engine {
	return &Engine{
		DB:            db,
		InvertedIndex: NewInvertedIndex(),
	}
}

// EnsureTable checks if the "documents" table exists, and if not, creates it.
// If the table exists but is missing the "file_path" column, it alters the table to add it.
func (e *Engine) EnsureTable(ctx context.Context) error {
	var tableName *string
	// Check if the table exists using PostgreSQL's to_regclass.
	err := e.DB.QueryRow(ctx, "SELECT to_regclass('public.documents')").Scan(&tableName)
	if err != nil {
		return fmt.Errorf("failed to check for documents table: %w", err)
	}

	if tableName == nil || *tableName == "" {
		// Table doesn't exist; create it.
		_, err := e.DB.Exec(ctx, `
			CREATE TABLE documents (
				id SERIAL PRIMARY KEY,
				content TEXT NOT NULL,
				embedding vector(768) NOT NULL,
				file_path TEXT
			)
		`)
		if err != nil {
			return fmt.Errorf("failed to create documents table: %w", err)
		}
		log.Println("Created table 'documents'")
	} else {
		log.Println("Table 'documents' already exists")
		// Check if the file_path column exists.
		var columnName string
		err = e.DB.QueryRow(ctx, `
			SELECT column_name 
			FROM information_schema.columns 
			WHERE table_name = 'documents' AND column_name = 'file_path'
		`).Scan(&columnName)
		if err == pgx.ErrNoRows {
			// The column doesn't exist, so alter the table to add it.
			_, err := e.DB.Exec(ctx, `ALTER TABLE documents ADD COLUMN file_path TEXT`)
			if err != nil {
				return fmt.Errorf("failed to add file_path column: %w", err)
			}
			log.Println("Added column 'file_path' to table 'documents'")
		} else if err != nil {
			return fmt.Errorf("failed to check for file_path column: %w", err)
		} else {
			log.Println("Column 'file_path' already exists")
		}
	}
	return nil
}

// EnsureInvertedIndexTable checks if the "inverted_index" table exists, and if not, creates it.
func (e *Engine) EnsureInvertedIndexTable(ctx context.Context) error {
	var tableName *string
	err := e.DB.QueryRow(ctx, "SELECT to_regclass('public.inverted_index')").Scan(&tableName)
	if err != nil {
		return fmt.Errorf("failed to check for inverted_index table: %w", err)
	}
	if tableName == nil || *tableName == "" {
		// Table doesn't exist; create it.
		_, err := e.DB.Exec(ctx, `
			CREATE TABLE inverted_index (
				token TEXT NOT NULL,
				chunk_id BIGINT NOT NULL,
				PRIMARY KEY (token, chunk_id)
			)
		`)
		if err != nil {
			return fmt.Errorf("failed to create inverted_index table: %w", err)
		}
		log.Println("Created table 'inverted_index'")
	} else {
		log.Println("Table 'inverted_index' already exists")
	}
	return nil
}

// persistTokenMapping saves a single token to chunk ID mapping into the inverted_index table.
func (e *Engine) persistTokenMapping(ctx context.Context, token string, chunkID int64) error {
	_, err := e.DB.Exec(ctx, `
		INSERT INTO inverted_index (token, chunk_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`, token, chunkID)
	return err
}

// IngestDocument splits the text into chunks, generates embeddings, saves them, and updates the inverted index.
// It also persists each token mapping into the inverted_index table.
func (e *Engine) IngestDocument(ctx context.Context, text, language, filePath, embeddingsHost, apiKey string, chunkSize, chunkOverlap int) error {
	// Ensure the required tables exist.
	if err := e.EnsureTable(ctx); err != nil {
		return err
	}
	if err := e.EnsureInvertedIndexTable(ctx); err != nil {
		return err
	}

	// Use the existing splitter from internal/documents.
	splitter, err := documents.FromLanguage(documents.Language(language))
	if err != nil {
		return err
	}
	if chunkSize > 0 {
		splitter.ChunkSize = chunkSize
	}
	if chunkOverlap > 0 {
		splitter.OverlapSize = chunkOverlap
	}
	chunksText := splitter.SplitText(text)
	log.Printf("SEFII: Document split into %d chunks", len(chunksText))

	// Generate embeddings using the new embeddings package.
	embeds, err := embeddings.GenerateEmbeddings(embeddingsHost, apiKey, chunksText)
	if err != nil {
		return err
	}
	if len(embeds) != len(chunksText) {
		return fmt.Errorf("embedding count mismatch: got %d embeddings for %d chunks", len(embeds), len(chunksText))
	}

	// Save each chunk into the database and update the inverted index.
	for i, chunkContent := range chunksText {
		var chunkID int64
		err := e.DB.QueryRow(ctx,
			`INSERT INTO documents (content, embedding, file_path) 
             VALUES ($1, $2, $3) RETURNING id`,
			chunkContent, pgvector.NewVector(embeds[i]), filePath).Scan(&chunkID)
		if err != nil {
			return err
		}

		// Tokenize the chunk and update the in-memory inverted index,
		// then persist each token mapping in the database.
		tokens := tokenize(chunkContent)
		for _, token := range tokens {
			normalizedToken := strings.ToLower(token)
			e.InvertedIndex.Add(normalizedToken, chunkID)
			if err := e.persistTokenMapping(ctx, normalizedToken, chunkID); err != nil {
				return fmt.Errorf("failed to persist token mapping for '%s': %w", normalizedToken, err)
			}
		}
	}
	return nil
}

// tokenize performs a basic whitespace tokenization.
func tokenize(text string) []string {
	return strings.Fields(text)
}

// SearchChunks performs a semantic search on stored chunks using the query embedding.
// It also applies an optional file path filter.
func (e *Engine) SearchChunks(ctx context.Context, query, filePathFilter string, limit int, embeddingsHost, apiKey string) ([]Chunk, error) {
	// Generate the query embedding.
	queryEmbeds, err := embeddings.GenerateEmbeddings(embeddingsHost, apiKey, []string{query})
	if err != nil {
		return nil, err
	}
	if len(queryEmbeds) == 0 {
		return nil, fmt.Errorf("failed to generate query embedding")
	}
	queryVec := pgvector.NewVector(queryEmbeds[0])

	// Build the SQL query.
	sqlQuery := "SELECT id, content, file_path FROM documents WHERE 1=1"
	var args []interface{}

	if filePathFilter != "" {
		sqlQuery += " AND file_path = $1"
		args = append(args, filePathFilter)
	}
	sqlQuery += " ORDER BY embedding <-> $2 LIMIT $3"
	args = append(args, queryVec, limit)

	rows, err := e.DB.Query(ctx, sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Chunk
	for rows.Next() {
		var c Chunk
		if err := rows.Scan(&c.ID, &c.Content, &c.FilePath); err != nil {
			return nil, err
		}
		results = append(results, c)
	}

	return results, nil
}
