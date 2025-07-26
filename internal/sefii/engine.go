package sefii

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	documentsv1 "manifold/internal/documents/v1deprecated"
	embeddings "manifold/internal/llm"

	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
)

type Chunk struct {
	ID       int64             `json:"id"`
	Content  string            `json:"content"`
	Summary  string            `json:"summary,omitempty"`
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

// SummarizeOutput contains a summary and extracted keywords
type SummarizeOutput struct {
	Summary  string   `json:"summary"`
	Keywords []string `json:"keywords,omitempty"`
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
					   summary TEXT,
					   embedding vector(%d) NOT NULL,
					   tsv_content tsvector,
					   file_path TEXT,
					   metadata JSONB,
					   created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
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
		hasTSV := false
		hasSummary := false
		for _, c := range columnNames {
			if c == "metadata" {
				hasMetadata = true
			} else if c == "tsv_content" {
				hasTSV = true
			} else if c == "summary" {
				hasSummary = true
			}
		}
		if !hasMetadata {
			if err := e.execWithRetry(ctx, `ALTER TABLE documents ADD COLUMN metadata JSONB`); err != nil {
				return fmt.Errorf("failed to add metadata column: %w", err)
			}
			log.Println("Added column 'metadata' to table 'documents'")
		}
		if !hasTSV {
			if err := e.execWithRetry(ctx, `ALTER TABLE documents ADD COLUMN tsv_content tsvector`); err != nil {
				return fmt.Errorf("failed to add tsv_content column: %w", err)
			}
			log.Println("Added column 'tsv_content' to table 'documents'")
		}
		if !hasSummary {
			if err := e.execWithRetry(ctx, `ALTER TABLE documents ADD COLUMN summary TEXT`); err != nil {
				return fmt.Errorf("failed to add summary column: %w", err)
			}
			log.Println("Added column 'summary' to table 'documents'")
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

	// Ensure FTS index on main content
	ftsQuery := `CREATE INDEX IF NOT EXISTS documents_tsv_idx ON documents USING GIN (tsv_content)`
	if err := e.execWithRetry(ctx, ftsQuery); err != nil {
		return fmt.Errorf("failed to create FTS index: %w", err)
	}

	// Ensure FTS index on summary column for better semantic search
	summaryFtsQuery := `CREATE INDEX IF NOT EXISTS documents_summary_tsv_idx ON documents USING GIN (to_tsvector('english', COALESCE(summary, '')))`
	if err := e.execWithRetry(ctx, summaryFtsQuery); err != nil {
		return fmt.Errorf("failed to create summary FTS index: %w", err)
	}

	if err := e.EnsureDocumentMetadataTable(ctx); err != nil {
		return err
	}

	return nil
}

func (e *Engine) EnsureDocumentMetadataTable(ctx context.Context) error {
	var tableName *string
	err := e.DB.QueryRow(ctx, "SELECT to_regclass('public.document_metadata')").Scan(&tableName)
	if err != nil {
		return fmt.Errorf("failed to check for document_metadata table: %w", err)
	}

	if tableName == nil || *tableName == "" {
		createTableQuery := `
			CREATE TABLE document_metadata (
				file_path TEXT PRIMARY KEY,
				metadata JSONB NOT NULL
			)
		`
		if err := e.execWithRetry(ctx, createTableQuery); err != nil {
			return fmt.Errorf("failed to create document_metadata table: %w", err)
		}
		log.Println("Created document_metadata table")
	}
	return nil
}

// summarizeChunk sends the chunk content to the completions endpoint to obtain a summary.
func SummarizeChunk(ctx context.Context, content string, endpoint string, model string, apiKey string) (SummarizeOutput, error) {
	// Prepare summary instruction prompt
	summaryInstructions := `You are an expert text summarizer designed to create concise, informative summaries of document chunks for use in a Retrieval-Augmented Generation (RAG) system. Your goal is to generate summaries that maximize the RAG system's effectiveness by enabling it to retrieve the most relevant text chunks based on user queries.

**Instructions:**
1. Analyze the provided text chunk and understand its main topics, key points, and important context.
2. Generate a very concise summary (1-2 sentences maximum, no more than 50 words) that captures the essential information of the chunk.
3. Focus on creating a summary that preserves the most important searchable elements that a user might query for.
4. Maintain factual accuracy while condensing information.
5. The output format must be:
	Summary: [text]
	Keywords: [comma-separated keywords]`

	// Use SDK to get summary
	msgs := []embeddings.ChatCompletionMessage{
		{Role: "system", Content: summaryInstructions},
		{Role: "user", Content: "Please summarize:\n" + content},
	}
	resp, err := embeddings.CallLLM(ctx, endpoint, apiKey, model, msgs, 2048, 0.6)
	if err != nil {
		return SummarizeOutput{}, err
	}
	summaryText := resp
	log.Printf("Summary: %s", summaryText)

	// Extract keywords via SDK
	keywords, err := extractKeywords(ctx, summaryText, endpoint, model, apiKey)
	if err != nil {
		return SummarizeOutput{}, err
	}

	return SummarizeOutput{Summary: summaryText, Keywords: keywords}, nil
}

// extractKeywords calls the LLM with a tuned system prompt to extract keywords.
// The LLM should return a comma delimited list of keywords which we then parse.
func extractKeywords(ctx context.Context, summary string, endpoint string, model string, apiKey string) ([]string, error) {
	// Prepare keyword extraction prompt
	keywordInstructions := `You are a specialized keyword extractor. Given the summary text of a document chunk, extract the most relevant keywords as a comma-delimited list.`

	// Use SDK to extract keywords
	msgs := []embeddings.ChatCompletionMessage{
		{Role: "system", Content: keywordInstructions},
		{Role: "user", Content: "Please extract keywords from the summary:\n" + summary},
	}
	resp, err := embeddings.CallLLM(ctx, endpoint, apiKey, model, msgs, 256, 0.6)
	if err != nil {
		return nil, err
	}

	parts := strings.Split(resp, ",")
	var keywords []string
	for _, part := range parts {
		if kw := strings.TrimSpace(part); kw != "" {
			keywords = append(keywords, kw)
		}
	}
	return keywords, nil
}

// IngestDocument ingests a document into the SEFII index. Summaries and
// keyword generation can be toggled via the generateSummary and
// generateKeywords flags.
func (e *Engine) IngestDocument(
	ctx context.Context,
	text, languageStr, filePath, docTitle string,
	keywords []string,
	embeddingsHost, model, apiKey, completionsHost, completionsAPIKey string,
	chunkSize int, chunkOverlap int, embeddingDims int,
	embedPrefix string,
	generateSummary bool,
	generateKeywords bool,
) error {

	// ensure table
	if err := e.EnsureTable(ctx, embeddingDims); err != nil {
		log.Printf("SEFII: Failed to ensure table: %v", err)
		return err
	}

	// Determine language based on provided string and get appropriate splitter
	language := documentsv1.DeduceLanguage(languageStr)
	splitter, err := documentsv1.FromLanguage(language)
	if err != nil {
		// Fallback to default if language not supported
		splitter, _ = documentsv1.FromLanguage(documentsv1.DEFAULT)
	}

	splitter.ChunkSize = chunkSize
	splitter.OverlapSize = chunkOverlap

	// Use adaptive method for chunking based on language
	var chunksText []string
	if language == documentsv1.DEFAULT {
		chunksText = splitter.SplitText(text)
	} else {
		// Use adaptive splitting for code and structured content
		chunksText = splitter.AdaptiveSplit(text)
	}

	// Remove empty or too-short chunks
	for i := 0; i < len(chunksText); i++ {
		if len(strings.TrimSpace(chunksText[i])) < 10 {
			log.Printf("Warning: Skipping too short chunk at index %d", i)
			chunksText = append(chunksText[:i], chunksText[i+1:]...)
			i--
		}
	}

	// Concurrent summarization and keyword extraction
	type chunkResult struct {
		summary  string
		keywords []string
		err      error
	}
	results := make([]chunkResult, len(chunksText))
	var wg sync.WaitGroup

	for i, chunkContent := range chunksText {
		wg.Add(1)
		go func(idx int, content string) {
			defer wg.Done()
			var res chunkResult
			if (generateSummary || generateKeywords) && completionsHost != "" && completionsAPIKey != "" && len(strings.TrimSpace(content)) > 10 {
				log.Printf("SEFII: Using completions endpoint: %s", completionsHost)
				summaryOutput, err := SummarizeChunk(ctx, content, completionsHost, model, completionsAPIKey)
				if err == nil {
					if generateSummary {
						res.summary = summaryOutput.Summary
					}
					if generateKeywords {
						res.keywords = summaryOutput.Keywords
					}
				} else {
					log.Printf("SEFII: Failed to summarize chunk %d: %v", idx, err)
					res.err = err
				}
			}
			results[idx] = res
		}(i, chunkContent)
	}
	wg.Wait()

	// Create embeddings
	embeds, err := embeddings.GenerateEmbeddings(embeddingsHost, apiKey, chunksText)
	if err != nil {
		log.Printf("SEFII: Failed to generate embeddings: %v", err)
		return err
	}
	if len(embeds) != len(chunksText) {
		return fmt.Errorf("embedding count mismatch: got %d for %d chunks", len(embeds), len(chunksText))
	}

	// Collect all keywords from chunks for document-level aggregation
	var allChunkKeywords []string

	// Insert each chunk with enhanced metadata and FTS optimization
	for i, chunkContent := range chunksText {
		chunkSummary := results[i].summary
		extractedKeywords := results[i].keywords

		if len(extractedKeywords) > 0 {
			allChunkKeywords = append(allChunkKeywords, extractedKeywords...)
		}

		// Combine provided keywords with extracted keywords
		var allKeywords []string
		allKeywords = append(allKeywords, keywords...)
		allKeywords = append(allKeywords, extractedKeywords...)

		// Remove duplicates from keywords
		keywordSet := make(map[string]bool)
		var uniqueKeywords []string
		for _, kw := range allKeywords {
			if kw != "" && !keywordSet[kw] {
				keywordSet[kw] = true
				uniqueKeywords = append(uniqueKeywords, kw)
			}
		}

		// Create enhanced metadata object
		chunkMetadata := map[string]string{
			"docTitle":   docTitle,
			"chunkIndex": fmt.Sprintf("%d", i),
			"language":   languageStr,
		}
		if generateKeywords && len(uniqueKeywords) > 0 {
			chunkMetadata["keywords"] = strings.Join(uniqueKeywords, ",")
		}

		// Include summary in metadata if available
		if chunkSummary != "" {
			chunkMetadata["summary"] = chunkSummary
		}

		// Create content optimized for both vector and FTS retrieval
		vectorContent := fmt.Sprintf("%s %s", embedPrefix, chunkContent)
		searchText := buildEnhancedSearchText(chunkContent, chunkSummary, uniqueKeywords, filePath, docTitle)

		var chunkID int64
		mdBytes, _ := json.Marshal(chunkMetadata)
		err = e.DB.QueryRow(ctx,
			`INSERT INTO documents (content, summary, embedding, tsv_content, file_path, metadata)
			 VALUES ($1, $2, $3, to_tsvector('english', $4), $5, $6)
			 RETURNING id`,
			vectorContent, chunkSummary, pgvector.NewVector(embeds[i]), searchText, filePath, mdBytes).Scan(&chunkID)
		if err != nil {
			log.Printf("SEFII: Failed to insert chunk %d: %v", i, err)
			return err
		}

		log.Printf("SEFII: Successfully inserted chunk %d with ID %d", i, chunkID)
	}

	// Store document-level aggregated metadata
	if generateKeywords && len(allChunkKeywords) > 0 {
		err = e.StoreDocumentMetadata(ctx, filePath, map[string]interface{}{
			"docTitle":           docTitle,
			"language":           languageStr,
			"totalChunks":        len(chunksText),
			"aggregatedKeywords": allChunkKeywords,
			"processingTime":     time.Now().UTC(),
		})
		if err != nil {
			log.Printf("SEFII: Failed to store document metadata: %v", err)
			// Don't fail the entire ingestion for metadata storage issues
		}
	}

	kwCount := 0
	if generateKeywords {
		kwCount = len(allChunkKeywords)
	}
	log.Printf("SEFII: Successfully ingested document with %d chunks, %d total keywords",
		len(chunksText), kwCount)

	return nil
}

// StoreDocumentMetadata stores document-level aggregated metadata
func (e *Engine) StoreDocumentMetadata(ctx context.Context, filePath string, metadata map[string]interface{}) error {
	mdBytes, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal document metadata: %w", err)
	}

	_, err = e.DB.Exec(ctx,
		`INSERT INTO document_metadata (file_path, metadata) 
		 VALUES ($1, $2) 
		 ON CONFLICT (file_path) 
		 DO UPDATE SET metadata = $2`,
		filePath, mdBytes)

	if err != nil {
		return fmt.Errorf("failed to store document metadata: %w", err)
	}

	log.Printf("Stored document metadata for %s", filePath)
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

	sqlQuery := "SELECT id, content, summary, file_path, metadata FROM documents WHERE 1=1"
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
		var summary *string // Use pointer to handle NULL values
		if err := rows.Scan(&c.ID, &c.Content, &summary, &c.FilePath, &mdBytes); err != nil {
			return nil, err
		}
		// Handle NULL summary
		if summary != nil {
			c.Summary = *summary
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
	embeddingsHost,
	apiKey string,
	searchPrefix string,
	mergeMode string,
	alpha, beta float64,
) ([]Chunk, error) {

	vectorSet := make(map[int64]float64)   // store a float "score" from vector
	invertedSet := make(map[int64]float64) // store a float "score" for keyword
	finalSet := make(map[int64]float64)

	// 1) Vector
	if useVectorSearch {
		searchQuery := fmt.Sprintf("%s%s", searchPrefix, query)
		qEmb, err := e.getQueryEmbedding(searchQuery, embeddingsHost, apiKey)
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

	// 2) Full text search
	if useInvertedIndex {
		sqlQuery := `SELECT id, ts_rank(tsv_content, q) AS rank FROM documents, plainto_tsquery('english', $1) q WHERE tsv_content @@ q`
		var args []interface{}
		args = append(args, query)
		if filePathFilter != "" {
			sqlQuery += " AND file_path = $2"
			args = append(args, filePathFilter)
		}
		sqlQuery += " ORDER BY rank DESC LIMIT $" + fmt.Sprint(len(args)+1)
		args = append(args, limit)
		rows, err := e.DB.Query(ctx, sqlQuery, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			var rank float64
			if err := rows.Scan(&id, &rank); err != nil {
				return nil, err
			}
			invertedSet[id] = rank
		}
		log.Printf("FTS search found %d chunks", len(invertedSet))
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
		// Use the provided alpha and beta values
		// Normalize scores if needed
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
		SELECT id, content, summary, file_path, metadata
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
		var summary *string // Use pointer to handle NULL values
		if err := rows.Scan(&c.ID, &c.Content, &summary, &c.FilePath, &mdBytes); err != nil {
			return nil, err
		}
		// Handle NULL summary
		if summary != nil {
			c.Summary = *summary
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

// buildEnhancedSearchText creates optimized content for PostgreSQL full-text search
// by combining chunk content, summary, keywords, and metadata in a structured way
func buildEnhancedSearchText(content, summary string, keywords []string, filePath, docTitle string) string {
	var searchComponents []string

	// 1. Primary content with higher weight (will appear first)
	if content != "" {
		searchComponents = append(searchComponents, content)
	}

	// 2. Summary for semantic context (if available)
	if summary != "" {
		searchComponents = append(searchComponents, fmt.Sprintf("SUMMARY: %s", summary))
	}

	// 3. Keywords for exact matching
	if len(keywords) > 0 {
		keywordText := fmt.Sprintf("KEYWORDS: %s", strings.Join(keywords, " "))
		searchComponents = append(searchComponents, keywordText)
	}

	// 4. File metadata for path-based searches
	if filePath != "" {
		// Extract filename from path for better tokenization
		filename := filepath.Base(filePath)
		// Normalize the path to improve matching
		normalizedPath := strings.ReplaceAll(filePath, "/", " ")
		normalizedPath = strings.ReplaceAll(normalizedPath, ".", " ")

		fileMetadata := fmt.Sprintf("FILE: %s FILENAME: %s FILEPATH: %s",
			normalizedPath, filename, filePath)
		searchComponents = append(searchComponents, fileMetadata)
	}

	// 5. Document title for document-level searches
	if docTitle != "" {
		searchComponents = append(searchComponents, fmt.Sprintf("TITLE: %s", docTitle))
	}

	// Join all components with double newlines for clear separation
	return strings.Join(searchComponents, "\n\n")
}

// buildSearchText combines the content with file path and document title so that
// tokens from these metadata fields are also indexed.
// Deprecated: Use buildEnhancedSearchText for better FTS optimization
func buildSearchText(content, filePath, docTitle string) string {
	// Extract filename from path for better tokenization
	filename := filepath.Base(filePath)
	// Normalize the path to improve matching
	normalizedPath := strings.ReplaceAll(filePath, "/", " ")
	normalizedPath = strings.ReplaceAll(normalizedPath, ".", " ")

	return fmt.Sprintf("%s %s %s %s", content, normalizedPath, filename, docTitle)
}

// GetDocumentMetadata retrieves the consolidated metadata for a document by file path
func (e *Engine) GetDocumentMetadata(ctx context.Context, filePath string) (map[string]interface{}, error) {
	var metadataBytes []byte
	err := e.DB.QueryRow(ctx,
		"SELECT metadata FROM document_metadata WHERE file_path = $1",
		filePath).Scan(&metadataBytes)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // No metadata found, not an error
		}
		return nil, fmt.Errorf("failed to retrieve document metadata: %w", err)
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal document metadata: %w", err)
	}

	return metadata, nil
}

// SearchBySummary performs searches prioritizing summary content for semantic queries
func (e *Engine) SearchBySummary(ctx context.Context, query, filePathFilter string, limit int) ([]Chunk, error) {
	// Enhanced FTS query that gives higher weight to summary matches
	sqlQuery := `
		SELECT id, content, summary, file_path, metadata,
			   (ts_rank(to_tsvector('english', COALESCE(summary, '')), q) * 2.0 + 
				ts_rank(tsv_content, q)) AS combined_rank
		FROM documents, plainto_tsquery('english', $1) q 
		WHERE (to_tsvector('english', COALESCE(summary, '')) @@ q OR tsv_content @@ q)
	`

	var args []interface{}
	args = append(args, query)

	if filePathFilter != "" {
		sqlQuery += " AND file_path = $2"
		args = append(args, filePathFilter)
	}

	sqlQuery += " ORDER BY combined_rank DESC LIMIT $" + fmt.Sprint(len(args)+1)
	args = append(args, limit)

	rows, err := e.DB.Query(ctx, sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Chunk
	for rows.Next() {
		var c Chunk
		var mdBytes []byte
		var summary *string // Use pointer to handle NULL values
		var rank float64
		if err := rows.Scan(&c.ID, &c.Content, &summary, &c.FilePath, &mdBytes, &rank); err != nil {
			return nil, err
		}
		// Handle NULL summary
		if summary != nil {
			c.Summary = *summary
		}
		if len(mdBytes) > 0 {
			meta := make(map[string]string)
			_ = json.Unmarshal(mdBytes, &meta)
			c.Metadata = meta
		}
		// Add rank to metadata for debugging
		if c.Metadata == nil {
			c.Metadata = make(map[string]string)
		}
		c.Metadata["fts_rank"] = fmt.Sprintf("%.4f", rank)

		results = append(results, c)
	}

	log.Printf("Summary-weighted FTS search found %d chunks", len(results))
	return results, nil
}
