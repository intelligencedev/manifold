package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"manifold/internal/file_editor"

	mcp "github.com/mark3labs/mcp-go/mcp"
)

func TestEditFileToolIntegration(t *testing.T) {
	// Create temporary workspace
	tmpDir, err := os.MkdirTemp("", "mcp-test-*")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Set DATA_PATH environment variable for test
	originalDataPath := os.Getenv("DATA_PATH")
	os.Setenv("DATA_PATH", tmpDir)
	defer os.Setenv("DATA_PATH", originalDataPath)

	// Reinitialize the global editor for testing
	globalEditor = nil
	initEditor()

	// Create a test file
	testContent := "line 1\nline 2\nline 3\nline 4\n"
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("creating test file: %v", err)
	}

	tests := []struct {
		name      string
		args      map[string]interface{}
		wantError bool
		checkFunc func(*testing.T, *mcp.CallToolResult)
	}{
		{
			name: "read file",
			args: map[string]interface{}{
				"operation": "read",
				"path":      "test.txt",
			},
			wantError: false,
			checkFunc: func(t *testing.T, result *mcp.CallToolResult) {
				if result.IsError {
					t.Errorf("unexpected error: %v", result)
					return
				}
				if len(result.Content) < 2 {
					t.Errorf("expected at least 2 content items, got %d", len(result.Content))
					return
				}
				// Check that content contains our test data
				content := result.Content[1].(mcp.TextContent).Text
				if !strings.Contains(content, "line 1") {
					t.Errorf("content doesn't contain expected text: %s", content)
				}
			},
		},
		{
			name: "read range",
			args: map[string]interface{}{
				"operation": "read_range",
				"path":      "test.txt",
				"start":     2,
				"end":       3,
			},
			wantError: false,
			checkFunc: func(t *testing.T, result *mcp.CallToolResult) {
				if result.IsError {
					t.Errorf("unexpected error: %v", result)
				}
				// Should contain lines 2 and 3
				content := result.Content[1].(mcp.TextContent).Text
				if !strings.Contains(content, "line 2\nline 3") {
					t.Errorf("content doesn't contain expected range")
				}
			},
		},
		{
			name: "search",
			args: map[string]interface{}{
				"operation": "search",
				"path":      "test.txt",
				"pattern":   "line [0-9]+",
			},
			wantError: false,
			checkFunc: func(t *testing.T, result *mcp.CallToolResult) {
				if result.IsError {
					t.Errorf("unexpected error: %v", result)
				}
				// Should find 4 matches
				content := result.Content[1].(mcp.TextContent).Text
				if !strings.Contains(content, "Line 1:") || !strings.Contains(content, "Line 4:") {
					t.Errorf("search didn't find expected matches")
				}
			},
		},
		{
			name: "replace line",
			args: map[string]interface{}{
				"operation":   "replace_line",
				"path":        "test.txt",
				"start":       2,
				"replacement": "REPLACED LINE 2",
			},
			wantError: false,
			checkFunc: func(t *testing.T, result *mcp.CallToolResult) {
				if result.IsError {
					t.Errorf("unexpected error: %v", result)
				}
				// Verify file was actually modified
				content, err := os.ReadFile(testFile)
				if err != nil {
					t.Fatalf("reading modified file: %v", err)
				}
				if !strings.Contains(string(content), "REPLACED LINE 2") {
					t.Errorf("file was not modified as expected")
				}
			},
		},
		{
			name: "insert after",
			args: map[string]interface{}{
				"operation":   "insert_after",
				"path":        "test.txt",
				"start":       2,
				"replacement": "INSERTED LINE",
			},
			wantError: false,
			checkFunc: func(t *testing.T, result *mcp.CallToolResult) {
				if result.IsError {
					t.Errorf("unexpected error: %v", result)
				}
				// Verify insertion
				content, err := os.ReadFile(testFile)
				if err != nil {
					t.Fatalf("reading modified file: %v", err)
				}
				if !strings.Contains(string(content), "INSERTED LINE") {
					t.Errorf("insertion was not applied as expected")
				}
			},
		},
		{
			name: "delete range",
			args: map[string]interface{}{
				"operation": "delete_range",
				"path":      "test.txt",
				"start":     2,
				"end":       3,
			},
			wantError: false,
			checkFunc: func(t *testing.T, result *mcp.CallToolResult) {
				if result.IsError {
					t.Errorf("unexpected error: %v", result)
				}
				// Verify deletion
				content, err := os.ReadFile(testFile)
				if err != nil {
					t.Fatalf("reading modified file: %v", err)
				}
				lines := strings.Split(string(content), "\n")
				// Should have fewer lines now
				if len(lines) >= 6 { // original had 5 lines including empty last line
					t.Errorf("deletion was not applied as expected, got %d lines", len(lines))
				}
			},
		},
		{
			name: "invalid operation",
			args: map[string]interface{}{
				"operation": "invalid_op",
				"path":      "test.txt",
			},
			wantError: true,
			checkFunc: func(t *testing.T, result *mcp.CallToolResult) {
				if !result.IsError {
					t.Errorf("expected error for invalid operation")
				}
			},
		},
		{
			name: "path outside workspace",
			args: map[string]interface{}{
				"operation": "read",
				"path":      "/etc/passwd",
			},
			wantError: true,
			checkFunc: func(t *testing.T, result *mcp.CallToolResult) {
				if !result.IsError {
					t.Errorf("expected error for path outside workspace")
				}
			},
		},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fresh test file for each test
			if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
				t.Fatalf("recreating test file: %v", err)
			}

			// Create request with proper arguments map
			request := mcp.CallToolRequest{}
			request.Params.Arguments = tt.args

			result, err := handleEditFileTool(ctx, request)
			if err != nil {
				if !tt.wantError {
					t.Errorf("handleEditFileTool() error = %v, wantError %v", err, tt.wantError)
				}
				return
			}

			if tt.checkFunc != nil {
				tt.checkFunc(t, result)
			}
		})
	}
}

// initEditor reinitializes the global editor (for testing)
func initEditor() {
	workspaceRoot := os.Getenv("DATA_PATH")
	if workspaceRoot == "" {
		workspaceRoot = "/tmp/manifold-workspace"
		os.MkdirAll(workspaceRoot, 0755)
	}

	var err error
	globalEditor, err = file_editor.NewEditor(workspaceRoot)
	if err != nil {
		log.Printf("Warning: Could not initialize file editor: %v", err)
	}
}
