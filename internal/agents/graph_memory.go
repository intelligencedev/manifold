// enhanced_agentic_memory.go
package agents

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"

	configpkg "manifold/internal/config"
	embeddings "manifold/internal/llm"
	"manifold/internal/sefii"
)

// Memory graph node types
const (
	NodeTypeMemory   = "memory"
	NodeTypeConcept  = "concept"
	NodeTypeDocument = "document"
	NodeTypeWorkflow = "workflow"
	NodeTypeAgent    = "agent"
)

// Relationship types for memory graph
const (
	RelationshipSimilar     = "similar"
	RelationshipDerived     = "derived"
	RelationshipContradicts = "contradicts"
	RelationshipReferences  = "references"
	RelationshipEvolved     = "evolved"
	RelationshipCauses      = "causes"
	RelationshipTemporal    = "temporal"
)

// Enhanced memory structures
type MemoryNode struct {
	ID          int64                  `json:"id"`
	NodeType    string                 `json:"node_type"`
	WorkflowID  string                 `json:"workflow_id"`
	Content     string                 `json:"content"`
	NoteContext string                 `json:"note_context"`
	Keywords    []string               `json:"keywords"`
	Tags        []string               `json:"tags"`
	Timestamp   time.Time              `json:"timestamp"`
	Embedding   pgvector.Vector        `json:"embedding"`
	Metadata    map[string]interface{} `json:"metadata"`
}

type MemoryEdge struct {
	ID               int64     `json:"id"`
	SourceID         int64     `json:"source_id"`
	TargetID         int64     `json:"target_id"`
	RelationshipType string    `json:"relationship_type"`
	Weight           float64   `json:"weight"`
	Confidence       float64   `json:"confidence"`
	CreatedAt        time.Time `json:"created_at"`
	Evidence         string    `json:"evidence"`
}

type MemoryPath struct {
	Nodes []MemoryNode `json:"nodes"`
	Edges []MemoryEdge `json:"edges"`
	Cost  float64      `json:"cost"`
	Hops  int          `json:"hops"`
}

// Enhanced memory engine interface
type EnhancedMemoryEngine interface {
	// Core memory operations
	IngestAgenticMemory(ctx context.Context, cfg *configpkg.Config, txt string, wf uuid.UUID) (int64, error)
	SearchWithinWorkflow(ctx context.Context, cfg *configpkg.Config, wf uuid.UUID, q string, k int) ([]AgenticMemory, error)

	// Graph operations
	FindMemoryPath(ctx context.Context, sourceID, targetID int64) (*MemoryPath, error)
	FindRelatedMemories(ctx context.Context, memoryID int64, maxHops int, relationshipTypes []string) ([]MemoryNode, error)
	DiscoverMemoryClusters(ctx context.Context, workflowID uuid.UUID, minClusterSize int) ([][]MemoryNode, error)
	TraceMemoryEvolution(ctx context.Context, conceptID int64) (*MemoryPath, error)
	FindMemoryConflicts(ctx context.Context, workflowID uuid.UUID) ([]MemoryEdge, error)

	// Advanced operations
	AnalyzeMemoryNetworkHealth(ctx context.Context, workflowID uuid.UUID) (*NetworkHealthMetrics, error)
	SuggestMemoryConnections(ctx context.Context, memoryID int64, limit int) ([]MemoryEdge, error)
	BuildKnowledgeMap(ctx context.Context, workflowID uuid.UUID, depth int) (*KnowledgeMap, error)
}

type NetworkHealthMetrics struct {
	TotalNodes            int     `json:"total_nodes"`
	TotalEdges            int     `json:"total_edges"`
	Density               float64 `json:"density"`
	AveragePathLength     float64 `json:"average_path_length"`
	ClusteringCoefficient float64 `json:"clustering_coefficient"`
	IsolatedNodes         int     `json:"isolated_nodes"`
}

type KnowledgeMap struct {
	CentralNodes []MemoryNode   `json:"central_nodes"`
	Communities  [][]MemoryNode `json:"communities"`
	Bridges      []MemoryEdge   `json:"bridges"`
}

// Enhanced agentic engine
type EnhancedAgenticEngine struct {
	DB *pgx.Conn
}

func NewEnhancedAgenticEngine(db *pgx.Conn) *EnhancedAgenticEngine {
	return &EnhancedAgenticEngine{DB: db}
}

// Enhanced table setup with pgRouting support
func (eae *EnhancedAgenticEngine) EnsureEnhancedMemoryTables(ctx context.Context, embeddingDim int) error {
	// Enable pgRouting extension
	_, err := eae.DB.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS pgrouting;`)
	if err != nil {
		return fmt.Errorf("failed to enable pgrouting: %w", err)
	}

	// Create enhanced memory nodes table
	_, err = eae.DB.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS memory_nodes (
			id BIGSERIAL PRIMARY KEY,
			node_type VARCHAR(50) NOT NULL DEFAULT 'memory',
			workflow_id UUID,
			content TEXT NOT NULL,
			note_context TEXT,
			keywords TEXT[],
			tags TEXT[],
			timestamp TIMESTAMP DEFAULT NOW(),
			embedding VECTOR(%d) NOT NULL,
			metadata JSONB DEFAULT '{}',
			x DOUBLE PRECISION DEFAULT RANDOM() * 100, -- Spatial coordinates for routing
			y DOUBLE PRECISION DEFAULT RANDOM() * 100,
			
			-- Indexes for performance
			CONSTRAINT memory_nodes_node_type_check CHECK (node_type IN ('memory', 'concept', 'document', 'workflow', 'agent'))
		);`, embeddingDim))
	if err != nil {
		return fmt.Errorf("failed to create memory_nodes table: %w", err)
	}

	// Create memory edges table for relationships
	_, err = eae.DB.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS memory_edges (
			id BIGSERIAL PRIMARY KEY,
			source BIGINT NOT NULL REFERENCES memory_nodes(id) ON DELETE CASCADE,
			target BIGINT NOT NULL REFERENCES memory_nodes(id) ON DELETE CASCADE,
			relationship_type VARCHAR(50) NOT NULL,
			cost DOUBLE PRECISION DEFAULT 1.0, -- Lower cost = stronger relationship
			reverse_cost DOUBLE PRECISION DEFAULT 1.0,
			weight DOUBLE PRECISION DEFAULT 1.0,
			confidence DOUBLE PRECISION DEFAULT 0.5,
			created_at TIMESTAMP DEFAULT NOW(),
			evidence TEXT,
			x1 DOUBLE PRECISION,
			y1 DOUBLE PRECISION,
			x2 DOUBLE PRECISION,
			y2 DOUBLE PRECISION,
			
			-- Prevent duplicate edges
			UNIQUE(source, target, relationship_type)
		);`)
	if err != nil {
		return fmt.Errorf("failed to create memory_edges table: %w", err)
	}

	// Create indexes for performance
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_memory_nodes_workflow ON memory_nodes(workflow_id, timestamp DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_memory_nodes_embedding ON memory_nodes USING ivfflat (embedding vector_cosine_ops);`,
		`CREATE INDEX IF NOT EXISTS idx_memory_nodes_type ON memory_nodes(node_type);`,
		`CREATE INDEX IF NOT EXISTS idx_memory_edges_source ON memory_edges(source);`,
		`CREATE INDEX IF NOT EXISTS idx_memory_edges_target ON memory_edges(target);`,
		`CREATE INDEX IF NOT EXISTS idx_memory_edges_type ON memory_edges(relationship_type);`,
		`CREATE INDEX IF NOT EXISTS idx_memory_edges_cost ON memory_edges(cost);`,
	}

	for _, indexSQL := range indexes {
		_, err = eae.DB.Exec(ctx, indexSQL)
		if err != nil {
			log.Printf("Warning: failed to create index: %v", err)
		}
	}

	return nil
}

// Enhanced memory ingestion with intelligent graph building
func (eae *EnhancedAgenticEngine) IngestEnhancedMemory(
	ctx context.Context,
	config *configpkg.Config,
	content string,
	workflowID uuid.UUID,
	nodeType string,
) (int64, error) {
	log.Printf("Ingesting enhanced memory: type=%s, workflow=%s", nodeType, workflowID.String())

	// Generate summary and keywords
	summaryOutput, err := sefii.SummarizeChunk(ctx, content, config.Completions.DefaultHost, config.Completions.CompletionsModel, config.Completions.APIKey)
	if err != nil {
		return 0, fmt.Errorf("failed to summarize content: %w", err)
	}

	// Skip unreadable content
	if len(summaryOutput.Keywords) == 0 || containsUnreadableKeywords(summaryOutput.Keywords) {
		return 0, fmt.Errorf("content appears to be unreadable or encoded")
	}

	// Generate embedding
	embeddingInput := config.Embeddings.EmbedPrefix + content + " " + summaryOutput.Summary + " " + strings.Join(summaryOutput.Keywords, " ")
	embeds, err := embeddings.GenerateEmbeddings(config.Embeddings.Host, config.Embeddings.APIKey, []string{embeddingInput})
	if err != nil || len(embeds) == 0 {
		return 0, fmt.Errorf("failed to generate embedding: %w", err)
	}
	vec := pgvector.NewVector(embeds[0])

	// Insert new memory node
	var newID int64
	insertQuery := `
		INSERT INTO memory_nodes (node_type, workflow_id, content, note_context, keywords, tags, embedding, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id`

	metadata := map[string]interface{}{
		"ingested_at": time.Now(),
		"source":      "agent",
	}

	err = eae.DB.QueryRow(ctx, insertQuery,
		nodeType, workflowID, content, summaryOutput.Summary,
		summaryOutput.Keywords, summaryOutput.Keywords, vec, metadata).Scan(&newID)
	if err != nil {
		return 0, fmt.Errorf("failed to insert memory node: %w", err)
	}

	// Build intelligent relationships using graph analysis
	err = eae.buildIntelligentRelationships(ctx, config, newID, workflowID)
	if err != nil {
		log.Printf("Warning: failed to build relationships for memory %d: %v", newID, err)
	}

	return newID, nil
}

// Build intelligent relationships using multiple strategies
func (eae *EnhancedAgenticEngine) buildIntelligentRelationships(ctx context.Context, config *configpkg.Config, newNodeID int64, workflowID uuid.UUID) error {
	// Get the new node's data
	var newNode MemoryNode
	err := eae.DB.QueryRow(ctx, `
		SELECT id, content, note_context, keywords, tags, embedding 
		FROM memory_nodes WHERE id = $1`, newNodeID).Scan(
		&newNode.ID, &newNode.Content, &newNode.NoteContext,
		&newNode.Keywords, &newNode.Tags, &newNode.Embedding)
	if err != nil {
		return err
	}

	// Strategy 1: Semantic similarity
	err = eae.createSemanticRelationships(ctx, newNode, workflowID)
	if err != nil {
		log.Printf("Warning: semantic relationship creation failed: %v", err)
	}

	// Strategy 2: Temporal relationships
	err = eae.createTemporalRelationships(ctx, newNode, workflowID)
	if err != nil {
		log.Printf("Warning: temporal relationship creation failed: %v", err)
	}

	// Strategy 3: Keyword-based conceptual links
	err = eae.createConceptualRelationships(ctx, newNode, workflowID)
	if err != nil {
		log.Printf("Warning: conceptual relationship creation failed: %v", err)
	}

	return nil
}

// Create relationships based on semantic similarity
func (eae *EnhancedAgenticEngine) createSemanticRelationships(ctx context.Context, newNode MemoryNode, workflowID uuid.UUID) error {
	// Find semantically similar nodes
	rows, err := eae.DB.Query(ctx, `
		SELECT id, content, embedding <-> $1 as distance
		FROM memory_nodes 
		WHERE id != $2 AND workflow_id = $3
		ORDER BY embedding <-> $1
		LIMIT 5`, newNode.Embedding, newNode.ID, workflowID)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var similarID int64
		var distance float64
		var content string
		if err := rows.Scan(&similarID, &content, &distance); err != nil {
			continue
		}

		// Only create relationships for sufficiently similar content
		if distance < 0.3 {
			confidence := 1.0 - distance
			cost := distance // Lower distance = lower cost = stronger relationship

			_, err = eae.DB.Exec(ctx, `
				INSERT INTO memory_edges (source, target, relationship_type, cost, confidence, evidence)
				VALUES ($1, $2, $3, $4, $5, $6)
				ON CONFLICT (source, target, relationship_type) DO NOTHING`,
				newNode.ID, similarID, RelationshipSimilar, cost, confidence,
				fmt.Sprintf("Semantic similarity: %.3f", confidence))
			if err != nil {
				log.Printf("Warning: failed to create semantic relationship: %v", err)
			}
		}
	}

	return nil
}

// Create temporal relationships with recent memories
func (eae *EnhancedAgenticEngine) createTemporalRelationships(ctx context.Context, newNode MemoryNode, workflowID uuid.UUID) error {
	// Find recent memories in the same workflow
	rows, err := eae.DB.Query(ctx, `
		SELECT id, timestamp
		FROM memory_nodes 
		WHERE id != $1 AND workflow_id = $2 AND timestamp > NOW() - INTERVAL '1 hour'
		ORDER BY timestamp DESC
		LIMIT 3`, newNode.ID, workflowID)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var recentID int64
		var timestamp time.Time
		if err := rows.Scan(&recentID, &timestamp); err != nil {
			continue
		}

		// Create temporal relationship
		timeDiff := time.Since(timestamp).Minutes()
		cost := timeDiff / 60.0          // Cost increases with time difference
		confidence := 1.0 / (1.0 + cost) // Confidence decreases with time

		_, err = eae.DB.Exec(ctx, `
			INSERT INTO memory_edges (source, target, relationship_type, cost, confidence, evidence)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (source, target, relationship_type) DO NOTHING`,
			recentID, newNode.ID, RelationshipTemporal, cost, confidence,
			fmt.Sprintf("Created %.1f minutes apart", timeDiff))
		if err != nil {
			log.Printf("Warning: failed to create temporal relationship: %v", err)
		}
	}

	return nil
}

// Create relationships based on shared concepts/keywords
func (eae *EnhancedAgenticEngine) createConceptualRelationships(ctx context.Context, newNode MemoryNode, workflowID uuid.UUID) error {
	if len(newNode.Keywords) == 0 {
		return nil
	}

	// Find nodes with overlapping keywords
	rows, err := eae.DB.Query(ctx, `
		SELECT id, keywords
		FROM memory_nodes 
		WHERE id != $1 AND workflow_id = $2 AND keywords && $3
		LIMIT 10`, newNode.ID, workflowID, newNode.Keywords)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var conceptualID int64
		var keywords []string
		if err := rows.Scan(&conceptualID, &keywords); err != nil {
			continue
		}

		// Calculate keyword overlap
		overlap := calculateKeywordOverlap(newNode.Keywords, keywords)
		if overlap > 0.2 { // At least 20% overlap
			confidence := overlap
			cost := 1.0 - overlap

			_, err = eae.DB.Exec(ctx, `
				INSERT INTO memory_edges (source, target, relationship_type, cost, confidence, evidence)
				VALUES ($1, $2, $3, $4, $5, $6)
				ON CONFLICT (source, target, relationship_type) DO NOTHING`,
				newNode.ID, conceptualID, RelationshipReferences, cost, confidence,
				fmt.Sprintf("Keyword overlap: %.1f%%", overlap*100))
			if err != nil {
				log.Printf("Warning: failed to create conceptual relationship: %v", err)
			}
		}
	}

	return nil
}

// Find memory path using pgRouting
func (eae *EnhancedAgenticEngine) FindMemoryPath(ctx context.Context, sourceID, targetID int64) (*MemoryPath, error) {
	// Use pgr_dijkstra to find shortest path
	rows, err := eae.DB.Query(ctx, `
		SELECT seq, path_seq, node, edge, cost, agg_cost
		FROM pgr_dijkstra(
			'SELECT id, source, target, cost FROM memory_edges',
			$1, $2, FALSE
		)`, sourceID, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var path MemoryPath
	var nodeIDs []int64
	var totalCost float64

	for rows.Next() {
		var seq, pathSeq, node, edge int64
		var cost, aggCost float64
		if err := rows.Scan(&seq, &pathSeq, &node, &edge, &cost, &aggCost); err != nil {
			continue
		}
		nodeIDs = append(nodeIDs, node)
		totalCost = aggCost
	}

	if len(nodeIDs) == 0 {
		return nil, fmt.Errorf("no path found between nodes %d and %d", sourceID, targetID)
	}

	// Fetch full node and edge data
	path.Nodes, err = eae.getNodesByIDs(ctx, nodeIDs)
	if err != nil {
		return nil, err
	}

	path.Edges, err = eae.getEdgesForPath(ctx, nodeIDs)
	if err != nil {
		return nil, err
	}

	path.Cost = totalCost
	path.Hops = len(nodeIDs) - 1

	return &path, nil
}

// Find related memories within a certain number of hops
func (eae *EnhancedAgenticEngine) FindRelatedMemories(ctx context.Context, memoryID int64, maxHops int, relationshipTypes []string) ([]MemoryNode, error) {
	// Use pgr_drivingDistance for finding nodes within distance
	typeFilter := ""
	if len(relationshipTypes) > 0 {
		typeFilter = fmt.Sprintf("AND relationship_type = ANY(ARRAY['%s'])", strings.Join(relationshipTypes, "','"))
	}

	query := fmt.Sprintf(`
		SELECT DISTINCT id FROM pgr_drivingDistance(
			'SELECT id, source, target, cost FROM memory_edges WHERE cost <= %d %s',
			%d, %d, FALSE
		) dd
		JOIN memory_nodes mn ON dd.node = mn.id
		WHERE dd.node != %d`, maxHops, typeFilter, memoryID, maxHops, memoryID)

	rows, err := eae.DB.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodeIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err == nil {
			nodeIDs = append(nodeIDs, id)
		}
	}

	return eae.getNodesByIDs(ctx, nodeIDs)
}

// Helper functions
func containsUnreadableKeywords(keywords []string) bool {
	unreadableTerms := []string{"encoded data", "encrypted text", "unreadable content"}
	keywordStr := strings.ToLower(strings.Join(keywords, " "))
	for _, term := range unreadableTerms {
		if strings.Contains(keywordStr, term) {
			return true
		}
	}
	return false
}

func calculateKeywordOverlap(keywords1, keywords2 []string) float64 {
	if len(keywords1) == 0 || len(keywords2) == 0 {
		return 0.0
	}

	set1 := make(map[string]bool)
	for _, k := range keywords1 {
		set1[strings.ToLower(k)] = true
	}

	overlap := 0
	for _, k := range keywords2 {
		if set1[strings.ToLower(k)] {
			overlap++
		}
	}

	return float64(overlap) / float64(len(keywords1)+len(keywords2)-overlap)
}

func (eae *EnhancedAgenticEngine) getNodesByIDs(ctx context.Context, nodeIDs []int64) ([]MemoryNode, error) {
	if len(nodeIDs) == 0 {
		return nil, nil
	}

	// Convert slice to PostgreSQL array format
	query := `
		SELECT id, node_type, workflow_id, content, note_context, keywords, tags, timestamp, embedding, metadata
		FROM memory_nodes 
		WHERE id = ANY($1)`

	rows, err := eae.DB.Query(ctx, query, nodeIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []MemoryNode
	for rows.Next() {
		var node MemoryNode
		var keywordStr, tagStr string
		err := rows.Scan(&node.ID, &node.NodeType, &node.WorkflowID, &node.Content,
			&node.NoteContext, &keywordStr, &tagStr, &node.Timestamp,
			&node.Embedding, &node.Metadata)
		if err != nil {
			continue
		}

		node.Keywords = parseTextArray(keywordStr)
		node.Tags = parseTextArray(tagStr)
		nodes = append(nodes, node)
	}

	return nodes, nil
}

func (eae *EnhancedAgenticEngine) getEdgesForPath(ctx context.Context, nodeIDs []int64) ([]MemoryEdge, error) {
	if len(nodeIDs) < 2 {
		return nil, nil
	}

	var edges []MemoryEdge
	for i := 0; i < len(nodeIDs)-1; i++ {
		var edge MemoryEdge
		err := eae.DB.QueryRow(ctx, `
			SELECT id, source, target, relationship_type, weight, confidence, created_at, evidence
			FROM memory_edges 
			WHERE source = $1 AND target = $2
			LIMIT 1`, nodeIDs[i], nodeIDs[i+1]).Scan(
			&edge.ID, &edge.SourceID, &edge.TargetID, &edge.RelationshipType,
			&edge.Weight, &edge.Confidence, &edge.CreatedAt, &edge.Evidence)
		if err == nil {
			edges = append(edges, edge)
		}
	}

	return edges, nil
}

// parseTextArray helper (reused from original code)
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
