package file_editor

import "errors"

// Operation represents the type of file editing operation to perform
type Operation string

const (
	OperationRead         Operation = "read"
	OperationReadRange    Operation = "read_range"
	OperationSearch       Operation = "search"
	OperationReplaceLine  Operation = "replace_line"
	OperationReplaceRange Operation = "replace_range"
	OperationInsertAfter  Operation = "insert_after"
	OperationDeleteRange  Operation = "delete_range"
	OperationApplyPatch   Operation = "apply_patch"
	OperationPreviewPatch Operation = "preview_patch"
)

// EditRequest represents a file editing request
type EditRequest struct {
	Operation   Operation `json:"operation"`
	Path        string    `json:"path"`
	Start       *int      `json:"start,omitempty"`       // 1-based line number
	End         *int      `json:"end,omitempty"`         // inclusive
	Pattern     string    `json:"pattern,omitempty"`     // regex or literal for search
	Replacement string    `json:"replacement,omitempty"` // replacement or insertion text
	Patch       string    `json:"patch,omitempty"`       // unified-diff content
}

// EditResponse represents the response from a file editing operation
type EditResponse struct {
	Success bool    `json:"success"`
	Message string  `json:"message"`
	Content string  `json:"content,omitempty"` // For read operations
	Matches []Match `json:"matches,omitempty"` // For search operations
	Diff    string  `json:"diff,omitempty"`    // For preview operations
	Error   string  `json:"error,omitempty"`
}

// Match represents a search result
type Match struct {
	LineNumber int    `json:"line_number"`
	Line       string `json:"line"`
	Context    string `json:"context,omitempty"`
}

// Common errors
var (
	ErrInvalidRange     = errors.New("invalid line range")
	ErrNoMatch          = errors.New("pattern not found")
	ErrPathOutsideRoot  = errors.New("path escapes workspace")
	ErrFileNotFound     = errors.New("file not found")
	ErrPermissionDenied = errors.New("permission denied")
	ErrLockTimeout      = errors.New("could not acquire file lock")
)
