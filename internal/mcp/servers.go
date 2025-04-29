package mcp

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	mcp "github.com/metoro-io/mcp-golang"
	"github.com/metoro-io/mcp-golang/transport/stdio"
	"gopkg.in/yaml.v2"
)

// MCPClient defines the interface for MCP client operations
// This interface helps with testing by allowing us to mock client behavior
type MCPClient interface {
	ListTools(ctx context.Context, cursor *string) (*mcp.ToolsResponse, error)
	CallTool(ctx context.Context, name string, args interface{}) (*mcp.ToolResponse, error)
	Initialize(ctx context.Context) (*mcp.InitializeResponse, error)
}

// Ensure the standard client implements our interface
var _ MCPClient = (*mcp.Client)(nil)

// ServerConfig holds the command, arguments, and environment for an MCP server
// as defined under the `mcpServers` key in config.yaml.
type ServerConfig struct {
	Command string            `yaml:"command"`
	Args    []string          `yaml:"args"`
	Env     map[string]string `yaml:"env"`
}

// serversConfig is a helper for unmarshaling only the mcpServers section.
type serversConfig struct {
	Servers map[string]ServerConfig `yaml:"mcpServers"`
}

// LoadServerConfigs reads the `mcpServers` section from the given YAML file
// and returns a map of server names to ServerConfig.
func LoadServerConfigs(configPath string) (map[string]ServerConfig, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}
	var scfg serversConfig
	if err := yaml.Unmarshal(data, &scfg); err != nil {
		return nil, fmt.Errorf("unmarshaling mcpServers: %w", err)
	}
	return scfg.Servers, nil
}

// StartClientsFromConfig starts an mcp.Client for each ServerConfig under the given context.
// It returns a map of server names to initialized clients and a map of cleanup functions.
func StartClientsFromConfig(ctx context.Context, configs map[string]ServerConfig) (map[string]MCPClient, map[string]func() error, error) {
	clients := make(map[string]MCPClient)
	cleanups := make(map[string]func() error)
	for name, cfg := range configs {
		cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...)
		// propagate environment variables
		if len(cfg.Env) > 0 {
			env := os.Environ()
			for k, v := range cfg.Env {
				env = append(env, fmt.Sprintf("%s=%s", k, v))
			}
			cmd.Env = env
		}
		stdin, err := cmd.StdinPipe()
		if err != nil {
			cleanupAll(cleanups)
			return nil, nil, fmt.Errorf("stdin pipe for %s: %w", name, err)
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			cleanupAll(cleanups)
			return nil, nil, fmt.Errorf("stdout pipe for %s: %w", name, err)
		}
		if err := cmd.Start(); err != nil {
			cleanupAll(cleanups)
			return nil, nil, fmt.Errorf("starting %s: %w", name, err)
		}
		transport := stdio.NewStdioServerTransportWithIO(stdout, stdin)
		client := mcp.NewClient(transport)
		if _, err := client.Initialize(ctx); err != nil {
			cmd.Process.Kill()
			cleanupAll(cleanups)
			return nil, nil, fmt.Errorf("initializing client for %s: %w", name, err)
		}
		clients[name] = client
		cleanups[name] = func() error { return cmd.Process.Kill() }
	}
	return clients, cleanups, nil
}

// cleanupAll invokes all provided cleanup functions, used to teardown processes on error.
func cleanupAll(funcs map[string]func() error) {
	for _, f := range funcs {
		_ = f()
	}
}

// Manager manages multiple MCP server clients based on config.yaml.
// It loads server configurations, starts clients, and cleans up on Close().
type Manager struct {
	clients  map[string]MCPClient
	cleanups map[string]func() error
}

// NewManager creates and initializes MCP clients from the given config file.
func NewManager(ctx context.Context, configPath string) (*Manager, error) {
	configs, err := LoadServerConfigs(configPath)
	if err != nil {
		return nil, err
	}
	clients, cleanups, err := StartClientsFromConfig(ctx, configs)
	if err != nil {
		return nil, err
	}
	return &Manager{clients: clients, cleanups: cleanups}, nil
}

// List returns the names of all configured MCP servers.
func (m *Manager) List() []string {
	names := make([]string, 0, len(m.clients))
	for name := range m.clients {
		names = append(names, name)
	}
	return names
}

// Client retrieves the MCP client for the given server name.
// The boolean indicates whether the server name was found.
func (m *Manager) Client(name string) (MCPClient, bool) {
	c, ok := m.clients[name]
	return c, ok
}

// ListTools returns the available tools for the given server name.
func (m *Manager) ListTools(ctx context.Context, name string) ([]mcp.ToolRetType, error) {
	client, ok := m.Client(name)
	if !ok {
		return nil, fmt.Errorf("server %q not found", name)
	}
	resp, err := client.ListTools(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("listing tools on %s: %w", name, err)
	}
	return resp.Tools, nil
}

// CallTool invokes the specified tool with args on the given server.
func (m *Manager) CallTool(ctx context.Context, server, tool string, args interface{}) (*mcp.ToolResponse, error) {
	client, ok := m.Client(server)
	if !ok {
		return nil, fmt.Errorf("server %q not found", server)
	}
	res, err := client.CallTool(ctx, tool, args)
	if err != nil {
		return nil, fmt.Errorf("calling tool %q on %s: %w", tool, server, err)
	}
	return res, nil
}

// Close terminates all server processes managed by this Manager.
func (m *Manager) Close() {
	cleanupAll(m.cleanups)
}
