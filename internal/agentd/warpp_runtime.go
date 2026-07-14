package agentd

import (
	"context"

	"manifold/internal/durable"
	persist "manifold/internal/persistence"
	"manifold/internal/warpp"
	warppruntime "manifold/internal/warpp/runtime"
	warppservice "manifold/internal/warpp/service"
)

type warppWorkflowSummary = warppruntime.WorkflowSummary

// newWarppService creates the package-owned WARP state service. agentd only
// supplies stores and durable infrastructure at this composition boundary.
func newWarppService(store persist.WarppWorkflowStore, durableClients ...*durable.Client) *warppservice.Service {
	return warppservice.New(warppservice.Deps{State: warppruntime.New(store, durableClients...)})
}

// newWarppRuntime is retained as a test-only construction compatibility shim
// while the state is now owned by warpp/service.
func newWarppRuntime(store persist.WarppWorkflowStore, durableClients ...*durable.Client) *warppservice.Service {
	return newWarppService(store, durableClients...)
}

func warppEventPayload(ev warpp.Event) map[string]any {
	return warppruntime.EventPayload(ev)
}

func (a *app) warppState() *warppservice.Service {
	if a.warpp == nil {
		var store persist.WarppWorkflowStore
		if a.mgr != nil {
			store = a.mgr.Warpp
		}
		a.warpp = newWarppService(store, a.durableClient)
	}
	return a.warpp
}

func (a *app) warppListWorkflowSummaries(ctx context.Context, userID int64) ([]warppWorkflowSummary, error) {
	return a.warppState().ListWorkflowSummaries(ctx, userID)
}
