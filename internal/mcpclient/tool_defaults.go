package mcpclient

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"manifold/internal/sandbox"
)

type mcpDefaultTemplateData struct {
	ProjectDir string
	ServerName string
	ToolName   string
	Timestamp  string
}

func (t *mcpTool) applyDefaults(ctx context.Context, args any) any {
	if len(t.defaults) == 0 {
		return args
	}
	argMap, ok := args.(map[string]any)
	if !ok {
		return args
	}

	data := t.templateData(ctx)
	rendered := renderMCPDefaultTemplates(t.defaults, data)
	for key, value := range rendered {
		if existing, ok := argMap[key]; ok && !isEmptyMCPDefaultValue(existing) {
			continue
		}
		argMap[key] = value
	}
	ensureMCPFilePathParent(argMap, data.ProjectDir)
	return argMap
}

func (t *mcpTool) templateData(ctx context.Context) mcpDefaultTemplateData {
	projectDir := strings.TrimSpace(t.workdir)
	if ctxProjectDir, ok := sandbox.BaseDirFromContext(ctx); ok && strings.TrimSpace(ctxProjectDir) != "" {
		projectDir = strings.TrimSpace(ctxProjectDir)
	}
	return mcpDefaultTemplateData{
		ProjectDir: projectDir,
		ServerName: t.server,
		ToolName:   t.tool.Name,
		Timestamp:  time.Now().UTC().Format("20060102T150405.000000000Z"),
	}
}

func renderMCPDefaultTemplates(defaults map[string]any, data mcpDefaultTemplateData) map[string]any {
	out := make(map[string]any, len(defaults))
	for key, value := range defaults {
		out[key] = renderMCPDefaultTemplateValue(value, data)
	}
	return out
}

func renderMCPDefaultTemplateValue(value any, data mcpDefaultTemplateData) any {
	switch v := value.(type) {
	case string:
		return renderMCPDefaultString(v, data)
	case map[string]any:
		return renderMCPDefaultTemplates(v, data)
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = renderMCPDefaultTemplateValue(item, data)
		}
		return out
	default:
		return value
	}
}

func renderMCPDefaultString(value string, data mcpDefaultTemplateData) string {
	replacements := []struct {
		token string
		value string
	}{
		{"{{PROJECT_DIR}}", data.ProjectDir},
		{"{{SERVER_NAME}}", data.ServerName},
		{"{{TOOL_NAME}}", data.ToolName},
		{"{{TIMESTAMP}}", data.Timestamp},
	}
	for _, replacement := range replacements {
		if replacement.value != "" {
			value = strings.ReplaceAll(value, replacement.token, replacement.value)
		}
	}
	return value
}

func isEmptyMCPDefaultValue(value any) bool {
	switch v := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(v) == ""
	default:
		return false
	}
}

func ensureMCPFilePathParent(args map[string]any, projectDir string) {
	rawFilePath, ok := args["filePath"].(string)
	if !ok {
		return
	}
	filePath := strings.TrimSpace(rawFilePath)
	if filePath == "" {
		return
	}
	dir := filepath.Dir(filePath)
	if dir == "." || dir == "" {
		return
	}
	if !filepath.IsAbs(dir) {
		if strings.TrimSpace(projectDir) == "" {
			return
		}
		dir = filepath.Join(projectDir, dir)
	}
	_ = os.MkdirAll(dir, 0o755)
}
