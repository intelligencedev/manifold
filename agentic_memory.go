// agentic_memory.go
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"github.com/pgvector/pgvector-go"

	"manifold/internal/sefii"
)

// AgenticMemory represents a single memory note.
type AgenticMemory struct {
	ID          int64           `json:"id"`
	Content     string          `json:"content"`
	NoteContext string          `json:"note_context"`
	Keywords    []string        `json:"keywords"`
	Tags        []string        `json:"tags"`
	Timestamp   time.Time       `json:"timestamp"`
	Embedding   pgvector.Vector `json:"embedding"`
	Links       []int64         `json:"links"`
}

// AgenticEngine handles agentic memory operations.
type AgenticEngine struct {
	DB *pgx.Conn
}

// NewAgenticEngine returns a new agentic engine instance.
func NewAgenticEngine(db *pgx.Conn) *AgenticEngine {
	return &AgenticEngine{DB: db}
}

// EnsureAgenticMemoryTable creates the table if it does not exist.
func (ae *AgenticEngine) EnsureAgenticMemoryTable(ctx context.Context, embeddingDim int) error {
	var tableName *string
	err := ae.DB.QueryRow(ctx, "SELECT to_regclass('public.agentic_memories')").Scan(&tableName)
	if err != nil {
		return fmt.Errorf("failed to check for agentic_memories table: %w", err)
	}
	if tableName == nil || *tableName == "" {
		createTableQuery := fmt.Sprintf(`
			CREATE TABLE agentic_memories (
				id SERIAL PRIMARY KEY,
				content TEXT NOT NULL,
				note_context TEXT,
				keywords TEXT[],
				tags TEXT[],
				timestamp TIMESTAMP,
				embedding vector(%d) NOT NULL,
				links INTEGER[]
			)
		`, embeddingDim)
		if _, err := ae.DB.Exec(ctx, createTableQuery); err != nil {
			return fmt.Errorf("failed to create agentic_memories table: %w", err)
		}
		// Create a vector index for similarity search.
		indexQuery := `
			CREATE INDEX IF NOT EXISTS agentic_memories_embedding_idx
			ON agentic_memories USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100)
		`
		if _, err := ae.DB.Exec(ctx, indexQuery); err != nil {
			return fmt.Errorf("failed to create ivfflat index on agentic_memories.embedding: %w", err)
		}
	}
	return nil
}

// IngestAgenticMemory ingests a new memory note.
func (ae *AgenticEngine) IngestAgenticMemory(
	ctx context.Context,
	config *Config,
	content string,
	docTitle string,
) (int64, error) {
	log.Println("Ingesting agentic memory note...")
	log.Println(content)
	// 1. Use an LLM (or completions endpoint) to generate note context, keywords, and tags.
	summaryOutput, err := sefii.SummarizeChunk(ctx, content, config.Completions.DefaultHost, config.Completions.APIKey)
	if err != nil {
		log.Printf("AgenticMemory: Failed to summarize content: %v", err)
		// If summarization fails, proceed with empty context.
		summaryOutput.Summary = ""
	}
	noteContext := summaryOutput.Summary
	keywords := summaryOutput.Keywords
	// For tags, here we simply reuse keywords. Adjust as needed.
	tags := keywords

	// 2. Compute the embedding.
	embeddingInput := config.Embeddings.EmbedPrefix + content + " " + noteContext + " " + strings.Join(keywords, " ") + " " + strings.Join(tags, " ")
	embeds, err := GenerateEmbeddings(config.Embeddings.Host, config.Embeddings.APIKey, []string{embeddingInput})
	if err != nil || len(embeds) == 0 {
		return 0, fmt.Errorf("failed to generate embedding: %w", err)
	}
	vec := pgvector.NewVector(embeds[0])

	// 3. Insert the new memory note.
	currentTime := time.Now()
	var newID int64
	insertQuery := `
        INSERT INTO agentic_memories (content, note_context, keywords, tags, timestamp, embedding, links)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
        RETURNING id
    `
	emptyLinks := []int64{}
	err = ae.DB.QueryRow(ctx, insertQuery, content, noteContext, keywords, tags, currentTime, vec, emptyLinks).Scan(&newID)
	if err != nil {
		return 0, fmt.Errorf("failed to insert agentic memory note: %w", err)
	}

	// 4. Generate links for the new note.
	relatedIDs, err := ae.generateLinks(ctx, newID, 5, config.Completions.DefaultHost, config.Completions.APIKey)
	if err != nil {
		log.Printf("AgenticMemory: Failed to generate links: %v", err)
	} else {
		updateQuery := `UPDATE agentic_memories SET links = $1 WHERE id = $2`
		_, err = ae.DB.Exec(ctx, updateQuery, relatedIDs, newID)
		if err != nil {
			log.Printf("AgenticMemory: Failed to update links: %v", err)
		}
		// Optionally, call memory evolution here to update neighbor notes.
	}

	return newID, nil
}

// generateLinks performs a vector search to find candidate related memory notes.
// In this example, we simply return the candidate IDs.
func (ae *AgenticEngine) generateLinks(ctx context.Context, newMemoryID int64, k int, completionsHost, completionsAPIKey string) ([]int64, error) {
	// Retrieve new note embedding and content.
	var newEmbedding pgvector.Vector
	var newContent string
	query := `SELECT embedding, content FROM agentic_memories WHERE id = $1`
	err := ae.DB.QueryRow(ctx, query, newMemoryID).Scan(&newEmbedding, &newContent)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch new memory note: %w", err)
	}

	// Vector search in agentic_memories (excluding the new note).
	searchQuery := `
        SELECT id FROM agentic_memories 
        WHERE id <> $1 
        ORDER BY embedding <-> $2
        LIMIT $3
    `
	rows, err := ae.DB.Query(ctx, searchQuery, newMemoryID, newEmbedding, k)
	if err != nil {
		return nil, fmt.Errorf("failed to search similar agentic memories: %w", err)
	}
	defer rows.Close()

	var candidateIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err == nil {
			candidateIDs = append(candidateIDs, id)
		}
	}

	// In a full implementation, you might call an LLM to verify these candidates.
	// Here, we simply return the candidate IDs.
	return candidateIDs, nil
}

// SearchAgenticMemories performs a vector-based search on agentic_memories.
func (ae *AgenticEngine) SearchAgenticMemories(ctx context.Context, config *Config, queryText string, limit int) ([]AgenticMemory, error) {
	embeds, err := GenerateEmbeddings(config.Embeddings.Host, config.Embeddings.APIKey, []string{queryText})
	if err != nil || len(embeds) == 0 {
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}
	queryVec := pgvector.NewVector(embeds[0])

	// Cast keywords and tags to text to force string output.
	searchQuery := `
        SELECT id, content, note_context, keywords::text, tags::text, timestamp, embedding, links
        FROM agentic_memories
        ORDER BY embedding <-> $1
        LIMIT $2
    `
	rows, err := ae.DB.Query(ctx, searchQuery, queryVec, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []AgenticMemory
	for rows.Next() {
		var mem AgenticMemory
		var kwStr, tagStr string
		var ts time.Time
		err := rows.Scan(&mem.ID, &mem.Content, &mem.NoteContext, &kwStr, &tagStr, &ts, &mem.Embedding, &mem.Links)
		if err != nil {
			return nil, err
		}
		mem.Keywords = parseTextArray(kwStr)
		mem.Tags = parseTextArray(tagStr)
		mem.Timestamp = ts
		results = append(results, mem)
	}
	return results, nil
}

// parseTextArray is a simple helper to convert Postgres TEXT[] output to a slice of strings.
func parseTextArray(input string) []string {
	input = strings.Trim(input, "{}")
	if input == "" {
		return []string{}
	}
	parts := strings.Split(input, ",")
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}
	return parts
}

// agenticMemoryIngestHandler handles POST /api/agentic-memory/ingest.
func agenticMemoryIngestHandler(config *Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req struct {
			Content           string `json:"content"`
			DocTitle          string `json:"doc_title"`
			CompletionsHost   string `json:"completions_host"`
			CompletionsAPIKey string `json:"completions_api_key"`
			EmbeddingsHost    string `json:"embeddings_host"`
			EmbeddingsAPIKey  string `json:"embeddings_api_key"`
		}
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		}
		if req.Content == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Content is required"})
		}
		ctx := c.Request().Context()
		conn, err := Connect(ctx, config.Database.ConnectionString)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to connect to database"})
		}
		defer conn.Close(ctx)
		engine := NewAgenticEngine(conn)
		if err := engine.EnsureAgenticMemoryTable(ctx, config.Embeddings.Dimensions); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Failed to ensure agentic memory table: %v", err)})
		}
		newID, err := engine.IngestAgenticMemory(ctx, config, req.Content, req.DocTitle)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Failed to ingest agentic memory: %v", err)})
		}
		return c.JSON(http.StatusOK, map[string]interface{}{"message": "Agentic memory ingested successfully", "id": newID})
	}
}

// agenticMemorySearchHandler handles POST /api/agentic-memory/search.
func agenticMemorySearchHandler(config *Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req struct {
			Query            string `json:"query"`
			Limit            int    `json:"limit"`
			EmbeddingsHost   string `json:"embeddings_host"`
			EmbeddingsAPIKey string `json:"embeddings_api_key"`
		}
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		}
		if req.Query == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Query is required"})
		}
		if req.Limit == 0 {
			req.Limit = 10
		}
		ctx := c.Request().Context()
		conn, err := Connect(ctx, config.Database.ConnectionString)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to connect to database"})
		}
		defer conn.Close(ctx)
		engine := NewAgenticEngine(conn)

		searchQuery := fmt.Sprintf("%s%s", config.Embeddings.SearchPrefix, req.Query)

		results, err := engine.SearchAgenticMemories(ctx, config, searchQuery, req.Limit)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Failed to search agentic memories: %v", err)})
		}
		return c.JSON(http.StatusOK, map[string]interface{}{"results": results})
	}
}
