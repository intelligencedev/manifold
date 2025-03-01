package sefii

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"manifold/internal/documents"
	"manifold/internal/embeddings"

	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
)

type Chunk struct {
	ID       int64             `json:"id"`
	Content  string            `json:"content"`
	FilePath string            `json:"file_path"`
	Metadata map[string]string `json:"metadata"` // additional metadata
}

type scoredItem struct {
	id    int64
	score float64
}

type Engine struct {
	DB                  *pgx.Conn
	queryEmbeddingCache map[string][]float32
	cacheMutex          sync.RWMutex
}

func NewEngine(db *pgx.Conn) *Engine {
	return &Engine{
		DB:                  db,
		queryEmbeddingCache: make(map[string][]float32),
	}
}

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

// We add a metadata JSONB column to the "documents" table, plus a new index if needed.
func (e *Engine) EnsureTable(ctx context.Context, embeddingVectorSize int) error {
	var tableName *string
	err := e.DB.QueryRow(ctx, "SELECT to_regclass('public.documents')").Scan(&tableName)
	if err != nil {
		return fmt.Errorf("failed to check for documents table: %w", err)
	}

	createTableQuery := fmt.Sprintf(`
		CREATE TABLE documents (
			id SERIAL PRIMARY KEY,
			content TEXT NOT NULL,
			embedding vector(%d) NOT NULL,
			file_path TEXT,
			metadata JSONB
		)
	`, embeddingVectorSize)

	if tableName == nil || *tableName == "" {
		// create the table
		if err := e.execWithRetry(ctx, createTableQuery); err != nil {
			return fmt.Errorf("failed to create documents table: %w", err)
		}
	} else {
		// ensure columns exist
		var columnNames []string
		rows, err := e.DB.Query(ctx, `
			SELECT column_name
			FROM information_schema.columns
			WHERE table_name = 'documents'
		`)
		if err != nil {
			return fmt.Errorf("failed to read columns: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var cn string
			if err := rows.Scan(&cn); err == nil {
				columnNames = append(columnNames, cn)
			}
		}
		// If metadata column not found, add it
		hasMetadata := false
		for _, c := range columnNames {
			if c == "metadata" {
				hasMetadata = true
				break
			}
		}
		if !hasMetadata {
			if err := e.execWithRetry(ctx, `ALTER TABLE documents ADD COLUMN metadata JSONB`); err != nil {
				return fmt.Errorf("failed to add metadata column: %w", err)
			}
			log.Println("Added column 'metadata' to table 'documents'")
		}
	}

	// Ensure vector index
	indexQuery := `
		CREATE INDEX IF NOT EXISTS documents_embedding_idx
		ON documents USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100)
	`
	if err := e.execWithRetry(ctx, indexQuery); err != nil {
		return fmt.Errorf("failed to create ivfflat index on documents.embedding: %w", err)
	}

	// check / create inverted_index if needed
	if err := e.EnsureInvertedIndexTable(ctx); err != nil {
		return err
	}

	return nil
}

func (e *Engine) EnsureInvertedIndexTable(ctx context.Context) error {
	var tableName *string
	err := e.DB.QueryRow(ctx, "SELECT to_regclass('public.inverted_index')").Scan(&tableName)
	if err != nil {
		return fmt.Errorf("failed to check for inverted_index: %w", err)
	}
	if tableName == nil || *tableName == "" {
		createIndexQuery := `
			CREATE TABLE inverted_index (
				token TEXT PRIMARY KEY,
				chunk_ids JSONB NOT NULL
			)
		`
		if err := e.execWithRetry(ctx, createIndexQuery); err != nil {
			return fmt.Errorf("failed to create inverted_index table: %w", err)
		}
	}
	return nil
}

// persistTokenMapping is unchanged except we might do minor improvements
func (e *Engine) persistTokenMapping(ctx context.Context, token string, chunkID int64) error {
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

// let's add some synonyms or expansions in tokenize
var synonyms = map[string][]string{
	"usa":  {"united_states", "america"},
	"nasa": {"space_agency"},
	// etc. This is just an example
}

// tokenize improved to handle synonyms
func tokenize(text string) []string {
	text = strings.ToLower(text)
	re := regexp.MustCompile(`[^\w\s]`)
	text = re.ReplaceAllString(text, "")
	words := strings.Fields(text)
	stopwords := map[string]bool{"the": true, "is": true, "at": true, "of": true, "on": true, "and": true}

	var tokens []string
	for _, w := range words {
		if stopwords[w] {
			continue
		}
		tokens = append(tokens, w)
		// add synonyms
		if syns, ok := synonyms[w]; ok {
			tokens = append(tokens, syns...)
		}
	}
	return tokens
}

// IngestDocument updated to accept docTitle, keywords, etc.
func (e *Engine) IngestDocument(
	ctx context.Context,
	text, language, filePath, docTitle string,
	keywords []string,
	embeddingsHost, apiKey string,
	chunkSize int, chunkOverlap int,
) error {

	// ensure table
	if err := e.EnsureTable(ctx, 768 /* or whatever dim you have */); err != nil {
		log.Printf("SEFII: Failed to ensure table: %v", err)
		return err
	}

	splitter, err := documents.FromLanguage(documents.Language(language))
	if err != nil {
		return err
	}

	splitter.ChunkSize = chunkSize
	splitter.OverlapSize = chunkOverlap

	// Use our new adaptive method for chunking
	chunksText := splitter.AdaptiveSplit(text)

	// Create embeddings
	embeds, err := embeddings.GenerateEmbeddings(embeddingsHost, apiKey, chunksText)
	if err != nil {
		log.Printf("SEFII: Failed to generate embeddings: %v", err)
		return err
	}
	if len(embeds) != len(chunksText) {
		return fmt.Errorf("embedding count mismatch: got %d for %d chunks", len(embeds), len(chunksText))
	}

	// Insert each chunk
	for i, chunkContent := range chunksText {
		chunkMetadata := map[string]string{
			"docTitle": docTitle,
		}
		// store keywords in metadata if you want
		if len(keywords) > 0 {
			// convert keywords to a comma string or store as JSON
			// We'll do a naive approach:
			chunkMetadata["keywords"] = strings.Join(keywords, ",")
		}

		mdBytes, _ := json.Marshal(chunkMetadata)

		var chunkID int64
		err := e.DB.QueryRow(ctx,
			`INSERT INTO documents (content, embedding, file_path, metadata)
			 VALUES ($1, $2, $3, $4)
			 RETURNING id`,
			chunkContent, pgvector.NewVector(embeds[i]), filePath, mdBytes).Scan(&chunkID)
		if err != nil {
			log.Printf("SEFII: Failed to insert chunk: %v", err)
			return err
		}

		// Inverted index
		tokens := tokenize(chunkContent)
		for _, token := range tokens {
			if err := e.persistTokenMapping(ctx, token, chunkID); err != nil {
				log.Printf("SEFII: Failed to persist token mapping: %v", err)
				return fmt.Errorf("failed to persist token mapping: %w", err)
			}
		}
	}

	return nil
}

// Caching for query embeddings
func (e *Engine) getQueryEmbedding(query, embeddingsHost, apiKey string) ([]float32, error) {
	e.cacheMutex.RLock()
	if emb, ok := e.queryEmbeddingCache[query]; ok {
		e.cacheMutex.RUnlock()
		log.Printf("SEFII: Query embedding cache hit for %q", query)
		return emb, nil
	}
	e.cacheMutex.RUnlock()

	embeds, err := embeddings.GenerateEmbeddings(embeddingsHost, apiKey, []string{query})
	if err != nil {
		log.Printf("SEFII: Failed to generate embedding for %q: %v", query, err)
		return nil, err
	}
	if len(embeds) == 0 {
		log.Printf("SEFII: No embeddings generated for %q", query)
		return nil, fmt.Errorf("failed to generate embedding")
	}
	e.cacheMutex.Lock()
	e.queryEmbeddingCache[query] = embeds[0]
	e.cacheMutex.Unlock()

	return embeds[0], nil
}

// Basic vector-only search
func (e *Engine) SearchChunks(ctx context.Context, query, filePathFilter string, limit int, embeddingsHost, apiKey string) ([]Chunk, error) {
	queryEmb, err := e.getQueryEmbedding(query, embeddingsHost, apiKey)
	if err != nil {
		return nil, err
	}
	vec := pgvector.NewVector(queryEmb)

	sqlQuery := "SELECT id, content, file_path, metadata FROM documents WHERE 1=1"
	var args []interface{}

	idx := 1
	if filePathFilter != "" {
		sqlQuery += fmt.Sprintf(" AND file_path = $%d", idx)
		args = append(args, filePathFilter)
		idx++
	}
	sqlQuery += fmt.Sprintf(" ORDER BY embedding <-> $%d LIMIT $%d", idx, idx+1)
	args = append(args, vec, limit)

	rows, err := e.DB.Query(ctx, sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Chunk
	for rows.Next() {
		var c Chunk
		var mdBytes []byte
		if err := rows.Scan(&c.ID, &c.Content, &c.FilePath, &mdBytes); err != nil {
			return nil, err
		}
		if len(mdBytes) > 0 {
			meta := make(map[string]string)
			_ = json.Unmarshal(mdBytes, &meta)
			c.Metadata = meta
		}
		results = append(results, c)
	}
	return results, nil
}

// Weighted or union/intersect approach
func (e *Engine) SearchRelevantChunks(ctx context.Context,
	query string,
	filePathFilter string,
	limit int,
	useInvertedIndex bool,
	useVectorSearch bool,
	embeddingsHost, apiKey string,
	mergeMode string,
) ([]Chunk, error) {

	vectorSet := make(map[int64]float64)   // store a float "score" from vector
	invertedSet := make(map[int64]float64) // store a float "score" for keyword
	finalSet := make(map[int64]float64)

	// 1) Vector
	if useVectorSearch {
		qEmb, err := e.getQueryEmbedding(query, embeddingsHost, apiKey)
		if err != nil {
			return nil, err
		}
		vec := pgvector.NewVector(qEmb)

		sqlQuery := `SELECT id, (1 - (embedding <-> $1)) as sim_score FROM documents`
		// (1 - distance) is a naive way to make bigger=better
		var conds []string
		var args []interface{}
		idx := 2
		if filePathFilter != "" {
			conds = append(conds, fmt.Sprintf("file_path = $%d", idx))
			args = append(args, filePathFilter)
			idx++
		}
		if len(conds) > 0 {
			sqlQuery += " WHERE " + strings.Join(conds, " AND ")
		}
		sqlQuery += fmt.Sprintf(" ORDER BY embedding <-> $1 LIMIT $%d", idx)
		args = append([]interface{}{vec}, limit)

		rows, err := e.DB.Query(ctx, sqlQuery, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		for rows.Next() {
			var id int64
			var simScore float64
			if err := rows.Scan(&id, &simScore); err != nil {
				return nil, err
			}
			vectorSet[id] = simScore
		}
		log.Printf("Vector search found %d chunks", len(vectorSet))
	}

	// 2) Inverted index
	if useInvertedIndex {
		toks := tokenize(query)
		seen := make(map[int64]bool)
		for _, tk := range toks {
			r2, err := e.DB.Query(ctx, `SELECT chunk_ids FROM inverted_index WHERE token=$1`, tk)
			if err != nil {
				return nil, fmt.Errorf("error in inverted_index for token=%s: %w", tk, err)
			}
			for r2.Next() {
				var jsonData []byte
				if err := r2.Scan(&jsonData); err != nil {
					r2.Close()
					return nil, err
				}
				var chunkIDs []int64
				if err := json.Unmarshal(jsonData, &chunkIDs); err != nil {
					r2.Close()
					return nil, err
				}
				for _, cid := range chunkIDs {
					if !seen[cid] {
						invertedSet[cid] += 1.0 // simple additive
						seen[cid] = true
					}
				}
			}
			r2.Close()
		}
		log.Printf("Inverted index found %d chunk references", len(invertedSet))
	}

	// 3) Merge
	switch mergeMode {
	case "union":
		// union: take all chunk IDs from both sets
		for k, vs := range vectorSet {
			finalSet[k] += vs
		}
		for k, is := range invertedSet {
			if _, ok := finalSet[k]; !ok {
				finalSet[k] = is
			} else {
				finalSet[k] += is
			}
		}
	case "intersect":
		// intersect: only IDs in both sets
		for k, vs := range vectorSet {
			if is, ok := invertedSet[k]; ok {
				finalSet[k] = vs + is
			}
		}
	case "weighted":
		// Weighted example: alpha * vector + beta * inverted
		alpha, beta := 0.7, 0.3
		allIDs := make(map[int64]bool)
		for k := range vectorSet {
			allIDs[k] = true
		}
		for k := range invertedSet {
			allIDs[k] = true
		}
		for cid := range allIDs {
			vs := vectorSet[cid]
			is := invertedSet[cid]
			score := alpha*vs + beta*is
			finalSet[cid] = score
		}
	default:
		// fallback union
		for k, vs := range vectorSet {
			finalSet[k] += vs
		}
		for k, is := range invertedSet {
			finalSet[k] += is
		}
	}
	// Define scoredItem type at the package level
	// Sort by final score descending
	var scored []scoredItem
	for cid, sc := range finalSet {
		scored = append(scored, scoredItem{cid, sc})
	}
	// sort
	scored = sortByScoreDesc(scored)

	// take top N
	if len(scored) > limit {
		scored = scored[:limit]
	}

	// 4) fetch actual chunks
	var idList []int64
	for _, si := range scored {
		idList = append(idList, si.id)
	}
	return e.fetchChunksByIDs(ctx, idList)
}

func (e *Engine) fetchChunksByIDs(ctx context.Context, ids []int64) ([]Chunk, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := e.DB.Query(ctx, `
		SELECT id, content, file_path, metadata
		FROM documents
		WHERE id = ANY($1)
	`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Chunk
	for rows.Next() {
		var c Chunk
		var mdBytes []byte
		if err := rows.Scan(&c.ID, &c.Content, &c.FilePath, &mdBytes); err != nil {
			return nil, err
		}
		if len(mdBytes) > 0 {
			meta := make(map[string]string)
			_ = json.Unmarshal(mdBytes, &meta)
			c.Metadata = meta
		}
		results = append(results, c)
	}
	return results, nil
}

// Sort helper
func sortByScoreDesc(items []scoredItem) []scoredItem {
	// simple bubble or any sorting
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if items[j].score > items[i].score {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
	return items
}

func (e *Engine) RetrieveDocumentsForChunks(ctx context.Context, chunkIDs []int64) (map[string]string, error) {
	// same as your existing logic
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
