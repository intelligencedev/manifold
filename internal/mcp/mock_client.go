package mcp

import (
	"context"

	mcp "github.com/metoro-io/mcp-golang"
)

// ToolRetType represents a tool definition with its schema
type ToolRetType struct {
	// A human-readable description of the tool.
	Description *string `json:"description,omitempty" yaml:"description,omitempty" mapstructure:"description,omitempty"`

	// A JSON Schema object defining the expected parameters for the tool.
	InputSchema interface{} `json:"inputSchema" yaml:"inputSchema" mapstructure:"inputSchema"`

	// The name of the tool.
	Name string `json:"name" yaml:"name" mapstructure:"name"`
}

// MockMCPClient is a mock implementation of the MCPClient interface for testing
type MockMCPClient struct {
	// ListToolsFunc allows test to customize the ListTools behavior
	ListToolsFunc func(ctx context.Context, cursor *string) (*mcp.ToolsResponse, error)

	// CallToolFunc allows test to customize the CallTool behavior
	CallToolFunc func(ctx context.Context, name string, args interface{}) (*mcp.ToolResponse, error)

	// InitializeFunc allows test to customize the Initialize behavior
	InitializeFunc func(ctx context.Context) (*mcp.InitializeResponse, error)

	// Records the calls for verification in tests
	ListToolsCalls  []ListToolsCall
	CallToolCalls   []CallToolCall
	InitializeCalls int

	// Sample tool schemas for testing
	ToolSchemas map[string]interface{}
}

// ListToolsCall records parameters from a ListTools call
type ListToolsCall struct {
	Ctx    context.Context
	Cursor *string
}

// CallToolCall records parameters from a CallTool call
type CallToolCall struct {
	Ctx  context.Context
	Name string
	Args interface{}
}

// Ensure MockMCPClient implements MCPClient
var _ MCPClient = (*MockMCPClient)(nil)

// NewMockMCPClient creates a new MockMCPClient with default schemas
func NewMockMCPClient() *MockMCPClient {
	mock := &MockMCPClient{
		ToolSchemas: map[string]interface{}{},
	}

	// Add default test schemas
	mock.AddToolSchema("list_directory", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The path to list",
			},
			"recursive": map[string]interface{}{
				"type":        "boolean",
				"description": "Whether to recursively list directories",
			},
		},
		"required": []string{"path"},
	})

	mock.AddToolSchema("read_file", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The path to the file",
			},
		},
		"required": []string{"path"},
	})

	mock.AddToolSchema("write_file", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The path to the file",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "The content to write",
			},
		},
		"required": []string{"path", "content"},
	})

	return mock
}

// AddToolSchema adds a tool schema to the mock client
func (m *MockMCPClient) AddToolSchema(toolName string, schema interface{}) {
	if m.ToolSchemas == nil {
		m.ToolSchemas = map[string]interface{}{}
	}
	m.ToolSchemas[toolName] = schema
}

// ListTools implements MCPClient.ListTools
func (m *MockMCPClient) ListTools(ctx context.Context, cursor *string) (*mcp.ToolsResponse, error) {
	m.ListToolsCalls = append(m.ListToolsCalls, ListToolsCall{Ctx: ctx, Cursor: cursor})
	if m.ListToolsFunc != nil {
		return m.ListToolsFunc(ctx, cursor)
	}

	// Create default response with schemas
	tools := []mcp.ToolRetType{}
	for name, schema := range m.ToolSchemas {
		desc := "Mock tool " + name
		tools = append(tools, mcp.ToolRetType{
			Name:        name,
			Description: &desc,
			InputSchema: schema,
		})
	}

	// If no schemas defined, return some default tools
	if len(tools) == 0 {
		desc1 := "Default mock tool 1"
		desc2 := "Default mock tool 2"
		tools = []mcp.ToolRetType{
			{
				Name:        "mock_tool_1",
				Description: &desc1,
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"arg1": map[string]interface{}{
							"type":        "string",
							"description": "First argument",
						},
					},
				},
			},
			{
				Name:        "mock_tool_2",
				Description: &desc2,
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"arg1": map[string]interface{}{
							"type":        "number",
							"description": "First argument",
						},
						"arg2": map[string]interface{}{
							"type":        "number",
							"description": "Second argument",
						},
					},
					"required": []string{"arg1", "arg2"},
				},
			},
		}
	}

	return &mcp.ToolsResponse{
		Tools: tools,
	}, nil
}

// CallTool implements MCPClient.CallTool
func (m *MockMCPClient) CallTool(ctx context.Context, name string, args interface{}) (*mcp.ToolResponse, error) {
	m.CallToolCalls = append(m.CallToolCalls, CallToolCall{Ctx: ctx, Name: name, Args: args})
	if m.CallToolFunc != nil {
		return m.CallToolFunc(ctx, name, args)
	}
	// Default implementation returning empty response
	return &mcp.ToolResponse{}, nil
}

// Initialize implements MCPClient.Initialize
func (m *MockMCPClient) Initialize(ctx context.Context) (*mcp.InitializeResponse, error) {
	m.InitializeCalls++
	if m.InitializeFunc != nil {
		return m.InitializeFunc(ctx)
	}
	// Default implementation returning empty response
	return &mcp.InitializeResponse{}, nil
}
