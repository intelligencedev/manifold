package agentd

import (
	"context"
	"fmt"
	"strings"

	llmpkg "manifold/internal/llm"
	"manifold/internal/sandbox"
	"manifold/internal/tools"
	"manifold/internal/warpp"
	warppservice "manifold/internal/warpp/service"
)

const (
	warppDurableQueue       = "warpp"
	warppDurableRunTaskName = "warpp.run"
)

func (a *app) warppService() *warppservice.Service {
	return a.warppState().WithDeps(warppservice.Deps{
		CatalogRegistry:   a.warppCatalogRegistry,
		ExecutionRegistry: a.warppExecutionRegistry,
		Chat:              a.warppChat,
		ProjectContext:    a.warppProjectContext,
		SystemUserID:      systemUserID,
		SaveWorkflow: func(ctx context.Context, userID int64, doc warpp.Document, canvas warpp.Canvas) (bool, error) {
			_, created, err := a.warppState().UpsertWorkflow(ctx, userID, doc, canvas)
			return created, err
		},
	})
}

func (a *app) warppExecutionRegistry() tools.Registry {
	if a.toolRegistry != nil {
		return a.toolRegistry
	}
	return a.baseToolRegistry
}

func (a *app) warppResolver(ctx context.Context, userID int64) warpp.Resolver {
	return a.warppService().Resolver(ctx, userID)
}

func (a *app) warppRunners(ctx context.Context, userID int64) map[string]warpp.NodeRunner {
	return a.warppService().Runners(ctx, userID)
}

func (a *app) warppCatalogRegistry() tools.Registry {
	if a.baseToolRegistry != nil {
		return a.baseToolRegistry
	}
	return a.toolRegistry
}

func (a *app) warppChat(ctx context.Context, instruction, input, model string) (string, error) {
	if a.llm == nil {
		return "", fmt.Errorf("no llm provider available")
	}
	var msgs []llmpkg.Message
	if strings.TrimSpace(instruction) != "" {
		msgs = append(msgs, llmpkg.Message{Role: "system", Content: instruction})
	}
	msgs = append(msgs, llmpkg.Message{Role: "user", Content: input})
	reply, err := a.llm.Chat(ctx, msgs, nil, strings.TrimSpace(model))
	if err != nil {
		return "", err
	}
	return reply.Content, nil
}

func (a *app) newWarppEngine(ctx context.Context, userID int64, runID string, doc warpp.Document) *warpp.Engine {
	return a.warppService().NewEngine(ctx, userID, runID, doc)
}

func (a *app) warppProjectContext(ctx context.Context, userID int64, projectID, sessionID string) (context.Context, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return ctx, nil
	}
	if a.workspaceManager != nil {
		ws, err := a.workspaceManager.Checkout(ctx, userID, projectID, sessionID)
		if err != nil {
			return ctx, err
		}
		if ws.BaseDir != "" {
			ctx = sandbox.WithBaseDir(ctx, ws.BaseDir)
			ctx = sandbox.WithProjectID(ctx, projectID)
		}
		return ctx, nil
	}
	return workflowToolContext(ctx, a.cfg, userID, projectID)
}

func (a *app) executeWarppRun(ctx context.Context, userID int64, runID string, doc warpp.Document, input map[string]any) warpp.Result {
	return a.warppService().Execute(ctx, userID, runID, doc, input)
}
