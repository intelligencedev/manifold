package transit

import "testing"

func TestTransitToolSchemasDescribeKeyNameConstraints(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		schema map[string]any
		path   []string
	}{
		{name: "create", schema: (&createTool{}).JSONSchema(), path: []string{"parameters", "properties", "items", "items", "properties", "keyName"}},
		{name: "get", schema: (&getTool{}).JSONSchema(), path: []string{"parameters", "properties", "keys", "items"}},
		{name: "update", schema: (&updateTool{}).JSONSchema(), path: []string{"parameters", "properties", "keyName"}},
		{name: "delete", schema: (&deleteTool{}).JSONSchema(), path: []string{"parameters", "properties", "keys", "items"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			keySchema := schemaAt(t, tt.schema, tt.path...)
			if keySchema["pattern"] != keyNamePattern {
				t.Fatalf("keyName pattern = %v, want %s", keySchema["pattern"], keyNamePattern)
			}
			if keySchema["description"] == "" {
				t.Fatalf("keyName description is empty")
			}
			if keySchema["examples"] == nil {
				t.Fatalf("keyName examples are missing")
			}
		})
	}
}

func schemaAt(t *testing.T, schema map[string]any, path ...string) map[string]any {
	t.Helper()
	current := schema
	for _, part := range path {
		next, ok := current[part].(map[string]any)
		if !ok {
			t.Fatalf("schema path %v missing object at %q", path, part)
		}
		current = next
	}
	return current
}
