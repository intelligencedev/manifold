// internal/sefii/engine.go
package sefii

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

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

// Engine is the SEFII engine that handles ingestion and search.
type Engine struct {
	DB                  *pgx.Conn
	queryEmbeddingCache map[string][]float32
	cacheMutex          sync.RWMutex
}

// NewEngine returns a new SEFII engine.
func NewEngine(db *pgx.Conn) *Engine {
	return &Engine{
		DB:                  db,
		queryEmbeddingCache: make(map[string][]float32),
	}
}

// execWithRetry is a helper to execute a DB command with retries.
func (e *Engine) execWithRetry(ctx context.Context, sqlQuery string, args ...interface{}) error {
	var err error
	const maxRetries = 3
	for i := 0; i < maxRetries; i++ {
		_, err = e.DB.Exec(ctx, sqlQuery, args...)
		if err == nil {
			return nil
		}
		log.Printf("[ERROR] DB Exec failed (attempt %d/%d): %s", i+1, maxRetries, err)
		time.Sleep(time.Duration(i+1) * time.Second)
	}
	return fmt.Errorf("db exec failed after retries: %w", err)
}

// EnsureTable checks if the "documents" table exists, and if not, creates it.
// It also creates an IVFFlat index on the embedding column.
func (e *Engine) EnsureTable(ctx context.Context, embeddingVectorSize int) error {
	var tableName *string
	// Check if the table exists using PostgreSQL's to_regclass.
	err := e.DB.QueryRow(ctx, "SELECT to_regclass('public.documents')").Scan(&tableName)
	if err != nil {
		return fmt.Errorf("failed to check for documents table: %w", err)
	}

	createTableQuery := fmt.Sprintf(`
		CREATE TABLE documents (
			id SERIAL PRIMARY KEY,
			content TEXT NOT NULL,
			embedding vector(%d) NOT NULL,
			file_path TEXT
		)
	`, embeddingVectorSize)

	if tableName == nil || *tableName == "" {
		// Table doesn't exist; create it.
		if err := e.execWithRetry(ctx, createTableQuery); err != nil {
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
		if err == sql.ErrNoRows {
			// The column doesn't exist, so alter the table to add it.
			if err := e.execWithRetry(ctx, `ALTER TABLE documents ADD COLUMN file_path TEXT`); err != nil {
				return fmt.Errorf("failed to add file_path column: %w", err)
			}
			log.Println("Added column 'file_path' to table 'documents'")
		} else if err != nil {
			return fmt.Errorf("failed to check for file_path column: %w", err)
		} else {
			log.Println("Column 'file_path' already exists")
		}
	}

	// Create an IVFFlat index on the embedding column if it doesn't exist.
	indexQuery := `
		CREATE INDEX IF NOT EXISTS documents_embedding_idx 
		ON documents USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100)
	`
	if err := e.execWithRetry(ctx, indexQuery); err != nil {
		return fmt.Errorf("failed to create ivfflat index on documents.embedding: %w", err)
	}
	log.Println("Ensured IVFFlat index on documents.embedding")
	return nil
}

// EnsureInvertedIndexTable checks if the "inverted_index" table exists, and if not, creates it.
// This table uses a JSONB column to store an array of chunk IDs per token.
func (e *Engine) EnsureInvertedIndexTable(ctx context.Context) error {
	var tableName *string
	err := e.DB.QueryRow(ctx, "SELECT to_regclass('public.inverted_index')").Scan(&tableName)
	if err != nil {
		return fmt.Errorf("failed to check for inverted_index table: %w", err)
	}
	if tableName == nil || *tableName == "" {
		// Table doesn't exist; create it.
		createIndexQuery := `
			CREATE TABLE inverted_index (
				token TEXT PRIMARY KEY,
				chunk_ids JSONB NOT NULL
			)
		`
		if err := e.execWithRetry(ctx, createIndexQuery); err != nil {
			return fmt.Errorf("failed to create inverted_index table: %w", err)
		}
		log.Println("Created table 'inverted_index'")
	} else {
		log.Println("Table 'inverted_index' already exists")
	}
	return nil
}

// persistTokenMapping saves a token→chunk mapping into the inverted_index table using JSONB array update.
func (e *Engine) persistTokenMapping(ctx context.Context, token string, chunkID int64) error {
	// This query inserts a new row for the token or updates the existing JSONB array with the new chunkID,
	// ensuring that chunk IDs remain unique.
	query := `
		INSERT INTO inverted_index (token, chunk_ids)
		VALUES ($1, to_jsonb(ARRAY[$2]::BIGINT[]))
		ON CONFLICT (token) DO UPDATE SET
		chunk_ids = (
			SELECT to_jsonb(array_agg(DISTINCT cid))
			FROM (
				SELECT jsonb_array_elements_text(inverted_index.chunk_ids)::BIGINT as cid
				UNION
				SELECT $2
			) sub
		)
	`
	return e.execWithRetry(ctx, query, token, chunkID)
}

// tokenize performs improved tokenization: lowercases text, removes punctuation, and filters stopwords.
func tokenize(text string) []string {
	// Normalize and remove punctuation.
	text = strings.ToLower(text)
	// Remove punctuation.
	re := regexp.MustCompile(`[^\w\s]`)
	text = re.ReplaceAllString(text, "")
	words := strings.Fields(text)
	// Filter out common stopwords.
	stopwords := map[string]bool{
		"the": true, "is": true, "at": true, "of": true, "on": true, "and": true,
	}
	var tokens []string
	for _, word := range words {
		if !stopwords[word] {
			tokens = append(tokens, word)
		}
	}
	return tokens
}

// IngestDocument splits the text into chunks, generates embeddings, saves them, and updates the inverted index.
// In-memory inverted index has been removed; token mappings are stored only in PostgreSQL.
func (e *Engine) IngestDocument(ctx context.Context, text, language, filePath, embeddingsHost, apiKey string, chunkSize, chunkOverlap int) error {
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

	// Generate embeddings using the embeddings package.
	embeds, err := embeddings.GenerateEmbeddings(embeddingsHost, apiKey, chunksText)
	if err != nil {
		return err
	}
	if len(embeds) != len(chunksText) {
		return fmt.Errorf("embedding count mismatch: got %d embeddings for %d chunks", len(embeds), len(chunksText))
	}

	// Save each chunk into the database and persist token mappings.
	for i, chunkContent := range chunksText {
		var chunkID int64
		err := e.DB.QueryRow(ctx,
			`INSERT INTO documents (content, embedding, file_path) 
             VALUES ($1, $2, $3) RETURNING id`,
			chunkContent, pgvector.NewVector(embeds[i]), filePath).Scan(&chunkID)
		if err != nil {
			return err
		}

		// Tokenize the chunk and persist each token mapping in the database.
		tokens := tokenize(chunkContent)
		for _, token := range tokens {
			normalizedToken := strings.ToLower(token)
			if err := e.persistTokenMapping(ctx, normalizedToken, chunkID); err != nil {
				return fmt.Errorf("failed to persist token mapping for '%s': %w", normalizedToken, err)
			}
		}
	}
	return nil
}

// getQueryEmbedding returns a cached embedding if available; otherwise it generates and caches it.
func (e *Engine) getQueryEmbedding(query, embeddingsHost, apiKey string) ([]float32, error) {
	e.cacheMutex.RLock()
	embed, found := e.queryEmbeddingCache[query]
	e.cacheMutex.RUnlock()
	if found {
		return embed, nil
	}
	embeds, err := embeddings.GenerateEmbeddings(embeddingsHost, apiKey, []string{query})
	if err != nil {
		return nil, err
	}
	if len(embeds) == 0 {
		return nil, fmt.Errorf("failed to generate query embedding")
	}
	e.cacheMutex.Lock()
	e.queryEmbeddingCache[query] = embeds[0]
	e.cacheMutex.Unlock()
	return embeds[0], nil
}

// SearchChunks performs a semantic search on stored chunks using the query embedding.
// It also applies an optional file path filter.
func (e *Engine) SearchChunks(ctx context.Context, query, filePathFilter string, limit int, embeddingsHost string, apiKey string) ([]Chunk, error) {
	// Use cached embedding if available.
	queryEmbed, err := e.getQueryEmbedding(query, embeddingsHost, apiKey)
	if err != nil {
		return nil, err
	}
	vec := pgvector.NewVector(queryEmbed)

	// Build the SQL query.
	sqlQuery := "SELECT id, content, file_path FROM documents WHERE 1=1"
	var args []interface{}

	if filePathFilter != "" {
		sqlQuery += " AND file_path = $1"
		args = append(args, filePathFilter)
	}
	sqlQuery += " ORDER BY embedding <-> $2 LIMIT $3"
	args = append(args, vec, limit)

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

// SearchRelevantChunks performs either or both of:
// - semantic search (vector-based)
// - keyword/inverted index search (token-based)
// then merges (via "union" or "intersect") the resulting chunk IDs and returns the corresponding chunks.
func (e *Engine) SearchRelevantChunks(ctx context.Context,
	query string,
	filePathFilter string,
	limit int,
	useInvertedIndex bool,
	useVectorSearch bool,
	embeddingsHost, apiKey string,
	mergeMode string, // "union" or "intersect"
) ([]Chunk, error) {

	vectorSet := make(map[int64]bool)
	invertedSet := make(map[int64]bool)
	finalSet := make(map[int64]bool)

	// 1. If vector search is enabled, get chunk IDs from similarity search.
	if useVectorSearch {
		queryEmbed, err := e.getQueryEmbedding(query, embeddingsHost, apiKey)
		if err != nil {
			return nil, err
		}
		queryVec := pgvector.NewVector(queryEmbed)

		sqlQuery := `SELECT id FROM documents WHERE 1=1`
		var args []interface{}
		idx := 1

		if filePathFilter != "" {
			sqlQuery += fmt.Sprintf(" AND file_path = $%d", idx)
			args = append(args, filePathFilter)
			idx++
		}
		sqlQuery += fmt.Sprintf(" ORDER BY embedding <-> $%d LIMIT $%d", idx, idx+1)
		args = append(args, queryVec, limit)

		rows, err := e.DB.Query(ctx, sqlQuery, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		for rows.Next() {
			var cID int64
			if err := rows.Scan(&cID); err != nil {
				return nil, err
			}
			vectorSet[cID] = true
		}
		log.Printf("[SEFII] Vector search found %d chunks", len(vectorSet))
	}

	// 2. If inverted index is enabled, get chunk IDs by tokenizing and querying the DB.
	if useInvertedIndex {
		tokens := tokenize(query)
		for _, tk := range tokens {
			normalized := strings.ToLower(tk)
			rows, err := e.DB.Query(ctx, `
                SELECT chunk_ids FROM inverted_index WHERE token = $1
            `, normalized)
			if err != nil {
				return nil, fmt.Errorf("error querying inverted_index for token=%s: %w", normalized, err)
			}
			for rows.Next() {
				var jsonData []byte
				if err := rows.Scan(&jsonData); err != nil {
					rows.Close()
					return nil, err
				}
				// Parse the JSONB array of chunk IDs.
				var chunkIDs []int64
				if err := json.Unmarshal(jsonData, &chunkIDs); err != nil {
					rows.Close()
					return nil, fmt.Errorf("failed to parse JSONB chunk_ids: %w", err)
				}
				for _, id := range chunkIDs {
					invertedSet[id] = true
				}
			}
			rows.Close()
		}
		log.Printf("[SEFII] Inverted index search found %d unique chunk references", len(invertedSet))
	}

	// 3. Combine sets based on mergeMode.
	switch mergeMode {
	case "union":
		for id := range vectorSet {
			finalSet[id] = true
		}
		for id := range invertedSet {
			finalSet[id] = true
		}
	case "intersect":
		for id := range vectorSet {
			if invertedSet[id] {
				finalSet[id] = true
			}
		}
	default:
		for id := range vectorSet {
			finalSet[id] = true
		}
		for id := range invertedSet {
			finalSet[id] = true
		}
	}

	if len(finalSet) == 0 {
		log.Printf("[SEFII] No chunks found after merge (mode=%s)", mergeMode)
		return []Chunk{}, nil
	}

	// 4. Retrieve the actual chunks using an IN query.
	var idList []int64
	for id := range finalSet {
		idList = append(idList, id)
	}
	if len(idList) > limit {
		idList = idList[:limit]
	}

	rows, err := e.DB.Query(ctx, `
        SELECT id, content, file_path
        FROM documents
        WHERE id = ANY($1)
    `, idList)
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

// RetrieveDocumentsForChunks looks up all distinct file_paths for the given chunk IDs,
// then fetches and concatenates all chunks belonging to each file (ordered by id) to reconstruct the full document.
func (e *Engine) RetrieveDocumentsForChunks(ctx context.Context, chunkIDs []int64) (map[string]string, error) {
	rows, err := e.DB.Query(ctx, `
        SELECT DISTINCT file_path
        FROM documents
        WHERE id = ANY($1)
    `, chunkIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}

	documentsMap := make(map[string]string)
	for _, fp := range paths {
		crows, err := e.DB.Query(ctx, `
            SELECT content
            FROM documents
            WHERE file_path = $1
            ORDER BY id ASC
        `, fp)
		if err != nil {
			return nil, err
		}
		var builder strings.Builder
		for crows.Next() {
			var chunkText string
			if err := crows.Scan(&chunkText); err != nil {
				crows.Close()
				return nil, err
			}
			builder.WriteString(chunkText)
			builder.WriteString("\n\n")
		}
		crows.Close()
		documentsMap[fp] = builder.String()
	}
	return documentsMap, nil
}
