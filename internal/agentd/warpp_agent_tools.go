package agentd

import (
	"context"
	"encoding/json"

	"manifold/internal/tools"
	"manifold/internal/warpp"
	warppservice "manifold/internal/warpp/service"
)

func baseWarppResolver() warpp.Resolver {
	return warppservice.BaseResolver()
}

func (a *app) warppToolRegistry() tools.Registry {
	if a.baseToolRegistry != nil {
		return a.baseToolRegistry
	}
	return a.toolRegistry
}

func (a *app) runWarppSync(ctx context.Context, userID int64, doc warpp.Document, input map[string]any) (warpp.Result, string, error) {
	return a.warppService().RunSync(ctx, userID, doc, input)
}

func (a *app) ExecuteWarppSync(ctx context.Context, userID int64, workflowID string, input map[string]any) (map[string]any, error) {
	return a.warppService().ExecuteSync(ctx, userID, workflowID, input)
}

func (a *app) subflowRunner(userID int64, workflowID string) warpp.NodeRunner {
	return a.warppService().SubflowRunner(userID, workflowID)
}

func (a *app) registerSubflowRunners(ctx context.Context, userID int64, runners map[string]warpp.NodeRunner) {
	summaries, err := a.warppState().ListWorkflowSummaries(ctx, userID)
	if err != nil {
		return
	}
	for _, summary := range summaries {
		runners["flow."+summary.ID] = a.subflowRunner(userID, summary.ID)
	}
}

func (a *app) warppSubflowResolver(ctx context.Context, userID int64) warpp.Resolver {
	return a.warppService().SubflowResolver(ctx, userID)
}

func (a *app) publishedWorkflowManifests(ctx context.Context, userID int64) []warpp.Manifest {
	return a.warppService().PublishedWorkflowManifests(ctx, userID)
}

func warppSubflowRefs(doc warpp.Document) []string {
	return warppservice.SubflowRefs(doc)
}

func (a *app) checkSubflowCycles(ctx context.Context, userID int64, doc warpp.Document) []warpp.Diagnostic {
	return a.warppService().CheckSubflowCycles(ctx, userID, doc)
}

func (a *app) syncPublishedWorkflowTools(ctx context.Context) {
	reg := a.warppToolRegistry()
	if reg == nil {
		return
	}
	a.warppToolMu.Lock()
	defer a.warppToolMu.Unlock()
	a.warppPublishedToolNames = a.warppService().SyncPublishedWorkflowTools(ctx, systemUserID, reg, a.warppPublishedToolNames)
}

func (a *app) registerWarppAgentTools() {
	reg := a.warppToolRegistry()
	if reg == nil {
		return
	}
	a.warppService().RegisterAuthoringTools(reg)
}

func warppDiagSummary(diags []warpp.Diagnostic) string {
	return warppservice.DiagnosticSummary(diags)
}

func sanitizeWarppToolName(id string) string {
	return warppservice.SanitizeToolName(id)
}

func portSpecJSONSchema(inputs []warpp.PortSpec) map[string]any {
	return warppservice.PortSpecJSONSchema(inputs)
}

func jsonSchemaForType(typeName, description string) map[string]any {
	return warppservice.JSONSchemaForType(typeName, description)
}

// These adapters retain the package-private names used by older in-package
// tests while the actual implementations live in warpp/service.
type warppCatalogTool struct{ app *app }

func (t *warppCatalogTool) inner() tools.Tool { return t.app.warppService().CatalogTool() }
func (t *warppCatalogTool) Name() string      { return t.inner().Name() }
func (t *warppCatalogTool) JSONSchema() map[string]any {
	return t.inner().JSONSchema()
}
func (t *warppCatalogTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	return t.inner().Call(ctx, raw)
}

type warppListTool struct{ app *app }

func (t *warppListTool) inner() tools.Tool { return t.app.warppService().ListTool() }
func (t *warppListTool) Name() string      { return t.inner().Name() }
func (t *warppListTool) JSONSchema() map[string]any {
	return t.inner().JSONSchema()
}
func (t *warppListTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	return t.inner().Call(ctx, raw)
}

type warppGetTool struct{ app *app }

func (t *warppGetTool) inner() tools.Tool { return t.app.warppService().GetTool() }
func (t *warppGetTool) Name() string      { return t.inner().Name() }
func (t *warppGetTool) JSONSchema() map[string]any {
	return t.inner().JSONSchema()
}
func (t *warppGetTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	return t.inner().Call(ctx, raw)
}

type warppSaveTool struct{ app *app }

func (t *warppSaveTool) inner() tools.Tool { return t.app.warppService().SaveTool() }
func (t *warppSaveTool) Name() string      { return t.inner().Name() }
func (t *warppSaveTool) JSONSchema() map[string]any {
	return t.inner().JSONSchema()
}
func (t *warppSaveTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	return t.inner().Call(ctx, raw)
}

type warppRunTool struct{ app *app }

func (t *warppRunTool) inner() tools.Tool { return t.app.warppService().RunTool() }
func (t *warppRunTool) Name() string      { return t.inner().Name() }
func (t *warppRunTool) JSONSchema() map[string]any {
	return t.inner().JSONSchema()
}
func (t *warppRunTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	return t.inner().Call(ctx, raw)
}
