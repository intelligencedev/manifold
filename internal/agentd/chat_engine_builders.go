package agentd

import (
	"fmt"
	"strings"

	chatpkg "manifold/internal/agentd/chat"
	"manifold/internal/config"
	"manifold/internal/llm"
	"manifold/internal/tools"
	tooldiscovery "manifold/internal/tools/discovery"
	inputrequesttool "manifold/internal/tools/inputrequest"
)

type chatEngineBuildResult = chatpkg.BuildResult

type chatEngineBuildRequest = chatpkg.BuildRequest

func sanitizeImageGenerationBuild(build chatEngineBuildResult) chatEngineBuildResult {
	return chatpkg.SanitizeImageGenerationBuild(build)
}

func (a *app) resolveAutoDiscover(autoDiscover *bool) bool {
	resolved := a.cfg.AutoDiscover
	if autoDiscover != nil {
		resolved = *autoDiscover
	}
	return resolved
}

func (a *app) resolveRequestInfoEnabled(requestInfoEnabled *bool) bool {
	if requestInfoEnabled != nil {
		return *requestInfoEnabled
	}
	return config.RequestInfoEnabled(a.cfg.RequestInfoEnabled)
}

func combineUserPromptContext(parts ...string) string {
	sections := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			sections = append(sections, trimmed)
		}
	}
	return strings.Join(sections, "\n\n")
}

func (a *app) chatSummaryContextSize(configured int, model string) int {
	if configured > 0 {
		return configured
	}
	if a.cfg.SummaryContextWindowTokens > 0 {
		return a.cfg.SummaryContextWindowTokens
	}
	ctxSize, _ := llm.ContextSize(model)
	const defaultSummaryContextWindowCap = 32000
	if ctxSize <= 0 || ctxSize > defaultSummaryContextWindowCap {
		ctxSize = defaultSummaryContextWindowCap
	}
	return ctxSize
}

func (a *app) chatToolRegistry(enableTools bool, allowTools []string, autoDiscover, requestInfoEnabled *bool) tools.Registry {
	resolvedAutoDiscover := a.resolveAutoDiscover(autoDiscover)
	resolvedRequestInfo := a.resolveRequestInfoEnabled(requestInfoEnabled)
	var reg tools.Registry
	if resolvedAutoDiscover && enableTools && a.toolIndex != nil {
		reg = tooldiscovery.NewDiscoverableRegistry(a.baseToolRegistry, a.toolIndex, allowTools, a.cfg.MaxDiscoveredTools)
	} else {
		reg = tools.ApplyTopLevelPolicy(a.baseToolRegistry, enableTools, allowTools)
	}
	return withChatInputRequestTool(reg, enableTools && resolvedRequestInfo)
}

func withChatInputRequestTool(reg tools.Registry, enabled bool) tools.Registry {
	if !enabled {
		return reg
	}
	if reg == nil {
		reg = tools.NewRegistry()
	}
	return tools.NewOverlayRegistry(reg, inputrequesttool.New())
}

func chatModelLabel(name, model string) string {
	if strings.TrimSpace(model) == "" {
		return name
	}
	return fmt.Sprintf("%s:%s", name, model)
}
