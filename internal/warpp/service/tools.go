package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"manifold/internal/tools"
	"manifold/internal/warpp"
	"manifold/internal/warpp/toolnode"
)

// RegisterAuthoringTools installs the workflow authoring tools into a
// registry. The tools call back into this service rather than retaining an
// application pointer.
func (s *Service) RegisterAuthoringTools(reg tools.Registry) {
	if s == nil || reg == nil {
		return
	}
	reg.Register(s.newCatalogTool())
	reg.Register(s.newListTool())
	reg.Register(s.newGetTool())
	reg.Register(s.newSaveTool())
	reg.Register(s.newRunTool())
}

// CatalogTool returns the workflow catalog tool.
func (s *Service) CatalogTool() tools.Tool { return s.newCatalogTool() }

// ListTool returns the workflow list tool.
func (s *Service) ListTool() tools.Tool { return s.newListTool() }

// GetTool returns the workflow retrieval tool.
func (s *Service) GetTool() tools.Tool { return s.newGetTool() }

// SaveTool returns the workflow persistence tool.
func (s *Service) SaveTool() tools.Tool { return s.newSaveTool() }

// RunTool returns the workflow execution tool.
func (s *Service) RunTool() tools.Tool { return s.newRunTool() }

// SyncPublishedWorkflowTools replaces the previously registered published
// workflow tools and returns the new names for the caller to retain.
func (s *Service) SyncPublishedWorkflowTools(ctx context.Context, userID int64, reg tools.Registry, previous []string) []string {
	if s == nil || reg == nil || s.deps.State == nil {
		return nil
	}
	for _, name := range previous {
		reg.Unregister(name)
	}
	summaries, err := s.deps.State.ListWorkflowSummaries(ctx, userID)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		if !summary.PublishTool {
			continue
		}
		name := "warpp_" + SanitizeToolName(summary.ID)
		reg.Register(&publishedWorkflowTool{service: s, workflowID: summary.ID, name: name, description: summary.Description, userID: userID})
		names = append(names, name)
	}
	return names
}

// SanitizeToolName converts a workflow id into a stable tool-name suffix.
func SanitizeToolName(id string) string {
	var builder strings.Builder
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			builder.WriteRune(r)
		default:
			builder.WriteRune('_')
		}
	}
	name := strings.Trim(builder.String(), "_")
	if name == "" {
		return "workflow"
	}
	return name
}

type publishedWorkflowTool struct {
	service     *Service
	workflowID  string
	name        string
	description string
	userID      int64
}

func (t *publishedWorkflowTool) Name() string { return t.name }

func (t *publishedWorkflowTool) JSONSchema() map[string]any {
	doc, _, found, _ := t.service.deps.State.GetWorkflow(context.Background(), t.userID, t.workflowID)
	description := strings.TrimSpace(t.description)
	if description == "" {
		description = fmt.Sprintf("Run the %q workflow.", t.workflowID)
	}
	parameters := map[string]any{"type": "object", "properties": map[string]any{}}
	if found {
		parameters = PortSpecJSONSchema(doc.Inputs)
	}
	return map[string]any{"name": t.name, "description": description, "parameters": parameters}
}

func (t *publishedWorkflowTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var input map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &input); err != nil {
			return map[string]any{"ok": false, "error": fmt.Sprintf("invalid args: %v", err)}, nil
		}
	}
	result, err := t.service.ExecuteSync(ctx, t.userID, t.workflowID, input)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error(), "workflow": t.workflowID}, nil
	}
	return result, nil
}

func (s *Service) systemUserID() int64 {
	if s == nil {
		return 0
	}
	return s.deps.SystemUserID
}

type catalogTool struct{ service *Service }

func (t *catalogTool) Name() string { return "workflow_catalog" }
func (t *catalogTool) JSONSchema() map[string]any {
	return map[string]any{"name": t.Name(), "description": "List available WARPP workflow node types (manifests) and type coercions.", "parameters": map[string]any{"type": "object", "properties": map[string]any{}}}
}
func (t *catalogTool) Call(ctx context.Context, _ json.RawMessage) (any, error) {
	manifests := warpp.BuiltinManifests()
	manifests = append(manifests, toolnode.Manifests(toolnode.Builtin())...)
	var registry tools.Registry
	if t.service.deps.CatalogRegistry != nil {
		registry = t.service.deps.CatalogRegistry()
	}
	manifests = append(manifests, toolnode.DynamicManifests(registry, toolnode.CuratedToolNames())...)
	manifests = append(manifests, t.service.PublishedWorkflowManifests(ctx, t.service.systemUserID())...)
	return map[string]any{"ok": true, "manifests": manifests, "coercions": [][2]string{{"number", "text"}, {"boolean", "text"}}}, nil
}

type listTool struct{ service *Service }

func (t *listTool) Name() string { return "workflow_list" }
func (t *listTool) JSONSchema() map[string]any {
	return map[string]any{"name": t.Name(), "description": "List saved WARPP workflows.", "parameters": map[string]any{"type": "object", "properties": map[string]any{}}}
}
func (t *listTool) Call(ctx context.Context, _ json.RawMessage) (any, error) {
	summaries, err := t.service.deps.State.ListWorkflowSummaries(ctx, t.service.systemUserID())
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	return map[string]any{"ok": true, "workflows": summaries}, nil
}

type getTool struct{ service *Service }

func (t *getTool) Name() string { return "workflow_get" }
func (t *getTool) JSONSchema() map[string]any {
	return map[string]any{"name": t.Name(), "description": "Get a saved WARPP workflow document by id.", "parameters": map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string"}}, "required": []string{"id"}}}
}
func (t *getTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(raw, &args)
	doc, canvas, found, err := t.service.deps.State.GetWorkflow(ctx, t.service.systemUserID(), strings.TrimSpace(args.ID))
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	if !found {
		return map[string]any{"ok": false, "error": "workflow not found"}, nil
	}
	return map[string]any{"ok": true, "document": doc, "canvas": canvas}, nil
}

type saveTool struct{ service *Service }

func (t *saveTool) Name() string { return "workflow_save" }
func (t *saveTool) JSONSchema() map[string]any {
	return map[string]any{"name": t.Name(), "description": "Validate and save a WARPP workflow document. Returns diagnostics on failure.", "parameters": map[string]any{"type": "object", "properties": map[string]any{"document": map[string]any{"type": "object"}, "canvas": map[string]any{"type": "object"}}, "required": []string{"document"}}}
}
func (t *saveTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct {
		Document warpp.Document `json:"document"`
		Canvas   warpp.Canvas   `json:"canvas"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return map[string]any{"ok": false, "error": fmt.Sprintf("invalid args: %v", err)}, nil
	}
	userID := t.service.systemUserID()
	diagnostics := warpp.Validate(args.Document, t.service.Resolver(ctx, userID))
	diagnostics = append(diagnostics, t.service.CheckSubflowCycles(ctx, userID, args.Document)...)
	if warpp.HasErrors(diagnostics) {
		return map[string]any{"ok": false, "diagnostics": diagnostics}, nil
	}
	if t.service.deps.SaveWorkflow == nil {
		return map[string]any{"ok": false, "error": "workflow save unavailable"}, nil
	}
	if _, err := t.service.deps.SaveWorkflow(ctx, userID, args.Document, args.Canvas); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	return map[string]any{"ok": true, "id": args.Document.ID, "diagnostics": diagnostics}, nil
}

type runTool struct{ service *Service }

func (t *runTool) Name() string { return "workflow_run" }
func (t *runTool) JSONSchema() map[string]any {
	return map[string]any{"name": t.Name(), "description": "Run a saved WARPP workflow by id and return its declared outputs.", "parameters": map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string"}, "input": map[string]any{"type": "object"}}, "required": []string{"id"}}}
}
func (t *runTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct {
		ID    string         `json:"id"`
		Input map[string]any `json:"input"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return map[string]any{"ok": false, "error": fmt.Sprintf("invalid args: %v", err)}, nil
	}
	result, err := t.service.ExecuteSync(ctx, t.service.systemUserID(), args.ID, args.Input)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	return result, nil
}

func (s *Service) newCatalogTool() tools.Tool { return &catalogTool{service: s} }
func (s *Service) newListTool() tools.Tool    { return &listTool{service: s} }
func (s *Service) newGetTool() tools.Tool     { return &getTool{service: s} }
func (s *Service) newSaveTool() tools.Tool    { return &saveTool{service: s} }
func (s *Service) newRunTool() tools.Tool     { return &runTool{service: s} }

// PortSpecJSONSchema creates the JSON schema used by published workflow tools.
func PortSpecJSONSchema(inputs []warpp.PortSpec) map[string]any {
	properties := map[string]any{}
	var required []string
	for _, input := range inputs {
		properties[input.Name] = JSONSchemaForType(input.Type, input.Description)
		if input.Required {
			required = append(required, input.Name)
		}
	}
	schema := map[string]any{"type": "object", "properties": properties}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// JSONSchemaForType maps a WARP type into a tool parameter schema.
func JSONSchemaForType(typeName, description string) map[string]any {
	typeSpec, err := warpp.ParseType(typeName)
	if err != nil {
		return map[string]any{"description": description}
	}
	switch typeSpec.Kind {
	case warpp.KindText, warpp.KindFile:
		return map[string]any{"type": "string", "description": description}
	case warpp.KindNumber:
		return map[string]any{"type": "number", "description": description}
	case warpp.KindBoolean:
		return map[string]any{"type": "boolean", "description": description}
	case warpp.KindList:
		return map[string]any{"type": "array", "items": JSONSchemaForType(string(typeSpec.Elem), ""), "description": description}
	default:
		return map[string]any{"type": "object", "description": description}
	}
}
