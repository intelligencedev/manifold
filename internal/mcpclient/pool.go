package mcpclient

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"manifold/internal/auth"
	"manifold/internal/config"
	"manifold/internal/persistence"
	"manifold/internal/tools"
	"manifold/internal/workspaces"
)

// MCPServerPool manages MCP server sessions with support for both
// shared (global) and per-user instances based on path dependency.
//
// In simple deployment mode (no auth), all MCP servers are shared globally.
// When auth is enabled, path-dependent MCP servers get per-user instances with
// project paths injected.
type MCPServerPool struct {
	mu  sync.RWMutex
	cfg *config.Config

	// shared holds sessions for non-path-dependent servers (shared globally)
	shared *Manager

	// perUser holds sessions for path-dependent servers
	// Key is userID
	perUser map[int64]*userMCPState

	// workspaceMgr resolves project IDs to filesystem paths
	workspaceMgr workspaces.WorkspaceManager

	// prefsStore retrieves user's active project
	prefsStore persistence.UserPreferencesStore

	// toolRegistry for registering/unregistering tools
	toolRegistry tools.Registry

	// pathDependentServers caches the list of path-dependent server configs
	pathDependentServers []config.MCPServerConfig

	// sharedToolNames tracks tools registered from shared servers (for cleanup)
	sharedToolNames map[string][]string
}

// userMCPState holds the MCP session state for a single user.
type userMCPState struct {
	manager       *Manager
	projectID     string
	workspacePath string
	lastAccess    time.Time
	toolNames     []string // tools registered by this user's sessions
}

// NewMCPServerPool creates a new MCP server pool.
func NewMCPServerPool(
	cfg *config.Config,
	wsMgr workspaces.WorkspaceManager,
	prefsStore persistence.UserPreferencesStore,
) *MCPServerPool {
	pool := &MCPServerPool{
		cfg:             cfg,
		shared:          NewManager(),
		perUser:         make(map[int64]*userMCPState),
		workspaceMgr:    wsMgr,
		prefsStore:      prefsStore,
		sharedToolNames: make(map[string][]string),
	}

	// Pre-categorize servers by path dependency
	for _, srv := range cfg.MCP.Servers {
		if srv.PathDependent {
			pool.pathDependentServers = append(pool.pathDependentServers, srv)
		}
	}

	return pool
}

// SetToolRegistry sets the tool registry for registering/unregistering MCP tools.
// This must be called before any per-user sessions are created.
func (p *MCPServerPool) SetToolRegistry(reg tools.Registry) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.toolRegistry = reg
}

// OnWorkspaceCheckout is a callback that can be registered with workspace managers
// to automatically set up MCP sessions when a workspace is checked out.
// This is the integration point between workspaces and MCP session management.
func (p *MCPServerPool) OnWorkspaceCheckout(ctx context.Context, userID int64, projectID, workspacePath string) {
	if !p.RequiresPerUserMCP() {
		return
	}
	if projectID == "" || workspacePath == "" {
		return
	}

	p.mu.RLock()
	reg := p.toolRegistry
	p.mu.RUnlock()

	if reg == nil {
		log.Warn().Int64("userID", userID).Msg("mcp_pool_no_tool_registry_for_checkout_callback")
		return
	}

	if err := p.EnsureUserSession(ctx, reg, userID, projectID, workspacePath); err != nil {
		log.Warn().Err(err).Int64("userID", userID).Str("projectID", projectID).Msg("mcp_session_setup_on_checkout_failed")
	}
}

// RequiresPerUserMCP returns true if the deployment is configured for
// per-user MCP instances (auth enabled + path-dependent servers).
func (p *MCPServerPool) RequiresPerUserMCP() bool {
	if p.cfg == nil {
		return false
	}
	if !p.cfg.Auth.Enabled {
		return false
	}
	return len(p.pathDependentServers) > 0
}

// RegisterFromConfig connects to configured MCP servers and registers tools.
// Non-path-dependent servers are registered to the shared manager.
// Path-dependent servers in enterprise mode are deferred until user context is available.
func (p *MCPServerPool) RegisterFromConfig(ctx context.Context, reg tools.Registry) error {
	for _, srv := range p.cfg.MCP.Servers {
		if srv.PathDependent && p.RequiresPerUserMCP() {
			// Skip path-dependent servers in enterprise mode - they're registered per-user
			log.Debug().Str("server", srv.Name).Msg("deferring_path_dependent_mcp_server")
			continue
		}

		// For simple mode or non-path-dependent servers: register to shared manager
		if err := p.shared.RegisterOne(ctx, reg, srv); err != nil {
			log.Warn().Err(err).Str("server", srv.Name).Msg("shared_mcp_register_failed")
			continue
		}

		// Track tool names for this server
		p.mu.Lock()
		p.sharedToolNames[srv.Name] = p.shared.toolNames[srv.Name]
		p.mu.Unlock()
	}
	return nil
}

// PathDependentServerNames returns the names of path-dependent MCP servers.
// These servers are only available when a project is selected.
// Useful for UI to show which MCP servers will be available.
func (p *MCPServerPool) PathDependentServerNames() []string {
	names := make([]string, 0, len(p.pathDependentServers))
	for _, srv := range p.pathDependentServers {
		names = append(names, srv.Name)
	}
	return names
}

// RegisterPathDependentToolsForDiscovery temporarily starts path-dependent MCP servers
// with a temp directory to enumerate their tools for UI display.
// The servers are immediately closed after enumeration; actual tool execution
// requires a proper workspace checkout.
func (p *MCPServerPool) RegisterPathDependentToolsForDiscovery(ctx context.Context, reg tools.Registry) {
	if len(p.pathDependentServers) == 0 {
		return
	}

	log.Info().Int("servers", len(p.pathDependentServers)).Msg("mcp_tool_discovery_start")

	// Create a temp directory for tool discovery
	// NOTE: On macOS, Docker Desktop only shares certain paths by default (/Users, /tmp, /private/tmp).
	// The default os.TempDir() returns /var/folders/... which is NOT shared.
	// We explicitly use /tmp which Docker can access.
	tmpDir, err := os.MkdirTemp("/tmp", "mcp-discovery-*")
	if err != nil {
		log.Warn().Err(err).Msg("failed_to_create_temp_dir_for_mcp_discovery")
		return
	}
	log.Debug().Str("tmpDir", tmpDir).Msg("mcp_discovery_temp_dir")
	defer os.RemoveAll(tmpDir)

	discoveryMgr := NewManager()
	defer discoveryMgr.Close()

	for _, srv := range p.pathDependentServers {
		resolved := p.resolveServerConfig(srv, tmpDir)

		// Try to connect and enumerate tools
		if err := discoveryMgr.RegisterOne(ctx, reg, resolved); err != nil {
			log.Warn().Err(err).Str("server", srv.Name).Msg("mcp_tool_discovery_failed")
			continue
		}

		// Track discovered tools for this server
		p.mu.Lock()
		if names, ok := discoveryMgr.toolNames[srv.Name]; ok {
			log.Info().Str("server", srv.Name).Int("tools", len(names)).Strs("toolNames", names).Msg("mcp_tools_discovered")
		}
		p.mu.Unlock()
	}
}

// EnsureUserSession ensures MCP servers are running for the user's active project.
// This is called when:
// - User logs in (loads their persisted active project)
// - User switches projects via the preferences API
//
// For simple deployments, this is a no-op.
func (p *MCPServerPool) EnsureUserSession(
	ctx context.Context,
	reg tools.Registry,
	userID int64,
	projectID string,
	workspacePath string,
) error {
	if !p.RequiresPerUserMCP() {
		return nil // Simple mode - all servers are shared
	}

	if len(p.pathDependentServers) == 0 {
		return nil // No path-dependent servers configured
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	state, exists := p.perUser[userID]
	if exists && state.projectID == projectID && state.workspacePath == workspacePath {
		// Already configured for this project - just update access time
		state.lastAccess = time.Now()
		return nil
	}

	// Close existing sessions if project changed
	if exists {
		log.Debug().Int64("userID", userID).Str("oldProject", state.projectID).Str("newProject", projectID).Msg("switching_user_mcp_project")
		p.cleanupUserStateLocked(reg, userID, state)
	}

	// Create new manager with resolved paths
	mgr := NewManager()
	var registeredTools []string

	for _, srv := range p.pathDependentServers {
		resolved := p.resolveServerConfig(srv, workspacePath)
		if err := mgr.RegisterOne(ctx, reg, resolved); err != nil {
			log.Warn().Err(err).Str("server", srv.Name).Int64("userID", userID).Msg("user_mcp_register_failed")
			continue
		}
		// Track registered tools
		registeredTools = append(registeredTools, mgr.toolNames[srv.Name]...)
	}

	p.perUser[userID] = &userMCPState{
		manager:       mgr,
		projectID:     projectID,
		workspacePath: workspacePath,
		lastAccess:    time.Now(),
		toolNames:     registeredTools,
	}

	log.Info().
		Int64("userID", userID).
		Str("projectID", projectID).
		Str("workspacePath", workspacePath).
		Int("toolCount", len(registeredTools)).
		Msg("user_mcp_session_created")

	return nil
}

// resolveServerConfig expands {{PROJECT_DIR}} placeholders in args and env.
// NOTE: We use {{PROJECT_DIR}} instead of ${PROJECT_DIR} because the config loader
// runs os.ExpandEnv which would expand ${...} syntax using actual environment variables.
func (p *MCPServerPool) resolveServerConfig(srv config.MCPServerConfig, projectDir string) config.MCPServerConfig {
	resolved := srv

	// Expand args
	if len(srv.Args) > 0 {
		resolved.Args = make([]string, len(srv.Args))
		for i, arg := range srv.Args {
			resolved.Args[i] = strings.ReplaceAll(arg, "{{PROJECT_DIR}}", projectDir)
		}
	}

	// Expand env
	if len(srv.Env) > 0 {
		resolved.Env = make(map[string]string, len(srv.Env))
		for k, v := range srv.Env {
			resolved.Env[k] = strings.ReplaceAll(v, "{{PROJECT_DIR}}", projectDir)
		}
	}

	return resolved
}

// GetSession returns the appropriate MCP session for a tool call.
// It routes to shared or per-user sessions based on server configuration.
func (p *MCPServerPool) GetSession(ctx context.Context, serverName string) (*Manager, error) {
	// Find the server config to check if it's path-dependent
	var isPathDependent bool
	for _, srv := range p.cfg.MCP.Servers {
		if srv.Name == serverName {
			isPathDependent = srv.PathDependent
			break
		}
	}

	if !isPathDependent || !p.RequiresPerUserMCP() {
		// Return shared session manager
		return p.shared, nil
	}

	// Get user from context for per-user session
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		log.Warn().Str("server", serverName).Msg("user_context_required_for_path_dependent_mcp")
		// Fall back to shared session (may fail if server not registered there)
		return p.shared, nil
	}

	p.mu.RLock()
	state, exists := p.perUser[user.ID]
	p.mu.RUnlock()

	if !exists {
		log.Warn().Int64("userID", user.ID).Str("server", serverName).Msg("no_mcp_session_for_user")
		// Return shared manager as fallback - the tool call may fail
		return p.shared, nil
	}

	state.lastAccess = time.Now()
	return state.manager, nil
}

// CleanupUser removes all MCP sessions for a user (called on logout).
func (p *MCPServerPool) CleanupUser(reg tools.Registry, userID int64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if state, exists := p.perUser[userID]; exists {
		p.cleanupUserStateLocked(reg, userID, state)
		delete(p.perUser, userID)
		log.Info().Int64("userID", userID).Msg("user_mcp_sessions_cleaned_up")
	}
}

// cleanupUserStateLocked closes user MCP sessions and unregisters tools.
// Caller must hold p.mu lock.
func (p *MCPServerPool) cleanupUserStateLocked(reg tools.Registry, userID int64, state *userMCPState) {
	if state == nil {
		return
	}

	// Unregister tools from the shared registry
	for _, toolName := range state.toolNames {
		reg.Unregister(toolName)
	}

	// Close the manager (closes all underlying MCP sessions)
	state.manager.Close()
}

// StartReaper starts a background goroutine that cleans up idle user sessions.
func (p *MCPServerPool) StartReaper(ctx context.Context, reg tools.Registry, interval, maxIdle time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.reapIdleSessions(reg, maxIdle)
			}
		}
	}()
}

// reapIdleSessions removes user sessions that haven't been accessed within maxIdle.
func (p *MCPServerPool) reapIdleSessions(reg tools.Registry, maxIdle time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	var toDelete []int64

	for userID, state := range p.perUser {
		if now.Sub(state.lastAccess) > maxIdle {
			toDelete = append(toDelete, userID)
		}
	}

	for _, userID := range toDelete {
		if state, exists := p.perUser[userID]; exists {
			p.cleanupUserStateLocked(reg, userID, state)
			delete(p.perUser, userID)
			log.Info().Int64("userID", userID).Dur("idleTime", now.Sub(state.lastAccess)).Msg("reaped_idle_user_mcp_session")
		}
	}
}

// Close closes all MCP sessions (shared and per-user).
func (p *MCPServerPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Close shared sessions
	p.shared.Close()

	// Close all per-user sessions
	for _, state := range p.perUser {
		state.manager.Close()
	}
	p.perUser = make(map[int64]*userMCPState)
}

// ActiveUserCount returns the number of users with active MCP sessions.
func (p *MCPServerPool) ActiveUserCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.perUser)
}

// UserHasSession checks if a user has an active MCP session.
func (p *MCPServerPool) UserHasSession(userID int64) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	_, exists := p.perUser[userID]
	return exists
}

// GetUserProjectID returns the project ID for a user's current MCP session.
func (p *MCPServerPool) GetUserProjectID(userID int64) (string, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if state, exists := p.perUser[userID]; exists {
		return state.projectID, true
	}
	return "", false
}
