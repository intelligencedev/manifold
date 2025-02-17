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
func (e *Engine) EnsureTable(ctx context.Context, embeddingVectorSize int) error {
	var tableName *string
	// Check if the table exists using PostgreSQL's to_regclass.
	err := e.DB.QueryRow(ctx, "SELECT to_regclass('public.documents')").Scan(&tableName)
	if err != nil {
		return fmt.Errorf("failed to check for documents table: %w", err)
	}

	query := fmt.Sprintf(`
		CREATE TABLE documents (
			id SERIAL PRIMARY KEY,
			content TEXT NOT NULL,
			embedding vector(%d) NOT NULL,
			file_path TEXT
		)
	`, embeddingVectorSize)

	if tableName == nil || *tableName == "" {
		// Table doesn't exist; create it.
		_, err := e.DB.Exec(ctx, query)
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
func (e *Engine) SearchChunks(ctx context.Context, query, filePathFilter string, limit int, embeddingsHost string, apiKey string) ([]Chunk, error) {
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

	// 1. Prepare empty sets for chunk IDs from each method.
	vectorSet := make(map[int64]bool)
	invertedSet := make(map[int64]bool)
	finalSet := make(map[int64]bool)

	// 2. If vector search is enabled, get chunk IDs from similarity search.
	if useVectorSearch {
		// Generate query embedding.
		queryEmbeds, err := e.generateQueryEmbeddings(query, embeddingsHost, apiKey)
		if err != nil {
			return nil, err
		}
		if len(queryEmbeds) == 0 {
			return nil, fmt.Errorf("failed to generate query embedding")
		}
		queryVec := pgvector.NewVector(queryEmbeds[0])

		// Build the SQL query.
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

		count := 0
		for rows.Next() {
			var cID int64
			if err := rows.Scan(&cID); err != nil {
				return nil, err
			}
			vectorSet[cID] = true
			count++
		}
		log.Printf("[SEFII] Vector search found %d chunks", count)
	}

	// 3. If inverted index is enabled, get chunk IDs by tokenizing.
	if useInvertedIndex {
		tokens := tokenize(query) // using the same tokenization as in ingestion.
		countTotals := 0
		for _, tk := range tokens {
			normalized := strings.ToLower(tk)
			// We can query the inverted_index table.
			rows, err := e.DB.Query(ctx, `
                SELECT chunk_id FROM inverted_index WHERE token = $1
            `, normalized)
			if err != nil {
				return nil, fmt.Errorf("error querying inverted_index for token=%s: %w", normalized, err)
			}
			for rows.Next() {
				var cID int64
				if err := rows.Scan(&cID); err != nil {
					return nil, err
				}
				invertedSet[cID] = true
				countTotals++
			}
			rows.Close()
		}
		log.Printf("[SEFII] Inverted index search found %d chunk references", countTotals)
	}

	// 4. Combine sets depending on mergeMode.
	switch mergeMode {
	case "union":
		// All chunks that appear in either set.
		for cID := range vectorSet {
			finalSet[cID] = true
		}
		for cID := range invertedSet {
			finalSet[cID] = true
		}
	case "intersect":
		// Only those that appear in both.
		for cID := range vectorSet {
			if invertedSet[cID] {
				finalSet[cID] = true
			}
		}
	default:
		// If mergeMode is not recognized, default to union.
		for cID := range vectorSet {
			finalSet[cID] = true
		}
		for cID := range invertedSet {
			finalSet[cID] = true
		}
	}

	if len(finalSet) == 0 {
		log.Printf("[SEFII] No chunks found after merge (mode=%s)", mergeMode)
		return []Chunk{}, nil
	}

	// 5. Retrieve the actual chunks from `documents` using an IN query.
	chunkIDs := make([]int64, 0, len(finalSet))
	for id := range finalSet {
		chunkIDs = append(chunkIDs, id)
	}
	// If the set is larger than the limit, trim it.
	if len(chunkIDs) > limit {
		chunkIDs = chunkIDs[:limit]
	}

	rows, err := e.DB.Query(ctx, `
        SELECT id, content, file_path
        FROM documents
        WHERE id = ANY($1)
    `, chunkIDs)
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

// generateQueryEmbeddings is a helper to obtain the query embedding.
func (e *Engine) generateQueryEmbeddings(query string, embeddingsHost, apiKey string) ([][]float32, error) {
	// Reuse the embeddings package.
	return embeddings.GenerateEmbeddings(embeddingsHost, apiKey, []string{query})
}

// RetrieveDocumentsForChunks looks up all distinct file_paths for the given chunk IDs,
// then fetches and concatenates all chunks belonging to each file (ordered by id) to reconstruct the full document.
func (e *Engine) RetrieveDocumentsForChunks(ctx context.Context, chunkIDs []int64) (map[string]string, error) {
	// 1. Get distinct file_paths from the selected chunk IDs.
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

	// 2. For each file_path, get all chunks and concatenate them.
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
