package transit

import (
	"context"
	"encoding/json"

	"manifold/internal/auth"
	transitdomain "manifold/internal/transit"
)

const systemUserID int64 = 0

type createTool struct{ service *transitdomain.Service }
type getTool struct{ service *transitdomain.Service }
type updateTool struct{ service *transitdomain.Service }
type deleteTool struct{ service *transitdomain.Service }
type searchTool struct{ service *transitdomain.Service }
type discoverTool struct{ service *transitdomain.Service }
type listKeysTool struct{ service *transitdomain.Service }
type listRecentTool struct{ service *transitdomain.Service }

func NewCreateTool(service *transitdomain.Service) *createTool { return &createTool{service: service} }
func NewGetTool(service *transitdomain.Service) *getTool       { return &getTool{service: service} }
func NewUpdateTool(service *transitdomain.Service) *updateTool { return &updateTool{service: service} }
func NewDeleteTool(service *transitdomain.Service) *deleteTool { return &deleteTool{service: service} }
func NewSearchTool(service *transitdomain.Service) *searchTool { return &searchTool{service: service} }
func NewDiscoverTool(service *transitdomain.Service) *discoverTool {
	return &discoverTool{service: service}
}
func NewListKeysTool(service *transitdomain.Service) *listKeysTool {
	return &listKeysTool{service: service}
}
func NewListRecentTool(service *transitdomain.Service) *listRecentTool {
	return &listRecentTool{service: service}
}

func (t *createTool) Name() string     { return "transit_create" }
func (t *getTool) Name() string        { return "transit_get" }
func (t *updateTool) Name() string     { return "transit_update" }
func (t *deleteTool) Name() string     { return "transit_delete" }
func (t *searchTool) Name() string     { return "transit_search" }
func (t *discoverTool) Name() string   { return "transit_discover" }
func (t *listKeysTool) Name() string   { return "transit_list_keys" }
func (t *listRecentTool) Name() string { return "transit_list_recent" }

func (t *createTool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": "Create one or more Transit shared-memory records.",
		"parameters": map[string]any{
			"type":     "object",
			"required": []string{"items"},
			"properties": map[string]any{
				"items": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":     "object",
						"required": []string{"keyName", "description", "value"},
						"properties": map[string]any{
							"keyName":     map[string]any{"type": "string"},
							"description": map[string]any{"type": "string"},
							"value":       map[string]any{"type": "string"},
							"base64":      map[string]any{"type": "boolean"},
							"embed":       map[string]any{"type": "boolean"},
							"embedSource": map[string]any{"type": "string", "enum": []any{"value", "description"}},
						},
					},
				},
			},
		},
	}
}

func (t *getTool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": "Get Transit shared-memory records by key.",
		"parameters": map[string]any{
			"type":     "object",
			"required": []string{"keys"},
			"properties": map[string]any{
				"keys": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
		},
	}
}

func (t *updateTool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": "Update the value for a Transit shared-memory record.",
		"parameters": map[string]any{
			"type":     "object",
			"required": []string{"keyName", "value"},
			"properties": map[string]any{
				"keyName":     map[string]any{"type": "string"},
				"value":       map[string]any{"type": "string"},
				"base64":      map[string]any{"type": "boolean"},
				"embed":       map[string]any{"type": "boolean"},
				"embedSource": map[string]any{"type": "string", "enum": []any{"value", "description"}},
				"ifVersion":   map[string]any{"type": "integer"},
			},
		},
	}
}

func (t *deleteTool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": "Delete Transit shared-memory records by key.",
		"parameters": map[string]any{
			"type":     "object",
			"required": []string{"keys"},
			"properties": map[string]any{
				"keys": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
		},
	}
}

func (t *searchTool) JSONSchema() map[string]any {
	return searchSchema(t.Name(), "Search Transit shared memory and return full records.")
}

func (t *discoverTool) JSONSchema() map[string]any {
	return searchSchema(t.Name(), "Search Transit shared memory and return metadata-only results.")
}

func (t *listKeysTool) JSONSchema() map[string]any {
	return listSchema(t.Name(), "List Transit keys, optionally filtered by prefix.")
}

func (t *listRecentTool) JSONSchema() map[string]any {
	return listSchema(t.Name(), "List recent Transit keys ordered by update time.")
}

func searchSchema(name, description string) map[string]any {
	return map[string]any{
		"name":        name,
		"description": description,
		"parameters": map[string]any{
			"type":     "object",
			"required": []string{"query"},
			"properties": map[string]any{
				"query":      map[string]any{"type": "string"},
				"prefix":     map[string]any{"type": "string"},
				"limit":      map[string]any{"type": "integer"},
				"withinDays": map[string]any{"type": "integer"},
			},
		},
	}
}

func listSchema(name, description string) map[string]any {
	return map[string]any{
		"name":        name,
		"description": description,
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prefix": map[string]any{"type": "string"},
				"limit":  map[string]any{"type": "integer"},
			},
		},
	}
}

func (t *createTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct {
		Items []transitdomain.CreateMemoryItem `json:"items"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}
	tenantID := currentTenantID(ctx)
	return t.service.CreateMemory(ctx, tenantID, tenantID, args.Items)
}

func (t *getTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct {
		Keys []string `json:"keys"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}
	return t.service.GetMemory(ctx, currentTenantID(ctx), args.Keys)
}

func (t *updateTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args transitdomain.UpdateMemoryRequest
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}
	tenantID := currentTenantID(ctx)
	return t.service.UpdateMemory(ctx, tenantID, tenantID, args)
}

func (t *deleteTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct {
		Keys []string `json:"keys"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}
	if err := t.service.DeleteMemory(ctx, currentTenantID(ctx), args.Keys); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}

func (t *searchTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args transitdomain.SearchRequest
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}
	return t.service.SearchMemories(ctx, currentTenantID(ctx), args)
}

func (t *discoverTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args transitdomain.SearchRequest
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}
	return t.service.DiscoverMemories(ctx, currentTenantID(ctx), args)
}

func (t *listKeysTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args transitdomain.ListRequest
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}
	return t.service.ListKeys(ctx, currentTenantID(ctx), args)
}

func (t *listRecentTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args transitdomain.ListRequest
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}
	return t.service.ListRecent(ctx, currentTenantID(ctx), args)
}

func currentTenantID(ctx context.Context) int64 {
	if user, ok := auth.CurrentUser(ctx); ok && user != nil {
		return user.ID
	}
	return systemUserID
}
