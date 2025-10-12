package db

import (
    "context"
    "encoding/json"
    "testing"

    "manifold/internal/config"
    "manifold/internal/persistence/databases"
)

func TestIndexDocument_Single(t *testing.T) {
    s := databases.NewMemorySearch()
    v := databases.NewMemoryVector()
    tool := NewIndexDocumentTool(s, v, config.EmbeddingConfig{})
    ctx := context.Background()
    out, _ := tool.Call(ctx, json.RawMessage(`{"id":"doc:1","text":"hello single","metadata":{"k":"v"}}`))
    m := out.(map[string]any)
    if ok := m["ok"].(bool); !ok {
        t.Fatalf("expected ok true, got %#v", m)
    }
}

func TestIndexDocument_BatchTexts(t *testing.T) {
    s := databases.NewMemorySearch()
    v := databases.NewMemoryVector()
    tool := NewIndexDocumentTool(s, v, config.EmbeddingConfig{})
    ctx := context.Background()
    out, _ := tool.Call(ctx, json.RawMessage(`{"id_prefix":"batch","texts":["a","b","c"],"concurrency":2}`))
    m := out.(map[string]any)
    if ok := m["ok"].(bool); !ok {
        t.Fatalf("expected ok true, got %#v", m)
    }
    if cnt := intFrom(m["count"]); cnt != 3 {
        t.Fatalf("expected count 3, got %d", cnt)
    }
}

func TestIndexDocument_FromSplitTextJSON(t *testing.T) {
    s := databases.NewMemorySearch()
    v := databases.NewMemoryVector()
    tool := NewIndexDocumentTool(s, v, config.EmbeddingConfig{})
    ctx := context.Background()
    // Simulate split_text output
    payload := `{"ok":true,"chunks":["c1","c2","c3"],"count":3}`
    args := map[string]any{
        "id_prefix":  "split",
        "texts_json": payload,
        "concurrency": 3,
    }
    raw, _ := json.Marshal(args)
    out, _ := tool.Call(ctx, raw)
    m := out.(map[string]any)
    if ok := m["ok"].(bool); !ok {
        t.Fatalf("expected ok true, got %#v", m)
    }
    if cnt := intFrom(m["count"]); cnt != 3 {
        t.Fatalf("expected count 3, got %d", cnt)
    }
}

func intFrom(v any) int {
    switch x := v.(type) {
    case float64:
        return int(x)
    case int:
        return x
    default:
        return 0
    }
}

