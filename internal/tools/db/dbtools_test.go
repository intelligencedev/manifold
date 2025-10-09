package db

import (
	"context"
	"encoding/json"
	"testing"

	"manifold/internal/config"
	"manifold/internal/persistence/databases"
)

func TestSearchTools(t *testing.T) {
	s := databases.NewMemorySearch()
	idx := NewSearchIndexTool(s)
	qry := NewSearchQueryTool(s)
	rem := NewSearchRemoveTool(s)
	ctx := context.Background()

	_, _ = idx.Call(ctx, json.RawMessage(`{"id":"a","text":"hello world"}`))
	out, _ := qry.Call(ctx, json.RawMessage(`{"query":"hello"}`))
	m := out.(map[string]any)
	if ok, _ := m["ok"].(bool); !ok {
		t.Fatalf("expected ok true")
	}
	_, _ = rem.Call(ctx, json.RawMessage(`{"id":"a"}`))
}

func TestVectorTools(t *testing.T) {
	v := databases.NewMemoryVector()
	up := NewVectorUpsertTool(v, config.EmbeddingConfig{})
	q := NewVectorQueryTool(v)
	del := NewVectorDeleteTool(v)
	ctx := context.Background()

	_, _ = up.Call(ctx, json.RawMessage(`{"id":"a","vector":[1,0],"metadata":{"k":"v"}}`))
	out, _ := q.Call(ctx, json.RawMessage(`{"vector":[1,0],"k":1}`))
	m := out.(map[string]any)
	if ok, _ := m["ok"].(bool); !ok {
		t.Fatalf("expected ok true")
	}
	_, _ = del.Call(ctx, json.RawMessage(`{"id":"a"}`))
}

func TestHybridQueryTool(t *testing.T) {
    s := databases.NewMemorySearch()
    v := databases.NewMemoryVector()
    // Seed
    _, _ = NewSearchIndexTool(s).Call(context.Background(), json.RawMessage(`{"id":"doc:1","text":"Acme quarterly revenue Q3","metadata":{"source":"acme"}}`))
    _, _ = NewVectorUpsertTool(v, config.EmbeddingConfig{}).Call(context.Background(), json.RawMessage(`{"id":"doc:1","vector":[0.1,0.2,0.3],"metadata":{"source":"acme"}}`))

    h := NewHybridQueryTool(s, v, config.EmbeddingConfig{})
    out, _ := h.Call(context.Background(), json.RawMessage(`{"query":"Acme revenue","vector":[0.1,0.2,0.3],"k":5}`))
    m := out.(map[string]any)
    if ok, _ := m["ok"].(bool); !ok {
        t.Fatalf("expected ok true")
    }
}

func TestGraphTools(t *testing.T) {
	g := databases.NewMemoryGraph()
	upn := NewGraphUpsertNodeTool(g)
	upe := NewGraphUpsertEdgeTool(g)
	neigh := NewGraphNeighborsTool(g)
	getn := NewGraphGetNodeTool(g)
	ctx := context.Background()

	_, _ = upn.Call(ctx, json.RawMessage(`{"id":"n1","labels":["User"],"props":{"name":"Alice"}}`))
	_, _ = upn.Call(ctx, json.RawMessage(`{"id":"n2"}`))
	_, _ = upe.Call(ctx, json.RawMessage(`{"src":"n1","rel":"KNOWS","dst":"n2"}`))
	out, _ := neigh.Call(ctx, json.RawMessage(`{"id":"n1","rel":"KNOWS"}`))
	m := out.(map[string]any)
	if ok, _ := m["ok"].(bool); !ok {
		t.Fatalf("expected ok true")
	}
	out2, _ := getn.Call(ctx, json.RawMessage(`{"id":"n1"}`))
	m2 := out2.(map[string]any)
	if ok, _ := m2["ok"].(bool); !ok {
		t.Fatalf("expected ok true")
	}
}
