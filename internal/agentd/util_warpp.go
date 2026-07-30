package agentd

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"manifold/internal/config"
	"manifold/internal/sandbox"
)

// cloneMap deep-copies a map[string]any via JSON round-trip.
func cloneMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	var out map[string]any
	b, _ := json.Marshal(m)
	_ = json.Unmarshal(b, &out)
	if out == nil {
		out = map[string]any{}
	}
	return out
}

// workflowToolContext scopes filesystem-aware tools to a project sandbox for a
// workflow run.
func workflowToolContext(ctx context.Context, cfg *config.Config, userID int64, projectID string) (context.Context, error) {
	if cfg == nil {
		return ctx, fmt.Errorf("config unavailable")
	}
	cleanProjectID := filepath.Clean(projectID)
	if cleanProjectID != projectID || strings.HasPrefix(cleanProjectID, "..") || strings.Contains(cleanProjectID, string(filepath.Separator)+"..") || filepath.IsAbs(cleanProjectID) {
		return ctx, fmt.Errorf("invalid project_id")
	}
	if _, ok := sandbox.ProjectIDFromContext(ctx); !ok {
		ctx = sandbox.WithProjectID(ctx, cleanProjectID)
	}
	if _, ok := sandbox.BaseDirFromContext(ctx); ok {
		return ctx, nil
	}
	baseRoot := filepath.Join(cfg.Workdir, "users", fmt.Sprint(userID), "projects")
	baseDir := filepath.Join(baseRoot, cleanProjectID)
	if !strings.HasPrefix(baseDir, baseRoot+string(filepath.Separator)) && baseDir != baseRoot {
		return ctx, fmt.Errorf("invalid project_id")
	}
	return sandbox.WithBaseDir(ctx, baseDir), nil
}
