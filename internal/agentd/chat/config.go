package chat

import (
	"fmt"
	"strings"
	"time"

	"manifold/internal/agent/harness"
	"manifold/internal/agent/prompts"
	"manifold/internal/config"
	"manifold/internal/llm"
	"manifold/internal/tools"
	inputrequesttool "manifold/internal/tools/inputrequest"
)

func promptInstructionOverrides(cfg *config.Config) prompts.InstructionOverrides {
	if cfg == nil {
		return prompts.InstructionOverrides{}
	}
	return prompts.InstructionOverrides{
		BaseSystem:                 cfg.PromptOverrides.BaseSystem,
		MemoryInstructions:         cfg.PromptOverrides.MemoryInstructions,
		ToolDiscoveryInstructions:  cfg.PromptOverrides.ToolDiscoveryInstructions,
		SkillDiscoveryInstructions: cfg.PromptOverrides.SkillDiscoveryInstructions,
	}
}

func ensureDiscoveryInstructions(cfg *config.Config, systemPrompt string, enableTools, autoDiscover bool) string {
	if enableTools && autoDiscover {
		return prompts.EnsureToolDiscoveryInstructions(systemPrompt, promptInstructionOverrides(cfg))
	}
	return systemPrompt
}

func ensureRequestInfoInstructions(systemPrompt string, enableTools, requestInfo bool) string {
	if enableTools && requestInfo {
		return prompts.EnsureRequestInfoInstructions(systemPrompt)
	}
	return systemPrompt
}

func ensureMemoryInstructions(cfg *config.Config, systemPrompt string, enabled bool) string {
	if enabled {
		return prompts.EnsureMemoryInstructions(systemPrompt, promptInstructionOverrides(cfg))
	}
	return systemPrompt
}

func applySkillsMode(deps Deps, toolReg tools.Registry, systemPrompt, projectDir string, enableTools, autoDiscover bool) (tools.Registry, string, string) {
	if strings.TrimSpace(projectDir) == "" {
		return toolReg, systemPrompt, ""
	}
	if !enableTools {
		return toolReg, systemPrompt, prompts.RenderSkillsForProject(projectDir)
	}
	cached, err := prompts.CachedSkillsForProject(projectDir)
	if err != nil || cached == nil || len(cached.Skills) == 0 {
		return toolReg, systemPrompt, ""
	}
	if toolReg == nil {
		toolReg = tools.NewRegistry()
	}
	if !autoDiscover {
		if deps.NewSkillReadTool == nil {
			return toolReg, systemPrompt, cached.RenderedPrompt
		}
		return tools.NewOverlayRegistry(toolReg, deps.NewSkillReadTool(projectDir)), systemPrompt, cached.RenderedPrompt
	}
	if deps.NewSkillReadTool == nil || deps.NewSkillSearchTool == nil {
		return toolReg, systemPrompt, cached.RenderedPrompt
	}
	systemPrompt = prompts.EnsureSkillDiscoveryInstructions(systemPrompt, promptInstructionOverrides(deps.Cfg))
	return tools.NewOverlayRegistry(toolReg, deps.NewSkillReadTool(projectDir), deps.NewSkillSearchTool(projectDir)), systemPrompt, ""
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

func withInputRequestTool(reg tools.Registry, enabled bool) tools.Registry {
	if !enabled {
		return reg
	}
	if reg == nil {
		reg = tools.NewRegistry()
	}
	return tools.NewOverlayRegistry(reg, inputrequesttool.New())
}

func modelLabel(name, model string) string {
	if strings.TrimSpace(model) == "" {
		return name
	}
	return fmt.Sprintf("%s:%s", name, model)
}

func summaryContextSize(deps Deps, configured int, model string) int {
	if configured > 0 {
		return configured
	}
	if deps.Cfg != nil && deps.Cfg.SummaryContextWindowTokens > 0 {
		return deps.Cfg.SummaryContextWindowTokens
	}
	ctxSize, _ := llm.ContextSize(model)
	const defaultSummaryContextWindowCap = 32000
	if ctxSize <= 0 || ctxSize > defaultSummaryContextWindowCap {
		ctxSize = defaultSummaryContextWindowCap
	}
	return ctxSize
}

func harnessRunConfig(deps Deps, cfg config.HarnessConfig) harness.RunConfig {
	if deps.HarnessRunConfig != nil {
		return deps.HarnessRunConfig(cfg)
	}
	return harness.RunConfig{}
}

func harnessOverrideConfig(deps Deps, base config.HarnessConfig, override *config.HarnessConfig) config.HarnessConfig {
	if deps.HarnessOverrideConfig != nil {
		return deps.HarnessOverrideConfig(base, override)
	}
	if override != nil {
		return *override
	}
	return base
}

func summaryCallTimeout(deps Deps) time.Duration {
	if deps.SummaryCallTimeout != nil {
		return deps.SummaryCallTimeout(deps.Cfg)
	}
	return 0
}
