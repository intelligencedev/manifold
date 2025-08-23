package db

import (
	"context"
	"encoding/json"
	"testing"

	"singularityio/internal/config"
	"singularityio/internal/persistence/databases"
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
