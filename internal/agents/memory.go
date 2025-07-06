// agentic_memory.go
package agents

import (
	"context"
	"fmt"
	"log"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"github.com/pgvector/pgvector-go"

	openai "github.com/sashabaranov/go-openai"
	configpkg "manifold/internal/config"
	llm "manifold/internal/llm"
	"manifold/internal/sefii"
)

// MemoryRequest defines the structure of a request to store a memory
type MemoryRequest struct {
	WorkflowID  string   `json:"workflow_id"`
	Content     string   `json:"content"`
	NoteContext string   `json:"note_context"`
	Keywords    []string `json:"keywords"`
	Tags        []string `json:"tags"`
	Links       []int64  `json:"links"`
}

// Memory represents a memory entry in the database
type AgenticMemory struct {
	ID          int64           `json:"id"`
	WorkflowID  string          `json:"workflow_id"`
	Content     string          `json:"content"`
	NoteContext string          `json:"note_context"`
	Keywords    []string        `json:"keywords"`
	Tags        []string        `json:"tags"`
	Timestamp   time.Time       `json:"timestamp"`
	Embedding   pgvector.Vector `json:"embedding"`
	Links       []int64         `json:"links"`
}

// MemoryCluster represents a cluster of related memories
type MemoryCluster struct {
	ID         int             `json:"id"`
	CenterID   int64           `json:"center_id"`
	Members    []AgenticMemory `json:"members"`
	Themes     []string        `json:"themes"`
	Confidence float64         `json:"confidence"`
	CreatedAt  time.Time       `json:"created_at"`
}

// NetworkHealth represents the health metrics of the memory network
type NetworkHealth struct {
	TotalMemories       int     `json:"total_memories"`
	TotalConnections    int     `json:"total_connections"`
	AverageConnectivity float64 `json:"average_connectivity"`
	IsolatedMemories    int     `json:"isolated_memories"`
	LargestComponent    int     `json:"largest_component"`
	ClusteringScore     float64 `json:"clustering_score"`
	Contradictions      int     `json:"contradictions"`
	HealthScore         float64 `json:"health_score"`
}

// MemoryEvolution represents the evolution of a memory concept
type MemoryEvolution struct {
	ID            int64     `json:"id"`
	OriginalID    int64     `json:"original_id"`
	EvolvedID     int64     `json:"evolved_id"`
	EvolutionType string    `json:"evolution_type"` // "refinement", "contradiction", "expansion"
	Confidence    float64   `json:"confidence"`
	CreatedAt     time.Time `json:"created_at"`
}

// MemoryContradiction represents a detected contradiction between memories
type MemoryContradiction struct {
	ID           int64      `json:"id"`
	Memory1ID    int64      `json:"memory1_id"`
	Memory2ID    int64      `json:"memory2_id"`
	ConflictType string     `json:"conflict_type"`
	Severity     float64    `json:"severity"`
	Description  string     `json:"description"`
	Status       string     `json:"status"` // "pending", "resolved", "ignored"
	DetectedAt   time.Time  `json:"detected_at"`
	ResolvedAt   *time.Time `json:"resolved_at,omitempty"`
}

// KnowledgeNode represents a node in the knowledge graph
type KnowledgeNode struct {
	ID       int64    `json:"id"`
	Content  string   `json:"content"`
	Keywords []string `json:"keywords"`
	Tags     []string `json:"tags"`
	Type     string   `json:"type"`
	Weight   float64  `json:"weight"`
}

// KnowledgeEdge represents an edge in the knowledge graph
type KnowledgeEdge struct {
	SourceID   int64   `json:"source_id"`
	TargetID   int64   `json:"target_id"`
	Weight     float64 `json:"weight"`
	Type       string  `json:"type"`
	Confidence float64 `json:"confidence"`
}

// MapStatistics contains statistics about the knowledge map
type MapStatistics struct {
	NodeCount int     `json:"node_count"`
	EdgeCount int     `json:"edge_count"`
	Density   float64 `json:"density"`
	AvgDegree float64 `json:"avg_degree"`
	MaxDepth  int     `json:"max_depth"`
}

// SimpleKnowledgeMap represents a simple knowledge graph (avoiding conflict with graph_memory.go)
type SimpleKnowledgeMap struct {
	Nodes []KnowledgeNode `json:"nodes"`
	Edges []KnowledgeEdge `json:"edges"`
	Stats MapStatistics   `json:"stats"`
}

// MemoryEngine defines the interface for memory operations
type MemoryEngine interface {
	IngestAgenticMemory(ctx context.Context, cfg *configpkg.Config, txt string, wf uuid.UUID) (int64, error)
	SearchWithinWorkflow(ctx context.Context, cfg *configpkg.Config, wf uuid.UUID, q string, k int) ([]AgenticMemory, error)
	EnsureAgenticMemoryTable(ctx context.Context, embeddingDim int) error

	// Advanced graph-based memory methods
	DiscoverMemoryClusters(ctx context.Context, cfg *configpkg.Config, wf uuid.UUID, minClusterSize int) ([]MemoryCluster, error)
	AnalyzeMemoryNetworkHealth(ctx context.Context, cfg *configpkg.Config, wf uuid.UUID) (*NetworkHealth, error)
	BuildKnowledgeMap(ctx context.Context, cfg *configpkg.Config, wf uuid.UUID, depth int) (*SimpleKnowledgeMap, error)
	FindMemoryPath(ctx context.Context, startMemoryID, endMemoryID int64) ([]int64, error)
	FindRelatedMemories(ctx context.Context, memoryID int64, hops int, limit int) ([]AgenticMemory, error)
}

// NilMemoryEngine is a no-op implementation of MemoryEngine
type NilMemoryEngine struct{}

func (n *NilMemoryEngine) IngestAgenticMemory(ctx context.Context, cfg *configpkg.Config, txt string, wf uuid.UUID) (int64, error) {
	return 0, nil
}

func (n *NilMemoryEngine) SearchWithinWorkflow(ctx context.Context, cfg *configpkg.Config, wf uuid.UUID, q string, k int) ([]AgenticMemory, error) {
	return nil, nil
}

func (n *NilMemoryEngine) EnsureAgenticMemoryTable(ctx context.Context, embeddingDim int) error {
	return nil
}

// Add new methods to NilMemoryEngine to satisfy extended interface
func (n *NilMemoryEngine) DiscoverMemoryClusters(ctx context.Context, cfg *configpkg.Config, wf uuid.UUID, minClusterSize int) ([]MemoryCluster, error) {
	return nil, nil
}

func (n *NilMemoryEngine) AnalyzeMemoryNetworkHealth(ctx context.Context, cfg *configpkg.Config, wf uuid.UUID) (*NetworkHealth, error) {
	return nil, nil
}

func (n *NilMemoryEngine) BuildKnowledgeMap(ctx context.Context, cfg *configpkg.Config, wf uuid.UUID, depth int) (*SimpleKnowledgeMap, error) {
	return nil, nil
}

func (n *NilMemoryEngine) FindMemoryPath(ctx context.Context, startMemoryID, endMemoryID int64) ([]int64, error) {
	return nil, nil
}

func (n *NilMemoryEngine) FindRelatedMemories(ctx context.Context, memoryID int64, hops int, limit int) ([]AgenticMemory, error) {
	return nil, nil
}

// AgenticEngine handles agentic memory operations.
type AgenticEngine struct {
	DB *pgx.Conn
}

// NewAgenticEngine returns a new agentic engine instance.
func NewAgenticEngine(db *pgx.Conn) *AgenticEngine {
	return &AgenticEngine{DB: db}
}

// EnsureAgenticMemoryTable creates the table if it does not exist *or* patches it
func (ae *AgenticEngine) EnsureAgenticMemoryTable(ctx context.Context, embeddingDim int) error {
	// 1) create if missing
	_, err := ae.DB.Exec(ctx, fmt.Sprintf(`
        CREATE TABLE IF NOT EXISTS agentic_memories (
            id           SERIAL PRIMARY KEY,
            workflow_id  UUID,                       -- <<< NEW
            content      TEXT        NOT NULL,
            note_context TEXT,
            keywords     TEXT[],
            tags         TEXT[],
            timestamp    TIMESTAMP,
            embedding    vector(%d) NOT NULL,
            links        INTEGER[]
        );`, embeddingDim))
	if err != nil {
		return err
	}

	// 2) patch older deployments that don’t have workflow_id yet
	_, _ = ae.DB.Exec(ctx, `ALTER TABLE agentic_memories
                            ADD COLUMN IF NOT EXISTS workflow_id UUID;`)
	// 3) index for fast “same-session” look-ups
	_, _ = ae.DB.Exec(ctx, `
        CREATE INDEX IF NOT EXISTS agentic_memories_workflow_ts_idx
        ON agentic_memories (workflow_id, timestamp DESC);`)

	// Create memory evolution table for tracking concept development
	_, err = ae.DB.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS memory_evolution (
			id BIGSERIAL PRIMARY KEY,
			original_id BIGINT NOT NULL REFERENCES agentic_memories(id) ON DELETE CASCADE,
			evolved_id BIGINT NOT NULL REFERENCES agentic_memories(id) ON DELETE CASCADE,
			evolution_type VARCHAR(50) NOT NULL,
			confidence DOUBLE PRECISION DEFAULT 0.5,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			metadata JSONB,
			CONSTRAINT evolution_type_check CHECK (evolution_type IN ('refinement', 'contradiction', 'expansion', 'merger'))
		);
	`)
	if err != nil {
		log.Printf("Warning: failed to create memory_evolution table: %v", err)
	}

	// Create memory contradictions table for conflict detection
	_, err = ae.DB.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS memory_contradictions (
			id BIGSERIAL PRIMARY KEY,
			memory1_id BIGINT NOT NULL REFERENCES agentic_memories(id) ON DELETE CASCADE,
			memory2_id BIGINT NOT NULL REFERENCES agentic_memories(id) ON DELETE CASCADE,
			conflict_type VARCHAR(50) NOT NULL,
			severity DOUBLE PRECISION DEFAULT 0.5,
			description TEXT,
			status VARCHAR(20) DEFAULT 'pending',
			detected_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			resolved_at TIMESTAMP,
			CONSTRAINT contradiction_status_check CHECK (status IN ('pending', 'resolved', 'ignored')),
			CONSTRAINT contradiction_severity_check CHECK (severity >= 0 AND severity <= 1),
			UNIQUE(memory1_id, memory2_id)
		);
	`)
	if err != nil {
		log.Printf("Warning: failed to create memory_contradictions table: %v", err)
	}

	// Create indices for performance
	_, _ = ae.DB.Exec(ctx, `CREATE INDEX IF NOT EXISTS idx_memory_evolution_original ON memory_evolution(original_id);`)
	_, _ = ae.DB.Exec(ctx, `CREATE INDEX IF NOT EXISTS idx_memory_evolution_evolved ON memory_evolution(evolved_id);`)
	_, _ = ae.DB.Exec(ctx, `CREATE INDEX IF NOT EXISTS idx_memory_contradictions_memory1 ON memory_contradictions(memory1_id);`)
	_, _ = ae.DB.Exec(ctx, `CREATE INDEX IF NOT EXISTS idx_memory_contradictions_memory2 ON memory_contradictions(memory2_id);`)
	_, _ = ae.DB.Exec(ctx, `CREATE INDEX IF NOT EXISTS idx_memory_contradictions_status ON memory_contradictions(status);`)

	return nil
}

// IngestAgenticMemory ingests a new memory note.
func (ae *AgenticEngine) IngestAgenticMemory(
	ctx context.Context,
	config *configpkg.Config,
	content string,
	workflowID uuid.UUID,
) (int64, error) {
	log.Println("Ingesting agentic memory note...")
	log.Println(content)
	// 1. Use an LLM (or completions endpoint) to generate note context, keywords, and tags.
	summaryOutput, err := sefii.SummarizeChunk(ctx, content, "http://192.168.1.244:32182/v1/chat/completions", config.Completions.CompletionsModel, config.Completions.APIKey)
	if err != nil {
		log.Printf("AgenticMemory: Failed to summarize content: %v", err)
		// If summarization fails, proceed with empty context.
		summaryOutput.Summary = ""
	}
	noteContext := summaryOutput.Summary
	keywords := summaryOutput.Keywords

	// if the keywords contain 'encoded data, encrypted text, unreadable content' then immediately return
	if len(keywords) == 0 {
		log.Printf("AgenticMemory: No keywords found in summary output")
		return 0, fmt.Errorf("no keywords found in summary output")
	}
	// If the keywords contain 'encoded data, encrypted text, unreadable content' then immediately return
	if strings.Contains(strings.Join(keywords, " "), "encoded data") ||
		strings.Contains(strings.Join(keywords, " "), "encrypted text") ||
		strings.Contains(strings.Join(keywords, " "), "unreadable content") {
		log.Printf("AgenticMemory: Keywords contain unreadable content")
		return 0, fmt.Errorf("keywords contain unreadable content")
	}

	// For tags, here we simply reuse keywords. Adjust as needed.
	tags := keywords

	// 2. Compute the embedding.
	embeddingInput := config.Embeddings.EmbedPrefix + content + " " + noteContext + " " + strings.Join(keywords, " ") + " " + strings.Join(tags, " ")
	embeds, err := llm.GenerateEmbeddings(config.Embeddings.Host, config.Embeddings.APIKey, []string{embeddingInput})
	if err != nil || len(embeds) == 0 {
		return 0, fmt.Errorf("failed to generate embedding: %w", err)
	}
	vec := pgvector.NewVector(embeds[0])

	// 3. Insert the new memory note.
	currentTime := time.Now()
	var newID int64
	insertQuery := `
        INSERT INTO agentic_memories
            (workflow_id, content, note_context, keywords, tags, timestamp, embedding, links)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
        RETURNING id`
	emptyLinks := []int64{}
	err = ae.DB.QueryRow(ctx, insertQuery, workflowID, content, noteContext, keywords, tags, currentTime, vec, emptyLinks).Scan(&newID)
	if err != nil {
		return 0, fmt.Errorf("failed to insert agentic memory note: %w", err)
	}

	// 4. Generate links for the new note.
	relatedIDs, err := ae.generateLinks(ctx, newID, 5)
	// relatedIDs, err := ae.generateLinks(ctx, newID, 5, keywords)
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
func (ae *AgenticEngine) generateLinks(ctx context.Context, newMemoryID int64, k int) ([]int64, error) {
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
func (ae *AgenticEngine) SearchAgenticMemories(ctx context.Context, config *configpkg.Config, queryText string, limit int) ([]AgenticMemory, error) {
	embeds, err := llm.GenerateEmbeddings(config.Embeddings.Host, config.Embeddings.APIKey, []string{queryText})
	if err != nil || len(embeds) == 0 {
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}
	queryVec := pgvector.NewVector(embeds[0])

	// Cast keywords and tags to text to force string output.
	searchQuery := `
        SELECT id, workflow_id, content, note_context, keywords::text, tags::text, timestamp, embedding, links
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
		err := rows.Scan(&mem.ID, &mem.WorkflowID, &mem.Content, &mem.NoteContext, &kwStr, &tagStr, &ts, &mem.Embedding, &mem.Links)
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

// AgenticMemoryIngestHandler handles POST /api/agentic-memory/ingest.
func AgenticMemoryIngestHandler(config *configpkg.Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		// Check if agentic memory is enabled
		if !config.AgenticMemory.Enabled {
			return c.JSON(http.StatusForbidden, map[string]string{"error": "Agentic memory is disabled in configuration"})
		}

		var req struct {
			Content           string `json:"content"`
			WorkflowID        string `json:"workflow_id"` // New parameter for workflow ID
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

		// Parse workflow ID if provided
		var workflowID uuid.UUID
		var err error
		if req.WorkflowID != "" {
			workflowID, err = uuid.Parse(req.WorkflowID)
			if err != nil {
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid workflow_id format"})
			}
		} // If empty, it will be the zero UUID (represents global memory)

		ctx := c.Request().Context()

		// Use the connection pool instead of creating a new connection
		if config.DBPool == nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Database connection pool not initialized"})
		}

		// Get a connection from the pool
		conn, err := config.DBPool.Acquire(ctx)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to acquire database connection"})
		}
		// Return the connection to the pool when done
		defer conn.Release()

		engine := NewAgenticEngine(conn.Conn())
		if err := engine.EnsureAgenticMemoryTable(ctx, config.Embeddings.Dimensions); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Failed to ensure agentic memory table: %v", err)})
		}
		newID, err := engine.IngestAgenticMemory(ctx, config, req.Content, workflowID)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Failed to ingest agentic memory: %v", err)})
		}
		return c.JSON(http.StatusOK, map[string]interface{}{"message": "Agentic memory ingested successfully", "id": newID})
	}
}

// AgenticMemorySearchHandler handles POST /api/agentic-memory/search.
func AgenticMemorySearchHandler(config *configpkg.Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		// Check if agentic memory is enabled
		if !config.AgenticMemory.Enabled {
			return c.JSON(http.StatusForbidden, map[string]string{"error": "Agentic memory is disabled in configuration"})
		}

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

		// Use the connection pool instead of creating a new connection
		if config.DBPool == nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Database connection pool not initialized"})
		}

		// Get a connection from the pool
		conn, err := config.DBPool.Acquire(ctx)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to acquire database connection"})
		}
		// Return the connection to the pool when done
		defer conn.Release()

		engine := NewAgenticEngine(conn.Conn())

		searchQuery := fmt.Sprintf("%s%s", config.Embeddings.SearchPrefix, req.Query)

		results, err := engine.SearchAgenticMemories(ctx, config, searchQuery, req.Limit)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Failed to search agentic memories: %v", err)})
		}
		return c.JSON(http.StatusOK, map[string]interface{}{"results": results})
	}
}

// AgenticMemoryUpdateHandler handles POST /api/agentic-memory/update/:id
func AgenticMemoryUpdateHandler(config *configpkg.Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		if !config.AgenticMemory.Enabled {
			return c.JSON(http.StatusForbidden, map[string]string{"error": "Agentic memory is disabled in configuration"})
		}
		idStr := c.Param("id")
		memID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid memory ID"})
		}
		var req struct {
			Content       string `json:"content"`
			WorkflowID    string `json:"workflow_id"`
			EvolutionType string `json:"evolution_type"`
		}
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		}
		if req.Content == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Content is required"})
		}
		if req.EvolutionType == "" {
			req.EvolutionType = "refinement"
		}
		workflowID, err := uuid.Parse(req.WorkflowID)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid workflow ID"})
		}
		ctx := c.Request().Context()
		if config.DBPool == nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Database connection pool not initialized"})
		}
		conn, err := config.DBPool.Acquire(ctx)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to acquire database connection"})
		}
		defer conn.Release()
		engine := NewAgenticEngine(conn.Conn())
		if err := engine.EnsureAgenticMemoryTable(ctx, config.Embeddings.Dimensions); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to ensure agentic memory table"})
		}
		newID, err := engine.UpdateMemoryWithEvolution(ctx, config, memID, req.Content, workflowID, req.EvolutionType)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Failed to update memory: %v", err)})
		}
		return c.JSON(http.StatusOK, map[string]interface{}{"evolved_id": newID})
	}
}

// SearchWithinWorkflow finds memories for the same workflow only.
func (ae *AgenticEngine) SearchWithinWorkflow(
	ctx context.Context,
	cfg *configpkg.Config,
	workflowID uuid.UUID,
	query string,
	k int,
) ([]AgenticMemory, error) {

	embeds, err := llm.GenerateEmbeddings(cfg.Embeddings.Host, cfg.Embeddings.APIKey, []string{query})
	if err != nil || len(embeds) == 0 {
		return nil, err
	}

	qvec := pgvector.NewVector(embeds[0])
	rows, err := ae.DB.Query(ctx, `
        SELECT id, workflow_id, content, note_context, timestamp
        FROM agentic_memories
        WHERE workflow_id = $1
        ORDER BY embedding <-> $2
        LIMIT $3`,
		workflowID, qvec, k)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var memories []AgenticMemory
	for rows.Next() {
		var am AgenticMemory
		if err := rows.Scan(&am.ID, &am.WorkflowID, &am.Content, &am.NoteContext, &am.Timestamp); err != nil {
			return nil, err
		}
		memories = append(memories, am)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return memories, nil
}

// DiscoverMemoryClusters uses graph analysis to find clusters of related memories
func (ae *AgenticEngine) DiscoverMemoryClusters(ctx context.Context, cfg *configpkg.Config, wf uuid.UUID, minClusterSize int) ([]MemoryCluster, error) {
	// First ensure we have the enhanced tables
	eae := NewEnhancedAgenticEngine(ae.DB)
	err := eae.EnsureEnhancedMemoryTables(ctx, cfg.Embeddings.Dimensions)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure enhanced tables: %w", err)
	}

	// Migrate existing memories to enhanced format if needed
	err = ae.migrateToEnhancedMemories(ctx, wf)
	if err != nil {
		log.Printf("Warning: migration to enhanced memories failed: %v", err)
	}

	// Use pgRouting to find connected components (clusters)
	query := `
		WITH components AS (
			SELECT node, component
			FROM pgr_connectedComponents(
				'SELECT id, source as source, target as target FROM memory_edges 
				 JOIN memory_nodes s ON source = s.id 
				 JOIN memory_nodes t ON target = t.id 
				 WHERE s.workflow_id = ''' || $1 || ''' AND t.workflow_id = ''' || $1 || ''''
			)
		),
		cluster_sizes AS (
			SELECT component, COUNT(*) as size
			FROM components
			GROUP BY component
			HAVING COUNT(*) >= $2
		)
		SELECT c.component, c.node
		FROM components c
		JOIN cluster_sizes cs ON c.component = cs.component
		ORDER BY c.component, c.node`

	rows, err := ae.DB.Query(ctx, query, wf.String(), minClusterSize)
	if err != nil {
		return nil, fmt.Errorf("failed to discover clusters: %w", err)
	}
	defer rows.Close()

	clusterMap := make(map[int][]int64)
	for rows.Next() {
		var component int
		var nodeID int64
		if err := rows.Scan(&component, &nodeID); err != nil {
			continue
		}
		clusterMap[component] = append(clusterMap[component], nodeID)
	}

	// Convert to MemoryCluster format
	var clusters []MemoryCluster
	for clusterID, nodeIDs := range clusterMap {
		if len(nodeIDs) < minClusterSize {
			continue
		}

		cluster := MemoryCluster{
			ID:        clusterID,
			CenterID:  nodeIDs[0], // Use first node as center for now
			Members:   []AgenticMemory{},
			Themes:    []string{},
			CreatedAt: time.Now(),
		}

		// Fetch full memory details for cluster members
		for _, nodeID := range nodeIDs {
			var mem AgenticMemory
			err := ae.DB.QueryRow(ctx, `
				SELECT id, workflow_id, content, note_context, keywords, tags, timestamp, embedding, 
				       COALESCE(links, ARRAY[]::INTEGER[])
				FROM agentic_memories WHERE id = $1`, nodeID).Scan(
				&mem.ID, &mem.WorkflowID, &mem.Content, &mem.NoteContext,
				&mem.Keywords, &mem.Tags, &mem.Timestamp, &mem.Embedding, &mem.Links)
			if err == nil {
				cluster.Members = append(cluster.Members, mem)
				// Add keywords to themes
				cluster.Themes = append(cluster.Themes, mem.Keywords...)
			}
		}

		// Calculate cluster confidence based on internal connectivity
		cluster.Confidence = ae.calculateClusterConfidence(ctx, nodeIDs)
		clusters = append(clusters, cluster)
	}

	return clusters, nil
}

// AnalyzeMemoryNetworkHealth provides comprehensive network health metrics
func (ae *AgenticEngine) AnalyzeMemoryNetworkHealth(ctx context.Context, cfg *configpkg.Config, wf uuid.UUID) (*NetworkHealth, error) {
	health := &NetworkHealth{}

	// Count total memories
	err := ae.DB.QueryRow(ctx, `SELECT COUNT(*) FROM agentic_memories WHERE workflow_id = $1`, wf).Scan(&health.TotalMemories)
	if err != nil {
		return nil, fmt.Errorf("failed to count memories: %w", err)
	}

	// Count total connections (based on links array)
	err = ae.DB.QueryRow(ctx, `
		SELECT COALESCE(SUM(array_length(links, 1)), 0) 
		FROM agentic_memories 
		WHERE workflow_id = $1 AND links IS NOT NULL`, wf).Scan(&health.TotalConnections)
	if err != nil {
		return nil, fmt.Errorf("failed to count connections: %w", err)
	}

	// Calculate average connectivity
	if health.TotalMemories > 0 {
		health.AverageConnectivity = float64(health.TotalConnections) / float64(health.TotalMemories)
	}

	// Count isolated memories (those with no links)
	err = ae.DB.QueryRow(ctx, `
		SELECT COUNT(*) 
		FROM agentic_memories 
		WHERE workflow_id = $1 AND (links IS NULL OR array_length(links, 1) = 0)`, wf).Scan(&health.IsolatedMemories)
	if err != nil {
		return nil, fmt.Errorf("failed to count isolated memories: %w", err)
	}

	// Detect contradictions using semantic analysis
	health.Contradictions = ae.detectContradictions(ctx, wf)

	// Calculate clustering score (how well-connected the network is)
	health.ClusteringScore = ae.calculateClusteringScore(ctx, wf)

	// Calculate overall health score
	health.HealthScore = ae.calculateOverallHealthScore(health)

	return health, nil
}

// BuildKnowledgeMap creates a knowledge graph structure from memories
func (ae *AgenticEngine) BuildKnowledgeMap(ctx context.Context, cfg *configpkg.Config, wf uuid.UUID, depth int) (*SimpleKnowledgeMap, error) {
	knowledgeMap := &SimpleKnowledgeMap{
		Nodes: []KnowledgeNode{},
		Edges: []KnowledgeEdge{},
		Stats: MapStatistics{},
	}

	// Get all memories for the workflow
	rows, err := ae.DB.Query(ctx, `
		SELECT id, content, note_context, keywords, tags, links, timestamp
		FROM agentic_memories 
		WHERE workflow_id = $1
		ORDER BY timestamp DESC`, wf)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch memories: %w", err)
	}
	defer rows.Close()

	nodeMap := make(map[int64]bool)

	for rows.Next() {
		var mem AgenticMemory
		var linksArray []int64
		err := rows.Scan(&mem.ID, &mem.Content, &mem.NoteContext,
			&mem.Keywords, &mem.Tags, &linksArray, &mem.Timestamp)
		if err != nil {
			continue
		}

		// Create knowledge node
		node := KnowledgeNode{
			ID:       mem.ID,
			Content:  mem.Content,
			Keywords: mem.Keywords,
			Tags:     mem.Tags,
			Type:     "memory",
			Weight:   float64(len(mem.Keywords) + len(linksArray)), // Weight based on richness
		}
		knowledgeMap.Nodes = append(knowledgeMap.Nodes, node)
		nodeMap[mem.ID] = true

		// Create edges from links
		for _, linkedID := range linksArray {
			if nodeMap[linkedID] || linkedID == mem.ID {
				continue // Skip self-links and already processed
			}

			edge := KnowledgeEdge{
				SourceID:   mem.ID,
				TargetID:   linkedID,
				Weight:     1.0,
				Type:       "linked",
				Confidence: 0.8,
			}
			knowledgeMap.Edges = append(knowledgeMap.Edges, edge)
		}
	}

	// Calculate statistics
	knowledgeMap.Stats.NodeCount = len(knowledgeMap.Nodes)
	knowledgeMap.Stats.EdgeCount = len(knowledgeMap.Edges)
	if knowledgeMap.Stats.NodeCount > 1 {
		maxPossibleEdges := knowledgeMap.Stats.NodeCount * (knowledgeMap.Stats.NodeCount - 1) / 2
		knowledgeMap.Stats.Density = float64(knowledgeMap.Stats.EdgeCount) / float64(maxPossibleEdges)
	}
	if knowledgeMap.Stats.NodeCount > 0 {
		knowledgeMap.Stats.AvgDegree = float64(knowledgeMap.Stats.EdgeCount*2) / float64(knowledgeMap.Stats.NodeCount)
	}
	knowledgeMap.Stats.MaxDepth = depth

	return knowledgeMap, nil
}

// FindMemoryPath finds the shortest path between two memories using graph traversal
func (ae *AgenticEngine) FindMemoryPath(ctx context.Context, startMemoryID, endMemoryID int64) ([]int64, error) {
	// Use a simple BFS approach since we don't have pgRouting on the main agentic_memories table
	visited := make(map[int64]bool)
	queue := [][]int64{{startMemoryID}}

	for len(queue) > 0 {
		path := queue[0]
		queue = queue[1:]

		currentID := path[len(path)-1]
		if currentID == endMemoryID {
			return path, nil
		}

		if visited[currentID] {
			continue
		}
		visited[currentID] = true

		// Get linked memories
		var links []int64
		err := ae.DB.QueryRow(ctx, `SELECT COALESCE(links, ARRAY[]::INTEGER[]) FROM agentic_memories WHERE id = $1`, currentID).Scan(&links)
		if err != nil {
			continue
		}

		for _, linkedID := range links {
			if !visited[linkedID] {
				newPath := make([]int64, len(path)+1)
				copy(newPath, path)
				newPath[len(path)] = linkedID
				queue = append(queue, newPath)
			}
		}

		// Limit search depth to prevent infinite loops
		if len(path) > 10 {
			break
		}
	}

	return nil, fmt.Errorf("no path found between memories %d and %d", startMemoryID, endMemoryID)
}

// FindRelatedMemories discovers memories related within a certain number of hops
func (ae *AgenticEngine) FindRelatedMemories(ctx context.Context, memoryID int64, hops int, limit int) ([]AgenticMemory, error) {
	visited := make(map[int64]bool)
	queue := []int64{memoryID}
	currentHop := 0
	var relatedIDs []int64

	for currentHop < hops && len(queue) > 0 {
		nextQueue := []int64{}

		for _, currentID := range queue {
			if visited[currentID] {
				continue
			}
			visited[currentID] = true

			if currentID != memoryID {
				relatedIDs = append(relatedIDs, currentID)
			}

			// Get linked memories
			var links []int64
			err := ae.DB.QueryRow(ctx, `SELECT COALESCE(links, ARRAY[]::INTEGER[]) FROM agentic_memories WHERE id = $1`, currentID).Scan(&links)
			if err != nil {
				continue
			}

			for _, linkedID := range links {
				if !visited[linkedID] {
					nextQueue = append(nextQueue, linkedID)
				}
			}
		}

		queue = nextQueue
		currentHop++
	}

	// Limit results
	if len(relatedIDs) > limit {
		relatedIDs = relatedIDs[:limit]
	}

	// Fetch full memory objects
	var memories []AgenticMemory
	for _, id := range relatedIDs {
		var mem AgenticMemory
		err := ae.DB.QueryRow(ctx, `
			SELECT id, workflow_id, content, note_context, keywords, tags, timestamp, embedding, links
			FROM agentic_memories WHERE id = $1`, id).Scan(
			&mem.ID, &mem.WorkflowID, &mem.Content, &mem.NoteContext,
			&mem.Keywords, &mem.Tags, &mem.Timestamp, &mem.Embedding, &mem.Links)
		if err == nil {
			memories = append(memories, mem)
		}
	}

	return memories, nil
}

// TrackMemoryEvolution creates an evolution relationship when a memory is updated
func (ae *AgenticEngine) TrackMemoryEvolution(ctx context.Context, originalID, evolvedID int64, evolutionType string, confidence float64) error {
	_, err := ae.DB.Exec(ctx, `
		INSERT INTO memory_evolution (original_id, evolved_id, evolution_type, confidence, metadata)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT DO NOTHING`,
		originalID, evolvedID, evolutionType, confidence,
		map[string]interface{}{
			"tracked_at": time.Now(),
		})
	if err != nil {
		return fmt.Errorf("failed to track memory evolution: %w", err)
	}

	// Also create an "evolved" relationship in the enhanced graph if available
	eae := NewEnhancedAgenticEngine(ae.DB)
	_, err = eae.DB.Exec(ctx, `
		INSERT INTO memory_edges (source, target, relationship_type, cost, confidence, evidence)
		SELECT $1, $2, 'evolved', 0.1, $3, $4
		WHERE EXISTS (SELECT 1 FROM memory_nodes WHERE id = $1)
		  AND EXISTS (SELECT 1 FROM memory_nodes WHERE id = $2)
		ON CONFLICT (source, target, relationship_type) DO NOTHING`,
		originalID, evolvedID, confidence,
		fmt.Sprintf("Memory evolution: %s", evolutionType))

	return nil
}

// UpdateMemoryWithEvolution updates a memory and tracks the evolution
func (ae *AgenticEngine) UpdateMemoryWithEvolution(
	ctx context.Context,
	config *configpkg.Config,
	memoryID int64,
	newContent string,
	workflowID uuid.UUID,
	evolutionType string,
) (int64, error) {
	// First, ingest the new memory
	newMemoryID, err := ae.IngestAgenticMemory(ctx, config, newContent, workflowID)
	if err != nil {
		return 0, fmt.Errorf("failed to ingest evolved memory: %w", err)
	}

	// Calculate confidence based on semantic similarity between old and new content
	var oldContent string
	err = ae.DB.QueryRow(ctx, `SELECT content FROM agentic_memories WHERE id = $1`, memoryID).Scan(&oldContent)
	if err != nil {
		return newMemoryID, fmt.Errorf("failed to retrieve original memory: %w", err)
	}

	// Generate embeddings for similarity calculation
	embeds, err := llm.GenerateEmbeddings(config.Embeddings.Host, config.Embeddings.APIKey, []string{oldContent, newContent})
	if err != nil {
		log.Printf("Warning: could not calculate evolution confidence: %v", err)
		// Track evolution with default confidence
		err = ae.TrackMemoryEvolution(ctx, memoryID, newMemoryID, evolutionType, 0.7)
		if err != nil {
			log.Printf("Warning: failed to track memory evolution: %v", err)
		}
		return newMemoryID, nil
	}

	// Calculate cosine similarity (confidence)
	confidence := 1.0 - calculateDistance(embeds[0], embeds[1])
	if confidence < 0 {
		confidence = 0
	}

	// Track the evolution
	err = ae.TrackMemoryEvolution(ctx, memoryID, newMemoryID, evolutionType, confidence)
	if err != nil {
		log.Printf("Warning: failed to track memory evolution: %v", err)
	}

	return newMemoryID, nil
}

// DetectMemoryContradictions analyzes memories for potential conflicts
func (ae *AgenticEngine) DetectMemoryContradictions(ctx context.Context, config *configpkg.Config, workflowID uuid.UUID) ([]MemoryContradiction, error) {
	// Find pairs of memories with high semantic similarity but potentially contradictory content
	rows, err := ae.DB.Query(ctx, `
		WITH memory_pairs AS (
			SELECT 
				m1.id as id1, m1.content as content1, m1.embedding as emb1,
				m2.id as id2, m2.content as content2, m2.embedding as emb2,
				m1.embedding <-> m2.embedding as distance
			FROM agentic_memories m1
			JOIN agentic_memories m2 ON m1.id < m2.id
			WHERE m1.workflow_id = $1 AND m2.workflow_id = $1
			  AND m1.embedding <-> m2.embedding < 0.3  -- semantically similar
			  AND NOT EXISTS (
				  SELECT 1 FROM memory_contradictions mc 
				  WHERE (mc.memory1_id = m1.id AND mc.memory2_id = m2.id)
				     OR (mc.memory1_id = m2.id AND mc.memory2_id = m1.id)
			  )
		)
		SELECT id1, content1, id2, content2, distance
		FROM memory_pairs
		ORDER BY distance
		LIMIT 20`, workflowID)
	if err != nil {
		return nil, fmt.Errorf("failed to query potential contradictions: %w", err)
	}
	defer rows.Close()

	var contradictions []MemoryContradiction
	for rows.Next() {
		var id1, id2 int64
		var content1, content2 string
		var distance float64

		err := rows.Scan(&id1, &content1, &id2, &content2, &distance)
		if err != nil {
			continue
		}

		// Use LLM to analyze if contents are contradictory
		isContradictory, severity, description := ae.analyzeContradiction(ctx, config, content1, content2)
		if isContradictory {
			// Record the contradiction
			var contradictionID int64
			err = ae.DB.QueryRow(ctx, `
				INSERT INTO memory_contradictions (memory1_id, memory2_id, conflict_type, severity, description)
				VALUES ($1, $2, $3, $4, $5)
				RETURNING id`,
				id1, id2, "semantic_conflict", severity, description).Scan(&contradictionID)
			if err != nil {
				log.Printf("Warning: failed to record contradiction: %v", err)
				continue
			}

			contradictions = append(contradictions, MemoryContradiction{
				ID:           contradictionID,
				Memory1ID:    id1,
				Memory2ID:    id2,
				ConflictType: "semantic_conflict",
				Severity:     severity,
				Description:  description,
				Status:       "pending",
				DetectedAt:   time.Now(),
			})
		}
	}

	return contradictions, nil
}

// analyzeContradiction uses LLM to determine if two memory contents contradict each other
func (ae *AgenticEngine) analyzeContradiction(ctx context.Context, config *configpkg.Config, content1, content2 string) (bool, float64, string) {
	prompt := fmt.Sprintf(`Analyze these two memory contents for contradictions:

Memory 1: %s

Memory 2: %s

Determine if these memories contradict each other. Consider:
1. Do they make opposing claims about the same thing?
2. Are there factual inconsistencies?
3. Do they suggest incompatible actions or conclusions?

Respond with:
CONTRADICTORY: true/false
SEVERITY: 0.0-1.0 (how severe the contradiction is)
DESCRIPTION: Brief explanation of the contradiction (if any)`, content1, content2)

	// Use the LLM API to analyze the contradiction
	messages := []openai.ChatCompletionMessage{
		{Role: "system", Content: "You are an expert at analyzing contradictions in information. Analyze the following memories and determine if they contradict each other."},
		{Role: "user", Content: prompt},
	}

	response, err := llm.CallLLM(ctx, config.Completions.DefaultHost, config.Completions.APIKey, config.Completions.CompletionsModel, messages, 1024, 0.3)
	if err != nil {
		log.Printf("Warning: failed to analyze contradiction: %v", err)
		return false, 0.0, ""
	}

	// Parse response (simple pattern matching)
	responseText := strings.ToLower(response)

	// Check if contradictory
	isContradictory := strings.Contains(responseText, "contradictory: true")

	// Extract severity
	severity := 0.5 // default
	if severityMatch := regexp.MustCompile(`severity:\s*(\d+\.?\d*)`).FindStringSubmatch(responseText); len(severityMatch) > 1 {
		if s, err := strconv.ParseFloat(severityMatch[1], 64); err == nil {
			severity = s
		}
	}

	// Extract description
	description := "Potential contradiction detected via semantic analysis"
	if descMatch := regexp.MustCompile(`description:\s*(.+)`).FindStringSubmatch(response); len(descMatch) > 1 {
		description = strings.TrimSpace(descMatch[1])
	}

	return isContradictory, severity, description
}

// GetMemoryEvolutions retrieves evolution history for a memory
func (ae *AgenticEngine) GetMemoryEvolutions(ctx context.Context, memoryID int64) ([]MemoryEvolution, error) {
	rows, err := ae.DB.Query(ctx, `
		SELECT id, original_id, evolved_id, evolution_type, confidence, created_at
		FROM memory_evolution
		WHERE original_id = $1 OR evolved_id = $1
		ORDER BY created_at DESC`, memoryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var evolutions []MemoryEvolution
	for rows.Next() {
		var evolution MemoryEvolution
		err := rows.Scan(&evolution.ID, &evolution.OriginalID, &evolution.EvolvedID,
			&evolution.EvolutionType, &evolution.Confidence, &evolution.CreatedAt)
		if err != nil {
			continue
		}
		evolutions = append(evolutions, evolution)
	}

	return evolutions, nil
}

// GetPendingContradictions retrieves unresolved memory contradictions
func (ae *AgenticEngine) GetPendingContradictions(ctx context.Context, workflowID uuid.UUID) ([]MemoryContradiction, error) {
	rows, err := ae.DB.Query(ctx, `
		SELECT mc.id, mc.memory1_id, mc.memory2_id, mc.conflict_type, mc.severity, 
		       mc.description, mc.status, mc.detected_at, mc.resolved_at
		FROM memory_contradictions mc
		JOIN agentic_memories m1 ON mc.memory1_id = m1.id
		JOIN agentic_memories m2 ON mc.memory2_id = m2.id
		WHERE (m1.workflow_id = $1 OR m2.workflow_id = $1)
		  AND mc.status = 'pending'
		ORDER BY mc.severity DESC, mc.detected_at DESC`, workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contradictions []MemoryContradiction
	for rows.Next() {
		var contradiction MemoryContradiction
		err := rows.Scan(&contradiction.ID, &contradiction.Memory1ID, &contradiction.Memory2ID,
			&contradiction.ConflictType, &contradiction.Severity, &contradiction.Description,
			&contradiction.Status, &contradiction.DetectedAt, &contradiction.ResolvedAt)
		if err != nil {
			continue
		}
		contradictions = append(contradictions, contradiction)
	}

	return contradictions, nil
}

// ResolveContradiction marks a contradiction as resolved
func (ae *AgenticEngine) ResolveContradiction(ctx context.Context, contradictionID int64, resolution string) error {
	now := time.Now()
	_, err := ae.DB.Exec(ctx, `
		UPDATE memory_contradictions 
		SET status = 'resolved', resolved_at = $1, description = description || ' | Resolution: ' || $2
		WHERE id = $3`, now, resolution, contradictionID)

	return err
}

// MemoryContradictionsHandler runs contradiction detection for a workflow
func MemoryContradictionsHandler(config *configpkg.Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		if !config.AgenticMemory.Enabled {
			return c.JSON(http.StatusForbidden, map[string]string{"error": "Agentic memory is disabled in configuration"})
		}
		workflowStr := c.Param("workflowId")
		wf, err := uuid.Parse(workflowStr)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid workflow ID"})
		}
		ctx := c.Request().Context()
		if config.DBPool == nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Database connection pool not initialized"})
		}
		conn, err := config.DBPool.Acquire(ctx)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to acquire database connection"})
		}
		defer conn.Release()
		engine := NewAgenticEngine(conn.Conn())
		if err := engine.EnsureAgenticMemoryTable(ctx, config.Embeddings.Dimensions); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to ensure agentic memory table"})
		}
		contradictions, err := engine.DetectMemoryContradictions(ctx, config, wf)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusOK, map[string]interface{}{"contradictions": contradictions})
	}
}

// calculateDistance calculates cosine distance between two embeddings
func calculateDistance(emb1, emb2 []float32) float64 {
	if len(emb1) != len(emb2) {
		return 1.0 // maximum distance
	}

	var dotProduct, norm1, norm2 float64
	for i := range emb1 {
		dotProduct += float64(emb1[i]) * float64(emb2[i])
		norm1 += float64(emb1[i]) * float64(emb1[i])
		norm2 += float64(emb2[i]) * float64(emb2[i])
	}

	if norm1 == 0 || norm2 == 0 {
		return 1.0
	}

	similarity := dotProduct / (math.Sqrt(norm1) * math.Sqrt(norm2))
	return 1.0 - similarity // convert to distance
}

// Helper methods for network analysis and clustering

// migrateToEnhancedMemories migrates existing memories to the enhanced graph format
func (ae *AgenticEngine) migrateToEnhancedMemories(ctx context.Context, wf uuid.UUID) error {
	// This is a no-op for now - would migrate data to enhanced format if needed
	log.Printf("Migration to enhanced memories not needed for workflow %s", wf.String())
	return nil
}

// calculateClusterConfidence calculates confidence score for a memory cluster
func (ae *AgenticEngine) calculateClusterConfidence(ctx context.Context, nodeIDs []int64) float64 {
	if len(nodeIDs) < 2 {
		return 0.0
	}

	totalConnections := 0
	possibleConnections := len(nodeIDs) * (len(nodeIDs) - 1) / 2

	// Count actual connections between nodes
	for i, nodeID1 := range nodeIDs {
		var links []int64
		err := ae.DB.QueryRow(ctx, `SELECT COALESCE(links, ARRAY[]::INTEGER[]) FROM agentic_memories WHERE id = $1`, nodeID1).Scan(&links)
		if err != nil {
			continue
		}

		for j := i + 1; j < len(nodeIDs); j++ {
			nodeID2 := nodeIDs[j]
			for _, link := range links {
				if link == nodeID2 {
					totalConnections++
					break
				}
			}
		}
	}

	if possibleConnections == 0 {
		return 0.0
	}

	return float64(totalConnections) / float64(possibleConnections)
}

// detectContradictions detects contradictions in the workflow memories
func (ae *AgenticEngine) detectContradictions(ctx context.Context, wf uuid.UUID) int {
	var count int
	err := ae.DB.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM memory_contradictions mc
		JOIN agentic_memories m1 ON mc.memory1_id = m1.id
		JOIN agentic_memories m2 ON mc.memory2_id = m2.id
		WHERE (m1.workflow_id = $1 OR m2.workflow_id = $1)
		  AND mc.status = 'pending'`, wf).Scan(&count)
	if err != nil {
		log.Printf("Warning: failed to count contradictions: %v", err)
		return 0
	}
	return count
}

// calculateClusteringScore calculates how well-clustered the memory network is
func (ae *AgenticEngine) calculateClusteringScore(ctx context.Context, wf uuid.UUID) float64 {
	// Get all memories with their links
	rows, err := ae.DB.Query(ctx, `
		SELECT id, COALESCE(links, ARRAY[]::INTEGER[])
		FROM agentic_memories
		WHERE workflow_id = $1`, wf)
	if err != nil {
		log.Printf("Warning: failed to calculate clustering score: %v", err)
		return 0.0
	}
	defer rows.Close()

	nodeConnections := make(map[int64][]int64)
	totalNodes := 0

	for rows.Next() {
		var nodeID int64
		var links []int64
		if err := rows.Scan(&nodeID, &links); err != nil {
			continue
		}
		nodeConnections[nodeID] = links
		totalNodes++
	}

	if totalNodes < 3 {
		return 0.0 // Need at least 3 nodes for meaningful clustering
	}

	// Calculate local clustering coefficient for each node
	totalClustering := 0.0
	validNodes := 0

	for _, neighbors := range nodeConnections {
		if len(neighbors) < 2 {
			continue // Need at least 2 neighbors
		}

		// Count connections between neighbors
		neighborConnections := 0
		for i, neighbor1 := range neighbors {
			neighbor1Links := nodeConnections[neighbor1]
			for j := i + 1; j < len(neighbors); j++ {
				neighbor2 := neighbors[j]
				for _, link := range neighbor1Links {
					if link == neighbor2 {
						neighborConnections++
						break
					}
				}
			}
		}

		// Local clustering coefficient
		possibleConnections := len(neighbors) * (len(neighbors) - 1) / 2
		if possibleConnections > 0 {
			localClustering := float64(neighborConnections) / float64(possibleConnections)
			totalClustering += localClustering
			validNodes++
		}
	}

	if validNodes == 0 {
		return 0.0
	}

	return totalClustering / float64(validNodes)
}

// calculateOverallHealthScore calculates the overall health score
func (ae *AgenticEngine) calculateOverallHealthScore(health *NetworkHealth) float64 {
	if health.TotalMemories == 0 {
		return 0.0
	}

	// Factors for health calculation
	connectivityScore := math.Min(health.AverageConnectivity/5.0, 1.0) // Normalize to 5 connections per memory
	isolationPenalty := float64(health.IsolatedMemories) / float64(health.TotalMemories)
	clusteringBonus := health.ClusteringScore
	contradictionPenalty := math.Min(float64(health.Contradictions)/float64(health.TotalMemories), 0.5)

	// Weighted health score (0-1 scale)
	healthScore := (connectivityScore*0.3 + clusteringBonus*0.3 + (1.0-isolationPenalty)*0.3 + (1.0-contradictionPenalty)*0.1)

	return math.Max(0.0, math.Min(1.0, healthScore))
}

// End of helper methods
