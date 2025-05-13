package mcp

import (
	"context"
	"fmt"
	"os"

	clientpkg "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"gopkg.in/yaml.v2"
)

// MCPClient defines the interface for MCP client operations.
// We use this to allow mocking in tests.
type MCPClient interface {
	Initialize(ctx context.Context, req mcp.InitializeRequest) (*mcp.InitializeResult, error)
	ListTools(ctx context.Context, req mcp.ListToolsRequest) (*mcp.ListToolsResult, error)
	CallTool(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error)
	Close() error
}

// Ensure the standard client satisfies our interface.
var _ MCPClient = (*clientpkg.Client)(nil)

// ServerConfig holds the command, args, and env for an MCP server.
type ServerConfig struct {
	Command string            `yaml:"command"`
	Args    []string          `yaml:"args"`
	Env     map[string]string `yaml:"env"`
}

// serversConfig is used only for unmarshaling the mcpServers section.
type serversConfig struct {
	Servers map[string]ServerConfig `yaml:"mcpServers"`
}

// LoadServerConfigs reads and parses only the `mcpServers` section.
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

// StartClientsFromConfig starts a stdio-based MCP client for each ServerConfig.
// Returns a map of names → clients, and a map of names → cleanup funcs.
func StartClientsFromConfig(ctx context.Context, configs map[string]ServerConfig) (
	map[string]MCPClient,
	map[string]func() error,
	error,
) {
	clients := make(map[string]MCPClient, len(configs))
	cleanups := make(map[string]func() error, len(configs))

	for name, cfg := range configs {
		// Build environment slice
		env := os.Environ()
		for k, v := range cfg.Env {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}

		// NewStdioMCPClient will launch the subprocess and wire up stdin/stdout.
		client, err := clientpkg.NewStdioMCPClient(cfg.Command, env, cfg.Args...)
		if err != nil {
			cleanupAll(cleanups)
			return nil, nil, fmt.Errorf("starting client for %q: %w", name, err)
		}

		// Initialize the client (handshake)
		if _, err := client.Initialize(ctx, mcp.InitializeRequest{}); err != nil {
			client.Close()
			cleanupAll(cleanups)
			return nil, nil, fmt.Errorf("initializing client for %q: %w", name, err)
		}

		clients[name] = client
		// We need to both Close() the client and kill the subprocess.
		// NewStdioMCPClient gives us access to c.GetTransport().Close(),
		// but Close() already does that. In addition, we track the process:
		cleanups[name] = func() error { return client.Close() }
	}

	return clients, cleanups, nil
}

// cleanupAll calls every registered cleanup func.
func cleanupAll(funcs map[string]func() error) {
	for _, f := range funcs {
		_ = f()
	}
}

// Manager holds a set of named MCP clients and their cleanup fns.
type Manager struct {
	clients  map[string]MCPClient
	cleanups map[string]func() error
}

// NewManager loads config, starts clients, and returns a Manager.
func NewManager(ctx context.Context, configPath string) (*Manager, error) {
	cfgs, err := LoadServerConfigs(configPath)
	if err != nil {
		return nil, err
	}
	clients, cleanups, err := StartClientsFromConfig(ctx, cfgs)
	if err != nil {
		return nil, err
	}
	return &Manager{
		clients:  clients,
		cleanups: cleanups,
	}, nil
}

// List returns all configured server names.
func (m *Manager) List() []string {
	names := make([]string, 0, len(m.clients))
	for name := range m.clients {
		names = append(names, name)
	}
	return names
}

// Client fetches a client by name.
func (m *Manager) Client(name string) (MCPClient, bool) {
	c, ok := m.clients[name]
	return c, ok
}

// ListTools pages through the tools available on the named server.
func (m *Manager) ListTools(ctx context.Context, server string) ([]mcp.Tool, error) {
	client, ok := m.Client(server)
	if !ok {
		return nil, fmt.Errorf("server %q not found", server)
	}
	// empty cursor/page → first page
	resp, err := client.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, fmt.Errorf("listing tools on %s: %w", server, err)
	}
	return resp.Tools, nil
}

// CallTool invokes the named tool with arbitrary params.
func (m *Manager) CallTool(
	ctx context.Context,
	server, tool string,
	params any,
) (*mcp.CallToolResult, error) {
	client, ok := m.clients[server]
	if !ok {
		return nil, fmt.Errorf("server %q not found", server)
	}

	// Assert that params is the expected map[string]any
	argsMap, ok := params.(map[string]any)
	if !ok {
		return nil, fmt.Errorf(
			"invalid params type for tool %q: expected map[string]any, got %T",
			tool, params,
		)
	}

	// Build the CallToolRequest by filling the embedded Params struct
	var req mcp.CallToolRequest
	req.Params.Name = tool
	req.Params.Arguments = argsMap
	// (Optional) If you need progress notifications you can also set:
	// req.Params.Meta = &struct{ ProgressToken mcp.ProgressToken }{ProgressToken: someToken}

	// Invoke the tool
	res, err := client.CallTool(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("calling tool %q on %s: %w", tool, server, err)
	}
	return res, nil
}

// Close terminates all subprocesses/clients.
func (m *Manager) Close() {
	cleanupAll(m.cleanups)
}
