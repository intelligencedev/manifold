package file_editor

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// MCPServer implements an MCP server for the file editor
type MCPServer struct {
	editor *Editor
	server *server.MCPServer
}

// NewMCPServer creates a new MCP server for file editing
func NewMCPServer(workspaceRoot string) (*MCPServer, error) {
	editor, err := NewEditor(workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("creating editor: %w", err)
	}

	mcpServer := server.NewMCPServer(
		"file-editor",
		"1.0.0",
		server.WithToolCapabilities(true),
		server.WithLogging(),
	)

	s := &MCPServer{
		editor: editor,
		server: mcpServer,
	}

	// Register the edit_file tool
	mcpServer.AddTool(mcp.NewTool("edit_file",
		mcp.WithDescription("High-precision, atomic edits to text files"),
		mcp.WithString("operation",
			mcp.Description("Type of operation to perform"),
			mcp.Enum("read", "read_range", "search", "replace_line",
				"replace_range", "insert_after", "delete_range",
				"apply_patch", "preview_patch"),
			mcp.Required(),
		),
		mcp.WithString("path",
			mcp.Description("Path to the file to edit (relative to workspace)"),
			mcp.Required(),
		),
		mcp.WithNumber("start",
			mcp.Description("1-based line number for range operations"),
		),
		mcp.WithNumber("end",
			mcp.Description("1-based end line number (inclusive) for range operations"),
		),
		mcp.WithString("pattern",
			mcp.Description("Regex or literal pattern for search operation"),
		),
		mcp.WithString("replacement",
			mcp.Description("Replacement or insertion text; may contain \\n"),
		),
		mcp.WithString("patch",
			mcp.Description("Unified-diff content for apply/preview_patch operations"),
		),
	), s.handleEditFile)

	return s, nil
}

// handleEditFile handles the edit_file tool calls
func (s *MCPServer) handleEditFile(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.Params.Arguments

	// Parse the request
	var req EditRequest
	if err := s.parseArgs(args, &req); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Error parsing arguments: %v", err),
				},
			},
			IsError: true,
		}, nil
	}

	// Resolve relative path to absolute path within workspace
	if !filepath.IsAbs(req.Path) {
		req.Path = filepath.Join(s.editor.workspaceRoot, req.Path)
	}

	// Perform the edit operation
	response, err := s.editor.Edit(ctx, req)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Internal error: %v", err),
				},
			},
			IsError: true,
		}, nil
	}

	// Convert response to MCP format
	return s.formatResponse(response), nil
}

// parseArgs parses the tool arguments into the request structure
func (s *MCPServer) parseArgs(args map[string]interface{}, req *EditRequest) error {
	// Marshal to JSON and back to properly convert types
	data, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("marshaling args: %w", err)
	}

	if err := json.Unmarshal(data, req); err != nil {
		return fmt.Errorf("unmarshaling args: %w", err)
	}

	// Validate required fields
	if req.Operation == "" {
		return fmt.Errorf("operation is required")
	}
	if req.Path == "" {
		return fmt.Errorf("path is required")
	}

	return nil
}

// formatResponse converts EditResponse to MCP CallToolResult
func (s *MCPServer) formatResponse(response EditResponse) *mcp.CallToolResult {
	if !response.Success {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Error: %s", response.Error),
				},
			},
			IsError: true,
		}
	}

	var content []mcp.Content

	// Add main message
	if response.Message != "" {
		content = append(content, mcp.TextContent{
			Type: "text",
			Text: response.Message,
		})
	}

	// Add file content if present (for read operations)
	if response.Content != "" {
		content = append(content, mcp.TextContent{
			Type: "text",
			Text: fmt.Sprintf("Content:\n%s", response.Content),
		})
	}

	// Add search matches if present
	if len(response.Matches) > 0 {
		matchText := "Matches:\n"
		for _, match := range response.Matches {
			matchText += fmt.Sprintf("Line %d: %s\n", match.LineNumber, match.Line)
		}
		content = append(content, mcp.TextContent{
			Type: "text",
			Text: matchText,
		})
	}

	// Add diff if present (for preview operations)
	if response.Diff != "" {
		content = append(content, mcp.TextContent{
			Type: "text",
			Text: fmt.Sprintf("Diff preview:\n%s", response.Diff),
		})
	}

	return &mcp.CallToolResult{
		Content: content,
		IsError: false,
	}
}

// GetServer returns the underlying MCP server
func (s *MCPServer) GetServer() *server.MCPServer {
	return s.server
}
