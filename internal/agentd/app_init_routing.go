package agentd

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"manifold/internal/config"
	"manifold/internal/mcpclient"
	"manifold/internal/persistence"
	"manifold/internal/persistence/databases"
	"manifold/internal/specialists"
	"manifold/internal/tools"
	agenttools "manifold/internal/tools/agents"
	tooldiscovery "manifold/internal/tools/discovery"
	"manifold/internal/workspaces"
)

type appRouting struct {
	toolRegistry     tools.Registry
	toolIndex        *tooldiscovery.ToolIndex
	specRegistry     *specialists.Registry
	workspaceManager workspaces.WorkspaceManager
	mcpManager       *mcpclient.Manager
	mcpPool          *mcpclient.MCPServerPool
}

func initAppRouting(ctx context.Context, cfg *config.Config, httpClient *http.Client, mgr databases.Manager, tooling appTooling) appRouting {
	wsMgr := workspaces.NewManager(cfg)
	log.Info().Str("mode", wsMgr.Mode()).Msg("workspace_manager_initialized")

	specReg := specialists.NewRegistryWithWorkdir(cfg.LLMClient, cfg.Specialists, httpClient, tooling.registry, cfg.Workdir)
	specReg.SetMaxSteps(cfg.MaxSteps)
	specReg.SetPromptOverrides(promptInstructionOverrides(cfg))
	specReg.SetRequestInfoEnabled(config.RequestInfoEnabled(cfg.RequestInfoEnabled))
	registerSpecialistTools(cfg, httpClient, tooling.registry, specReg, wsMgr)

	mcpMgr := registerSharedMCPServers(ctx, cfg, mgr, tooling.baseRegistry)
	mcpPool := initMCPPool(ctx, cfg, mgr, wsMgr, tooling.baseRegistry)
	toolRegistry, toolIndex := applyToolDiscoveryPolicy(cfg, tooling.baseRegistry, specReg)

	return appRouting{
		toolRegistry:     toolRegistry,
		toolIndex:        toolIndex,
		specRegistry:     specReg,
		workspaceManager: wsMgr,
		mcpManager:       mcpMgr,
		mcpPool:          mcpPool,
	}
}

func registerSpecialistTools(cfg *config.Config, httpClient *http.Client, toolRegistry tools.Registry, specReg *specialists.Registry, wsMgr workspaces.WorkspaceManager) {
	agentCallTool := agenttools.NewAgentCallTool(toolRegistry, specReg, wsMgr)
	agentCallTool.SetDefaultMaxSteps(cfg.MaxSteps)
	agentCallTool.SetDefaultTimeoutSeconds(cfg.AgentRunTimeoutSeconds)
	toolRegistry.Register(agentCallTool)
	toolRegistry.Register(agenttools.NewAskAgentTool(httpClient, "http://127.0.0.1:32180", cfg.AgentRunTimeoutSeconds))
	toolRegistry.Register(agenttools.NewDelegateToTeamTool(httpClient, "http://127.0.0.1:32180", 0))
}

func registerSharedMCPServers(ctx context.Context, cfg *config.Config, mgr databases.Manager, baseToolRegistry tools.Registry) *mcpclient.Manager {
	mcpMgr := mcpclient.NewManager()
	ctxInit, cancelInit := context.WithTimeout(ctx, 30*time.Second)
	defer cancelInit()
	_ = mcpMgr.RegisterFromConfig(ctxInit, baseToolRegistry, cfg.MCP)
	if mgr.MCP == nil {
		return mcpMgr
	}
	servers, err := mgr.MCP.List(ctxInit, systemUserID)
	if err != nil {
		log.Warn().Err(err).Msg("failed to load mcp servers from db")
		return mcpMgr
	}
	registerPersistedMCPServers(ctxInit, cfg, mcpMgr, baseToolRegistry, servers)
	return mcpMgr
}

func registerPersistedMCPServers(ctx context.Context, cfg *config.Config, mcpMgr *mcpclient.Manager, baseToolRegistry tools.Registry, servers []persistence.MCPServer) {
	requiresPerUserMCP := cfg.Auth.Enabled && hasPathDependentMCPServers(cfg)
	pathDependentNames := pathDependentMCPNames(cfg)
	for _, s := range servers {
		if s.Disabled || s.URL != "" {
			if s.URL != "" {
				log.Debug().Str("server", s.Name).Msg("skipping_remote_db_mcp_server_during_init")
			}
			continue
		}
		cfgSrv := convertToConfig(s)
		if requiresPerUserMCP && shouldSkipPersistedMCPServer(s.Name, cfgSrv, pathDependentNames) {
			continue
		}
		_ = mcpMgr.RegisterOne(ctx, baseToolRegistry, cfgSrv)
	}
}

func hasPathDependentMCPServers(cfg *config.Config) bool {
	for _, srv := range cfg.MCP.Servers {
		if srv.PathDependent {
			return true
		}
	}
	return false
}

func pathDependentMCPNames(cfg *config.Config) map[string]bool {
	names := map[string]bool{}
	for _, s := range cfg.MCP.Servers {
		if s.PathDependent {
			names[s.Name] = true
		}
	}
	return names
}

func shouldSkipPersistedMCPServer(name string, cfgSrv config.MCPServerConfig, pathDependentNames map[string]bool) bool {
	if pathDependentNames[name] {
		log.Debug().Str("server", name).Msg("skipping_path_dependent_mcp_server_from_db")
		return true
	}
	if mcpServerHasProjectPlaceholder(cfgSrv) {
		log.Debug().Str("server", name).Msg("skipping_placeholder_mcp_server_from_db")
		return true
	}
	return false
}

func mcpServerHasProjectPlaceholder(cfgSrv config.MCPServerConfig) bool {
	for _, arg := range cfgSrv.Args {
		if strings.Contains(arg, "{{PROJECT_DIR}}") {
			return true
		}
	}
	if strings.Contains(cfgSrv.Workdir, "{{PROJECT_DIR}}") {
		return true
	}
	for _, value := range cfgSrv.Env {
		if strings.Contains(value, "{{PROJECT_DIR}}") {
			return true
		}
	}
	for _, defaults := range cfgSrv.ToolDefaults {
		for _, value := range defaults {
			if stringContainsProjectPlaceholder(value) {
				return true
			}
		}
	}
	return false
}

func stringContainsProjectPlaceholder(value any) bool {
	switch v := value.(type) {
	case string:
		return strings.Contains(v, "{{PROJECT_DIR}}")
	case []any:
		for _, item := range v {
			if stringContainsProjectPlaceholder(item) {
				return true
			}
		}
	case map[string]any:
		for _, item := range v {
			if stringContainsProjectPlaceholder(item) {
				return true
			}
		}
	}
	return false
}

func initMCPPool(ctx context.Context, cfg *config.Config, mgr databases.Manager, wsMgr workspaces.WorkspaceManager, baseToolRegistry tools.Registry) *mcpclient.MCPServerPool {
	mcpPool := mcpclient.NewMCPServerPool(cfg, wsMgr, mgr.UserPreferences)
	mcpPool.SetToolRegistry(baseToolRegistry)
	workspaces.SetCheckoutCallback(mcpPool.OnWorkspaceCheckout)
	ctxPool, cancelPool := context.WithTimeout(ctx, 30*time.Second)
	defer cancelPool()
	if err := mcpPool.RegisterFromConfig(ctxPool, baseToolRegistry); err != nil {
		log.Warn().Err(err).Msg("mcp_pool_registration_failed")
	}
	if mcpPool.RequiresPerUserMCP() {
		mcpPool.RegisterPathDependentToolsForDiscovery(ctxPool, baseToolRegistry)
		mcpPool.StartReaper(ctx, baseToolRegistry, 15*time.Minute, 1*time.Hour)
	}
	return mcpPool
}

func applyToolDiscoveryPolicy(cfg *config.Config, baseToolRegistry tools.Registry, specReg *specialists.Registry) (tools.Registry, *tooldiscovery.ToolIndex) {
	toolIndex := tooldiscovery.NewToolIndex(baseToolRegistry.Schemas())
	var toolRegistry tools.Registry
	if cfg.AutoDiscover && cfg.EnableTools {
		toolRegistry = tooldiscovery.NewDiscoverableRegistry(baseToolRegistry, toolIndex, cfg.ToolAllowList, cfg.MaxDiscoveredTools)
	} else {
		toolRegistry = tools.ApplyTopLevelPolicy(baseToolRegistry, cfg.EnableTools, cfg.ToolAllowList)
	}
	toolRegistry = withChatInputRequestTool(toolRegistry, cfg.EnableTools && config.RequestInfoEnabled(cfg.RequestInfoEnabled))
	specReg.SetToolDiscovery(toolIndex, cfg.AutoDiscover, cfg.MaxDiscoveredTools)
	specReg.SetRequestInfoEnabled(config.RequestInfoEnabled(cfg.RequestInfoEnabled))
	log.Info().Bool("enableTools", cfg.EnableTools).Bool("autoDiscover", cfg.AutoDiscover).Strs("allowList", cfg.ToolAllowList).Strs("tools", tools.SchemaNames(toolRegistry)).Msg("tool_registry_contents")
	return toolRegistry, toolIndex
}
