package agents

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"

	embeddings "manifold/internal/llm"
)

// ensureToolMemoryTable creates the tool_memory table if it doesn't exist.
func (ae *AgentEngine) ensureToolMemoryTable(ctx context.Context, dim int) error {
	_, err := ae.DB.Exec(ctx, fmt.Sprintf(`
        CREATE TABLE IF NOT EXISTS tool_memory (
            tool_name TEXT PRIMARY KEY,
            description TEXT,
            embedding vector(%d) NOT NULL
        );`, dim))
	if err != nil {
		return err
	}
	_, _ = ae.DB.Exec(ctx, `CREATE INDEX IF NOT EXISTS tool_memory_embedding_idx ON tool_memory USING ivfflat (embedding vector_cosine_ops);`)
	return nil
}

func (ae *AgentEngine) upsertToolMemory(ctx context.Context, name, desc string, vec pgvector.Vector) error {
	_, err := ae.DB.Exec(ctx, `
        INSERT INTO tool_memory (tool_name, description, embedding)
        VALUES ($1,$2,$3)
        ON CONFLICT (tool_name) DO UPDATE SET description = EXCLUDED.description, embedding = EXCLUDED.embedding`, name, desc, vec)
	return err
}

// getRelevantTools returns the top-k tools most relevant to the objective.
func (ae *AgentEngine) getRelevantTools(ctx context.Context, objective string, k int) ([]ToolInfo, error) {
	if err := ae.ensureToolMemoryTable(ctx, ae.Config.Embeddings.Dimensions); err != nil {
		return nil, err
	}
	searchQuery := fmt.Sprintf("%s%s", ae.Config.Embeddings.SearchPrefix, objective)
	embeds, err := embeddings.GenerateEmbeddings(ae.Config.Embeddings.Host, ae.Config.Embeddings.APIKey, []string{searchQuery})
	if err != nil || len(embeds) == 0 {
		return nil, err
	}
	qvec := pgvector.NewVector(embeds[0])
	rows, err := ae.DB.Query(ctx, `SELECT tool_name FROM tool_memory ORDER BY embedding <-> $1 LIMIT $2`, qvec, k)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tools []ToolInfo
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if t, ok := ae.mcpTools[name]; ok {
			tools = append(tools, t)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tools, nil
}
