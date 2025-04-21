package mcp

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	mcp "github.com/metoro-io/mcp-golang"
)

func TestMockClientWithSchemas(t *testing.T) {
	// Create a new mock client with default schemas
	mock := NewMockMCPClient()

	// Add a custom schema
	mock.AddToolSchema("custom_tool", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search query",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum number of results",
				"default":     10,
			},
		},
		"required": []string{"query"},
	})

	// List tools and verify schemas
	ctx := context.Background()
	resp, err := mock.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}

	// Verify we have the expected number of tools
	// 3 default + 1 custom = 4
	if len(resp.Tools) != 4 {
		t.Errorf("Expected 4 tools, got %d", len(resp.Tools))
	}

	// Find and verify the custom tool schema
	var customTool *mcp.ToolRetType
	for _, tool := range resp.Tools {
		if tool.Name == "custom_tool" {
			customTool = &tool
			break
		}
	}

	if customTool == nil {
		t.Fatalf("Custom tool not found in response")
	}

	// Convert schema to string for easier inspection
	schemaJSON, err := json.Marshal(customTool.InputSchema)
	if err != nil {
		t.Fatalf("Failed to marshal schema: %v", err)
	}

	// Verify the schema has required properties
	schemaMap, ok := customTool.InputSchema.(map[string]interface{})
	if !ok {
		t.Fatalf("Schema is not a map: %s", string(schemaJSON))
	}

	props, ok := schemaMap["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("Schema properties not found or invalid type")
	}

	if _, ok := props["query"]; !ok {
		t.Errorf("Schema missing 'query' property")
	}

	if _, ok := props["limit"]; !ok {
		t.Errorf("Schema missing 'limit' property")
	}

	required, ok := schemaMap["required"].([]string)
	if !ok {
		t.Fatalf("Required property not found or invalid type")
	}

	if len(required) != 1 || required[0] != "query" {
		t.Errorf("Expected required=[\"query\"], got %v", required)
	}
}

func TestManagerWithMockClients(t *testing.T) {
	// Create mock clients with schemas
	githubClient := NewMockMCPClient()
	githubClient.AddToolSchema("github_search", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search query",
			},
			"owner": map[string]interface{}{
				"type":        "string",
				"description": "Repository owner",
			},
			"repo": map[string]interface{}{
				"type":        "string",
				"description": "Repository name",
			},
		},
		"required": []string{"query"},
	})

	customClient := NewMockMCPClient()
	customClient.AddToolSchema("custom_tool", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"input": map[string]interface{}{
				"type":        "string",
				"description": "Input data",
			},
		},
		"required": []string{"input"},
	})

	// Set up custom response for github client's CallTool
	githubClient.CallToolFunc = func(ctx context.Context, name string, args interface{}) (*mcp.ToolResponse, error) {
		return &mcp.ToolResponse{
			Content: []*mcp.Content{
				{
					TextContent: &mcp.TextContent{
						Text: "Tool response for " + name,
					},
				},
			},
		}, nil
	}

	// Create a manager with mock clients
	m := &Manager{
		clients: map[string]MCPClient{
			"github": githubClient,
			"custom": customClient,
		},
		cleanups: map[string]func() error{
			"github": func() error { return nil },
			"custom": func() error { return nil },
		},
	}

	// Test List method
	serverNames := m.List()
	if len(serverNames) != 2 {
		t.Errorf("Expected 2 servers, got %d", len(serverNames))
	}

	// Test ListTools method with schemas
	ctx := context.Background()
	tools, err := m.ListTools(ctx, "github")
	if err != nil {
		t.Errorf("Unexpected error listing tools: %v", err)
	}

	// Find the github_search tool and verify its schema
	var searchTool *mcp.ToolRetType
	for i := range tools {
		if tools[i].Name == "github_search" {
			searchTool = &tools[i]
			break
		}
	}

	if searchTool == nil {
		t.Fatalf("github_search tool not found")
	}

	// Verify schema properties
	schemaMap, ok := searchTool.InputSchema.(map[string]interface{})
	if !ok {
		t.Fatalf("Schema is not a map")
	}

	props, ok := schemaMap["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("Schema properties not found or invalid type")
	}

	if _, ok := props["query"]; !ok {
		t.Errorf("Schema missing 'query' property")
	}

	// Test CallTool with schema-compliant args
	resp, err := m.CallTool(ctx, "github", "github_search", map[string]string{
		"query": "test query",
		"owner": "test-owner",
	})
	if err != nil {
		t.Errorf("Unexpected error calling tool: %v", err)
	}
	if resp == nil || len(resp.Content) == 0 || resp.Content[0].TextContent == nil {
		t.Errorf("Expected non-empty tool response")
	}
	expectedText := "Tool response for github_search"
	if resp.Content[0].TextContent.Text != expectedText {
		t.Errorf("Expected tool response text %q, got %q", expectedText, resp.Content[0].TextContent.Text)
	}
}

func TestLoadServerConfigs(t *testing.T) {
	// Create a temporary config file for testing
	configContent := `
mcpServers:
  test-server:
    command: echo
    args:
      - hello
      - world
    env:
      TEST_VAR: test_value
  empty-server:
    command: cat
`
	tmpfile, err := os.CreateTemp("", "config*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(configContent)); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}

	// Test loading config
	configs, err := LoadServerConfigs(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to load server configs: %v", err)
	}

	// Verify server count
	if len(configs) != 2 {
		t.Errorf("Expected 2 servers, got %d", len(configs))
	}

	// Verify first server
	server, ok := configs["test-server"]
	if !ok {
		t.Errorf("Expected to find 'test-server'")
	}
	if server.Command != "echo" {
		t.Errorf("Expected command 'echo', got %q", server.Command)
	}
	if len(server.Args) != 2 || server.Args[0] != "hello" || server.Args[1] != "world" {
		t.Errorf("Expected args ['hello', 'world'], got %v", server.Args)
	}
	if len(server.Env) != 1 || server.Env["TEST_VAR"] != "test_value" {
		t.Errorf("Expected env {'TEST_VAR': 'test_value'}, got %v", server.Env)
	}

	// Verify second server
	server, ok = configs["empty-server"]
	if !ok {
		t.Errorf("Expected to find 'empty-server'")
	}
	if server.Command != "cat" {
		t.Errorf("Expected command 'cat', got %q", server.Command)
	}
	if len(server.Args) != 0 {
		t.Errorf("Expected empty args, got %v", server.Args)
	}
	if len(server.Env) != 0 {
		t.Errorf("Expected empty env, got %v", server.Env)
	}
}

// Helper function to create a string pointer
func stringPtr(s string) *string {
	return &s
}
