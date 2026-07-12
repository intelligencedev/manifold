package agentd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"manifold/internal/tools"
	"manifold/internal/warpp"
	"manifold/internal/warpp/toolnode"
)

// baseWarppResolver types workflows without recursing into other subflows; it
// is used for manifest derivation to keep subflow typing bounded.
func baseWarppResolver() warpp.Resolver {
	return warpp.ChainResolvers(warpp.BuiltinResolver(), toolnode.Resolver(toolnode.Builtin()))
}

func (a *app) warppToolRegistry() tools.Registry {
	if a.baseToolRegistry != nil {
		return a.baseToolRegistry
	}
	return a.toolRegistry
}

// runWarppSync validates and runs a document synchronously, returning the
// result and its run id.
func (a *app) runWarppSync(ctx context.Context, userID int64, doc warpp.Document, input map[string]any) (warpp.Result, string, error) {
	diags := warpp.Validate(doc, a.warppResolver(ctx, userID))
	if warpp.HasErrors(diags) {
		return warpp.Result{Status: warpp.StatusFailed}, "", fmt.Errorf("workflow validation failed: %s", warppDiagSummary(diags))
	}
	runID := a.warppState().createRun(userID, doc.ID, input)
	res := a.executeWarppRun(ctx, userID, runID, doc, input)
	return res, runID, res.Err
}

// ExecuteWarppSync runs a saved workflow by id and returns its declared outputs.
func (a *app) ExecuteWarppSync(ctx context.Context, userID int64, workflowID string, input map[string]any) (map[string]any, error) {
	doc, _, found, err := a.warppState().getWorkflow(ctx, userID, strings.TrimSpace(workflowID))
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("workflow not found")
	}
	res, runID, err := a.runWarppSync(ctx, userID, doc, input)
	if err != nil {
		return nil, err
	}
	if res.Status == warpp.StatusFailed || res.Status == warpp.StatusCancelled {
		return nil, fmt.Errorf("workflow finished with status %s", res.Status)
	}
	return map[string]any{
		"ok":       true,
		"run_id":   runID,
		"status":   res.Status,
		"outputs":  res.Outputs,
		"workflow": doc.ID,
	}, nil
}

// subflowRunner returns a runner that executes a saved workflow as a node.
func (a *app) subflowRunner(userID int64, workflowID string) warpp.NodeRunner {
	return func(ctx context.Context, _ warpp.RunnerCtx, in warpp.NodeInputs) (map[string]warpp.Value, error) {
		doc, _, found, err := a.warppState().getWorkflow(ctx, userID, workflowID)
		if err != nil || !found {
			return nil, fmt.Errorf("subflow %s unavailable", workflowID)
		}
		input := map[string]any{}
		for _, p := range doc.Inputs {
			if v, ok := in.Values[p.Name]; ok {
				input[p.Name] = v.Data
			}
		}
		res, _, err := a.runWarppSync(ctx, userID, doc, input)
		if err != nil {
			return nil, err
		}
		if res.Status == warpp.StatusFailed || res.Status == warpp.StatusCancelled {
			return nil, fmt.Errorf("subflow %s finished with status %s", workflowID, res.Status)
		}
		m, _ := warpp.WorkflowManifest(doc, baseWarppResolver())
		out := map[string]warpp.Value{}
		for _, p := range m.Outputs {
			raw, ok := res.Outputs[p.Name]
			if !ok {
				continue
			}
			pt, perr := warpp.ParseType(p.Type)
			if perr == nil {
				if v, cerr := warpp.CoerceRaw(raw, pt); cerr == nil {
					out[p.Name] = v
					continue
				}
			}
			out[p.Name] = warpp.Value{Type: warpp.InferLiteral(raw), Data: raw}
		}
		return out, nil
	}
}

func (a *app) registerSubflowRunners(ctx context.Context, userID int64, runners map[string]warpp.NodeRunner) {
	sums, err := a.warppState().listWorkflowSummaries(ctx, userID)
	if err != nil {
		return
	}
	for _, s := range sums {
		runners["flow."+s.ID] = a.subflowRunner(userID, s.ID)
	}
}

func (a *app) warppSubflowResolver(ctx context.Context, userID int64) warpp.Resolver {
	base := baseWarppResolver()
	return func(nodeType string) (warpp.Manifest, bool) {
		if !strings.HasPrefix(nodeType, "flow.") {
			return warpp.Manifest{}, false
		}
		id := strings.TrimPrefix(nodeType, "flow.")
		doc, _, found, err := a.warppState().getWorkflow(ctx, userID, id)
		if err != nil || !found {
			return warpp.Manifest{}, false
		}
		m, _ := warpp.WorkflowManifest(doc, base)
		return m, true
	}
}

func (a *app) publishedWorkflowManifests(ctx context.Context, userID int64) []warpp.Manifest {
	sums, err := a.warppState().listWorkflowSummaries(ctx, userID)
	if err != nil {
		return nil
	}
	base := baseWarppResolver()
	var out []warpp.Manifest
	for _, s := range sums {
		doc, _, found, err := a.warppState().getWorkflow(ctx, userID, s.ID)
		if err != nil || !found {
			continue
		}
		m, _ := warpp.WorkflowManifest(doc, base)
		out = append(out, m)
	}
	return out
}

func warppSubflowRefs(doc warpp.Document) []string {
	var ids []string
	var scan func(nodes []warpp.Node)
	scan = func(nodes []warpp.Node) {
		for _, n := range nodes {
			if strings.HasPrefix(n.Type, "flow.") {
				ids = append(ids, strings.TrimPrefix(n.Type, "flow."))
			}
			if n.Body != nil {
				scan(n.Body.Nodes)
			}
		}
	}
	scan(doc.Nodes)
	return ids
}

func (a *app) checkSubflowCycles(ctx context.Context, userID int64, doc warpp.Document) []warpp.Diagnostic {
	seen := map[string]bool{}
	var reaches func(id string) bool
	reaches = func(id string) bool {
		if id == doc.ID {
			return true
		}
		if seen[id] {
			return false
		}
		seen[id] = true
		child, _, found, err := a.warppState().getWorkflow(ctx, userID, id)
		if err != nil || !found {
			return false
		}
		for _, ref := range warppSubflowRefs(child) {
			if reaches(ref) {
				return true
			}
		}
		return false
	}
	for _, ref := range warppSubflowRefs(doc) {
		if reaches(ref) {
			return []warpp.Diagnostic{{
				Severity: warpp.SeverityError,
				Code:     "workflow.subflow.cycle",
				Message:  "subflow inclusion forms a cycle",
			}}
		}
	}
	return nil
}

// syncPublishedWorkflowTools registers one agent tool per system workflow that
// opts into tool publishing.
func (a *app) syncPublishedWorkflowTools(ctx context.Context) {
	reg := a.warppToolRegistry()
	if reg == nil {
		return
	}
	sums, err := a.warppState().listWorkflowSummaries(ctx, systemUserID)
	if err != nil {
		return
	}
	a.warppToolMu.Lock()
	defer a.warppToolMu.Unlock()
	for _, name := range a.warppPublishedToolNames {
		reg.Unregister(name)
	}
	var names []string
	for _, s := range sums {
		if !s.PublishTool {
			continue
		}
		name := "warpp_" + sanitizeWarppToolName(s.ID)
		reg.Register(&publishedWorkflowTool{app: a, workflowID: s.ID, name: name, description: s.Description})
		names = append(names, name)
	}
	a.warppPublishedToolNames = names
}

// registerWarppAgentTools registers the workflow authoring tools once.
func (a *app) registerWarppAgentTools() {
	reg := a.warppToolRegistry()
	if reg == nil {
		return
	}
	reg.Register(&warppCatalogTool{app: a})
	reg.Register(&warppListTool{app: a})
	reg.Register(&warppGetTool{app: a})
	reg.Register(&warppSaveTool{app: a})
	reg.Register(&warppRunTool{app: a})
}

func warppDiagSummary(diags []warpp.Diagnostic) string {
	parts := make([]string, 0, len(diags))
	for _, d := range diags {
		if d.Severity == warpp.SeverityError {
			parts = append(parts, d.Message)
		}
	}
	if len(parts) == 0 {
		return "invalid workflow"
	}
	return strings.Join(parts, "; ")
}

func sanitizeWarppToolName(id string) string {
	var b strings.Builder
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "workflow"
	}
	return out
}

func portSpecJSONSchema(inputs []warpp.PortSpec) map[string]any {
	props := map[string]any{}
	var required []string
	for _, p := range inputs {
		props[p.Name] = jsonSchemaForType(p.Type, p.Description)
		if p.Required {
			required = append(required, p.Name)
		}
	}
	schema := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func jsonSchemaForType(typeStr, desc string) map[string]any {
	t, err := warpp.ParseType(typeStr)
	if err != nil {
		return map[string]any{"description": desc}
	}
	switch t.Kind {
	case warpp.KindText, warpp.KindFile:
		return map[string]any{"type": "string", "description": desc}
	case warpp.KindNumber:
		return map[string]any{"type": "number", "description": desc}
	case warpp.KindBoolean:
		return map[string]any{"type": "boolean", "description": desc}
	case warpp.KindList:
		return map[string]any{"type": "array", "items": jsonSchemaForType(string(t.Elem), ""), "description": desc}
	default:
		return map[string]any{"type": "object", "description": desc}
	}
}

// --- published workflow tool ---

type publishedWorkflowTool struct {
	app         *app
	workflowID  string
	name        string
	description string
}

func (t *publishedWorkflowTool) Name() string { return t.name }

func (t *publishedWorkflowTool) JSONSchema() map[string]any {
	doc, _, found, _ := t.app.warppState().getWorkflow(context.Background(), systemUserID, t.workflowID)
	desc := strings.TrimSpace(t.description)
	if desc == "" {
		desc = fmt.Sprintf("Run the %q workflow.", t.workflowID)
	}
	params := map[string]any{"type": "object", "properties": map[string]any{}}
	if found {
		params = portSpecJSONSchema(doc.Inputs)
	}
	return map[string]any{"name": t.name, "description": desc, "parameters": params}
}

func (t *publishedWorkflowTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var input map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &input); err != nil {
			return map[string]any{"ok": false, "error": fmt.Sprintf("invalid args: %v", err)}, nil
		}
	}
	result, err := t.app.ExecuteWarppSync(ctx, systemUserID, t.workflowID, input)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error(), "workflow": t.workflowID}, nil
	}
	return result, nil
}

// --- authoring tools ---

type warppCatalogTool struct{ app *app }

func (t *warppCatalogTool) Name() string { return "workflow_catalog" }
func (t *warppCatalogTool) JSONSchema() map[string]any {
	return map[string]any{"name": t.Name(),
		"description": "List available WARPP workflow node types (manifests) and type coercions.",
		"parameters":  map[string]any{"type": "object", "properties": map[string]any{}}}
}
func (t *warppCatalogTool) Call(ctx context.Context, _ json.RawMessage) (any, error) {
	manifests := warpp.BuiltinManifests()
	manifests = append(manifests, toolnode.Manifests(toolnode.Builtin())...)
	manifests = append(manifests, toolnode.DynamicManifests(t.app.warppCatalogRegistry(), toolnode.CuratedToolNames())...)
	manifests = append(manifests, t.app.publishedWorkflowManifests(ctx, systemUserID)...)
	return map[string]any{"ok": true, "manifests": manifests,
		"coercions": [][2]string{{"number", "text"}, {"boolean", "text"}}}, nil
}

type warppListTool struct{ app *app }

func (t *warppListTool) Name() string { return "workflow_list" }
func (t *warppListTool) JSONSchema() map[string]any {
	return map[string]any{"name": t.Name(),
		"description": "List saved WARPP workflows.",
		"parameters":  map[string]any{"type": "object", "properties": map[string]any{}}}
}
func (t *warppListTool) Call(ctx context.Context, _ json.RawMessage) (any, error) {
	sums, err := t.app.warppState().listWorkflowSummaries(ctx, systemUserID)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	return map[string]any{"ok": true, "workflows": sums}, nil
}

type warppGetTool struct{ app *app }

func (t *warppGetTool) Name() string { return "workflow_get" }
func (t *warppGetTool) JSONSchema() map[string]any {
	return map[string]any{"name": t.Name(),
		"description": "Get a saved WARPP workflow document by id.",
		"parameters": map[string]any{"type": "object",
			"properties": map[string]any{"id": map[string]any{"type": "string"}},
			"required":   []string{"id"}}}
}
func (t *warppGetTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(raw, &args)
	doc, canvas, found, err := t.app.warppState().getWorkflow(ctx, systemUserID, strings.TrimSpace(args.ID))
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	if !found {
		return map[string]any{"ok": false, "error": "workflow not found"}, nil
	}
	return map[string]any{"ok": true, "document": doc, "canvas": canvas}, nil
}

type warppSaveTool struct{ app *app }

func (t *warppSaveTool) Name() string { return "workflow_save" }
func (t *warppSaveTool) JSONSchema() map[string]any {
	return map[string]any{"name": t.Name(),
		"description": "Validate and save a WARPP workflow document. Returns diagnostics on failure.",
		"parameters": map[string]any{"type": "object",
			"properties": map[string]any{
				"document": map[string]any{"type": "object"},
				"canvas":   map[string]any{"type": "object"},
			},
			"required": []string{"document"}}}
}
func (t *warppSaveTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct {
		Document warpp.Document `json:"document"`
		Canvas   warpp.Canvas   `json:"canvas"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return map[string]any{"ok": false, "error": fmt.Sprintf("invalid args: %v", err)}, nil
	}
	diags := warpp.Validate(args.Document, t.app.warppResolver(ctx, systemUserID))
	diags = append(diags, t.app.checkSubflowCycles(ctx, systemUserID, args.Document)...)
	if warpp.HasErrors(diags) {
		return map[string]any{"ok": false, "diagnostics": diags}, nil
	}
	if _, _, err := t.app.warppState().upsertWorkflow(ctx, systemUserID, args.Document, args.Canvas); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	t.app.syncPublishedWorkflowTools(ctx)
	return map[string]any{"ok": true, "id": args.Document.ID, "diagnostics": diags}, nil
}

type warppRunTool struct{ app *app }

func (t *warppRunTool) Name() string { return "workflow_run" }
func (t *warppRunTool) JSONSchema() map[string]any {
	return map[string]any{"name": t.Name(),
		"description": "Run a saved WARPP workflow by id and return its declared outputs.",
		"parameters": map[string]any{"type": "object",
			"properties": map[string]any{
				"id":    map[string]any{"type": "string"},
				"input": map[string]any{"type": "object"},
			},
			"required": []string{"id"}}}
}
func (t *warppRunTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct {
		ID    string         `json:"id"`
		Input map[string]any `json:"input"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return map[string]any{"ok": false, "error": fmt.Sprintf("invalid args: %v", err)}, nil
	}
	result, err := t.app.ExecuteWarppSync(ctx, systemUserID, args.ID, args.Input)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	return result, nil
}
