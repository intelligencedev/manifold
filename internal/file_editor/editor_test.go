package file_editor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// createTempWorkspace creates a temporary workspace for testing
func createTempWorkspace(t *testing.T) (string, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "file-editor-test-*")
	if err != nil {
		t.Fatalf("creating temp workspace: %v", err)
	}

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return tmpDir, cleanup
}

// createTestFile creates a test file with specified content
func createTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("creating test file %s: %v", path, err)
	}

	return path
}

func TestNewEditor(t *testing.T) {
	workspace, cleanup := createTempWorkspace(t)
	defer cleanup()

	editor, err := NewEditor(workspace)
	if err != nil {
		t.Fatalf("NewEditor() error = %v", err)
	}

	if editor.workspaceRoot != workspace {
		t.Errorf("NewEditor() workspace = %v, want %v", editor.workspaceRoot, workspace)
	}
}

func TestEditor_ValidatePath(t *testing.T) {
	workspace, cleanup := createTempWorkspace(t)
	defer cleanup()

	editor, err := NewEditor(workspace)
	if err != nil {
		t.Fatalf("NewEditor() error = %v", err)
	}

	tests := []struct {
		name    string
		path    string
		wantErr bool
		errType error
	}{
		{
			name:    "valid path within workspace",
			path:    filepath.Join(workspace, "test.txt"),
			wantErr: false,
		},
		{
			name:    "path outside workspace",
			path:    "/etc/passwd",
			wantErr: true,
			errType: ErrPathOutsideRoot,
		},
		{
			name:    "relative path that stays within workspace",
			path:    "subdir/test.txt",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := editor.validatePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errType != nil {
				if !strings.Contains(err.Error(), tt.errType.Error()) {
					t.Errorf("validatePath() error = %v, want error containing %v", err, tt.errType)
				}
			}
		})
	}
}

func TestEditor_HandleRead(t *testing.T) {
	workspace, cleanup := createTempWorkspace(t)
	defer cleanup()

	editor, err := NewEditor(workspace)
	if err != nil {
		t.Fatalf("NewEditor() error = %v", err)
	}

	// Create test file
	content := "line 1\nline 2\nline 3\n"
	testFile := createTestFile(t, workspace, "test.txt", content)

	response, err := editor.handleRead(testFile)
	if err != nil {
		t.Fatalf("handleRead() error = %v", err)
	}

	if !response.Success {
		t.Fatalf("handleRead() success = false, error = %v", response.Error)
	}

	if response.Content != content {
		t.Errorf("handleRead() content = %q, want %q", response.Content, content)
	}
}

func TestEditor_HandleReadRange(t *testing.T) {
	workspace, cleanup := createTempWorkspace(t)
	defer cleanup()

	editor, err := NewEditor(workspace)
	if err != nil {
		t.Fatalf("NewEditor() error = %v", err)
	}

	// Create test file
	content := "line 1\nline 2\nline 3\nline 4\n"
	testFile := createTestFile(t, workspace, "test.txt", content)

	tests := []struct {
		name        string
		start       *int
		end         *int
		wantContent string
		wantSuccess bool
	}{
		{
			name:        "read single line",
			start:       intPtr(2),
			end:         intPtr(2),
			wantContent: "line 2",
			wantSuccess: true,
		},
		{
			name:        "read range",
			start:       intPtr(2),
			end:         intPtr(3),
			wantContent: "line 2\nline 3",
			wantSuccess: true,
		},
		{
			name:        "read from start to end",
			start:       intPtr(1),
			end:         nil,
			wantContent: "line 1\nline 2\nline 3\nline 4",
			wantSuccess: true,
		},
		{
			name:        "invalid start line",
			start:       intPtr(0),
			end:         nil,
			wantSuccess: false,
		},
		{
			name:        "start beyond file",
			start:       intPtr(10),
			end:         nil,
			wantSuccess: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := editor.handleReadRange(testFile, tt.start, tt.end)
			if err != nil {
				t.Fatalf("handleReadRange() error = %v", err)
			}

			if response.Success != tt.wantSuccess {
				t.Errorf("handleReadRange() success = %v, want %v", response.Success, tt.wantSuccess)
				return
			}

			if tt.wantSuccess && response.Content != tt.wantContent {
				t.Errorf("handleReadRange() content = %q, want %q", response.Content, tt.wantContent)
			}
		})
	}
}

func TestEditor_HandleSearch(t *testing.T) {
	workspace, cleanup := createTempWorkspace(t)
	defer cleanup()

	editor, err := NewEditor(workspace)
	if err != nil {
		t.Fatalf("NewEditor() error = %v", err)
	}

	// Create test file
	content := "line 1\ntest line\nline 3\nanother test\nline 5\n"
	testFile := createTestFile(t, workspace, "test.txt", content)

	tests := []struct {
		name        string
		pattern     string
		wantMatches int
		wantSuccess bool
	}{
		{
			name:        "literal search",
			pattern:     "test",
			wantMatches: 2,
			wantSuccess: true,
		},
		{
			name:        "regex search",
			pattern:     "line [0-9]+",
			wantMatches: 3,
			wantSuccess: true,
		},
		{
			name:        "no matches",
			pattern:     "nonexistent",
			wantMatches: 0,
			wantSuccess: true,
		},
		{
			name:        "empty pattern",
			pattern:     "",
			wantSuccess: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := editor.handleSearch(testFile, tt.pattern)
			if err != nil {
				t.Fatalf("handleSearch() error = %v", err)
			}

			if response.Success != tt.wantSuccess {
				t.Errorf("handleSearch() success = %v, want %v", response.Success, tt.wantSuccess)
				return
			}

			if tt.wantSuccess && len(response.Matches) != tt.wantMatches {
				t.Errorf("handleSearch() matches = %d, want %d", len(response.Matches), tt.wantMatches)
			}
		})
	}
}

func TestEditor_HandleReplaceLine(t *testing.T) {
	workspace, cleanup := createTempWorkspace(t)
	defer cleanup()

	editor, err := NewEditor(workspace)
	if err != nil {
		t.Fatalf("NewEditor() error = %v", err)
	}

	// Create test file
	originalContent := "line 1\nline 2\nline 3\n"
	testFile := createTestFile(t, workspace, "test.txt", originalContent)

	// Replace line 2
	response, err := editor.handleReplaceLine(testFile, intPtr(2), "REPLACED LINE 2")
	if err != nil {
		t.Fatalf("handleReplaceLine() error = %v", err)
	}

	if !response.Success {
		t.Fatalf("handleReplaceLine() success = false, error = %v", response.Error)
	}

	// Verify the file was modified
	newContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("reading modified file: %v", err)
	}

	expected := "line 1\nREPLACED LINE 2\nline 3\n"
	if string(newContent) != expected {
		t.Errorf("file content = %q, want %q", string(newContent), expected)
	}
}

func TestEditor_HandleReplaceRange(t *testing.T) {
	workspace, cleanup := createTempWorkspace(t)
	defer cleanup()

	editor, err := NewEditor(workspace)
	if err != nil {
		t.Fatalf("NewEditor() error = %v", err)
	}

	// Create test file
	originalContent := "line 1\nline 2\nline 3\nline 4\n"
	testFile := createTestFile(t, workspace, "test.txt", originalContent)

	// Replace lines 2-3
	response, err := editor.handleReplaceRange(testFile, intPtr(2), intPtr(3), "REPLACED LINES 2-3")
	if err != nil {
		t.Fatalf("handleReplaceRange() error = %v", err)
	}

	if !response.Success {
		t.Fatalf("handleReplaceRange() success = false, error = %v", response.Error)
	}

	// Verify the file was modified
	newContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("reading modified file: %v", err)
	}

	expected := "line 1\nREPLACED LINES 2-3\nline 4\n"
	if string(newContent) != expected {
		t.Errorf("file content = %q, want %q", string(newContent), expected)
	}
}

func TestEditor_HandleInsertAfter(t *testing.T) {
	workspace, cleanup := createTempWorkspace(t)
	defer cleanup()

	editor, err := NewEditor(workspace)
	if err != nil {
		t.Fatalf("NewEditor() error = %v", err)
	}

	// Create test file
	originalContent := "line 1\nline 2\nline 3\n"
	testFile := createTestFile(t, workspace, "test.txt", originalContent)

	// Insert after line 2
	response, err := editor.handleInsertAfter(testFile, intPtr(2), "INSERTED LINE")
	if err != nil {
		t.Fatalf("handleInsertAfter() error = %v", err)
	}

	if !response.Success {
		t.Fatalf("handleInsertAfter() success = false, error = %v", response.Error)
	}

	// Verify the file was modified
	newContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("reading modified file: %v", err)
	}

	expected := "line 1\nline 2\nINSERTED LINE\nline 3\n"
	if string(newContent) != expected {
		t.Errorf("file content = %q, want %q", string(newContent), expected)
	}
}

func TestEditor_HandleDeleteRange(t *testing.T) {
	workspace, cleanup := createTempWorkspace(t)
	defer cleanup()

	editor, err := NewEditor(workspace)
	if err != nil {
		t.Fatalf("NewEditor() error = %v", err)
	}

	// Create test file
	originalContent := "line 1\nline 2\nline 3\nline 4\n"
	testFile := createTestFile(t, workspace, "test.txt", originalContent)

	// Delete lines 2-3
	response, err := editor.handleDeleteRange(testFile, intPtr(2), intPtr(3))
	if err != nil {
		t.Fatalf("handleDeleteRange() error = %v", err)
	}

	if !response.Success {
		t.Fatalf("handleDeleteRange() success = false, error = %v", response.Error)
	}

	// Verify the file was modified
	newContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("reading modified file: %v", err)
	}

	expected := "line 1\nline 4\n"
	if string(newContent) != expected {
		t.Errorf("file content = %q, want %q", string(newContent), expected)
	}
}

func TestEditor_ConcurrentEdits(t *testing.T) {
	workspace, cleanup := createTempWorkspace(t)
	defer cleanup()

	editor, err := NewEditor(workspace)
	if err != nil {
		t.Fatalf("NewEditor() error = %v", err)
	}

	// Create test file
	originalContent := "line 1\nline 2\nline 3\n"
	testFile := createTestFile(t, workspace, "test.txt", originalContent)

	ctx := context.Background()

	// Try concurrent edits - one should succeed, the other should fail with lock timeout
	done := make(chan error, 2)

	go func() {
		req := EditRequest{
			Operation:   OperationReplaceLine,
			Path:        testFile,
			Start:       intPtr(1),
			Replacement: "EDIT 1",
		}
		_, err := editor.Edit(ctx, req)
		done <- err
	}()

	go func() {
		req := EditRequest{
			Operation:   OperationReplaceLine,
			Path:        testFile,
			Start:       intPtr(2),
			Replacement: "EDIT 2",
		}
		_, err := editor.Edit(ctx, req)
		done <- err
	}()

	// Collect results
	var errs []error
	for i := 0; i < 2; i++ {
		errs = append(errs, <-done)
	}

	// At least one should succeed
	successCount := 0
	for _, err := range errs {
		if err == nil {
			successCount++
		}
	}

	if successCount == 0 {
		t.Errorf("All concurrent edits failed: %v", errs)
	}
}

func TestEditor_FilePermissions(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("Skipping permission test when running as root")
	}

	workspace, cleanup := createTempWorkspace(t)
	defer cleanup()

	editor, err := NewEditor(workspace)
	if err != nil {
		t.Fatalf("NewEditor() error = %v", err)
	}

	// Create test file with no read permissions
	testFile := createTestFile(t, workspace, "test.txt", "test content")
	if err := os.Chmod(testFile, 0000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	defer os.Chmod(testFile, 0644) // Restore for cleanup

	response, err := editor.handleRead(testFile)
	if err != nil {
		t.Fatalf("handleRead() error = %v", err)
	}

	if response.Success {
		t.Errorf("handleRead() should have failed on unreadable file")
	}

	if !strings.Contains(response.Error, "permission denied") {
		t.Errorf("handleRead() error = %q, should contain 'permission denied'", response.Error)
	}
}

func TestEditor_LargeFile(t *testing.T) {
	workspace, cleanup := createTempWorkspace(t)
	defer cleanup()

	editor, err := NewEditor(workspace)
	if err != nil {
		t.Fatalf("NewEditor() error = %v", err)
	}

	// Create a large test file (1000 lines)
	var content strings.Builder
	for i := 1; i <= 1000; i++ {
		content.WriteString(fmt.Sprintf("line %d\n", i))
	}

	testFile := createTestFile(t, workspace, "large.txt", content.String())

	// Test reading a range from the large file
	response, err := editor.handleReadRange(testFile, intPtr(500), intPtr(502))
	if err != nil {
		t.Fatalf("handleReadRange() error = %v", err)
	}

	if !response.Success {
		t.Fatalf("handleReadRange() success = false, error = %v", response.Error)
	}

	expected := "line 500\nline 501\nline 502"
	if response.Content != expected {
		t.Errorf("handleReadRange() content = %q, want %q", response.Content, expected)
	}

	// Test replacing a line in the large file
	replaceResponse, err := editor.handleReplaceLine(testFile, intPtr(500), "REPLACED LINE 500")
	if err != nil {
		t.Fatalf("handleReplaceLine() error = %v", err)
	}

	if !replaceResponse.Success {
		t.Fatalf("handleReplaceLine() success = false, error = %v", replaceResponse.Error)
	}

	// Verify the change
	verifyResponse, err := editor.handleReadRange(testFile, intPtr(499), intPtr(501))
	if err != nil {
		t.Fatalf("handleReadRange() verification error = %v", err)
	}

	expectedVerify := "line 499\nREPLACED LINE 500\nline 501"
	if verifyResponse.Content != expectedVerify {
		t.Errorf("Verification content = %q, want %q", verifyResponse.Content, expectedVerify)
	}
}

// Helper function to create int pointer
func intPtr(i int) *int {
	return &i
}
