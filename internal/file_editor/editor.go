package file_editor

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gofrs/flock"
)

// Editor provides high-precision, atomic file editing operations
type Editor struct {
	workspaceRoot string
}

// NewEditor creates a new file editor with the specified workspace root
func NewEditor(workspaceRoot string) (*Editor, error) {
	absRoot, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("resolving workspace root: %w", err)
	}

	return &Editor{
		workspaceRoot: absRoot,
	}, nil
}

// Edit performs the specified file editing operation
func (e *Editor) Edit(ctx context.Context, req EditRequest) (EditResponse, error) {
	// Validate and resolve path
	absPath, err := e.validatePath(req.Path)
	if err != nil {
		return EditResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	// Acquire file lock for write operations
	var lock *flock.Flock
	if e.isWriteOperation(req.Operation) {
		lock = flock.New(absPath + ".lock")
		lockCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		if ok, err := lock.TryLockContext(lockCtx, 100*time.Millisecond); err != nil || !ok {
			return EditResponse{
				Success: false,
				Error:   fmt.Sprintf("%v: %v", ErrLockTimeout, err),
			}, nil
		}
		defer lock.Unlock()
	}

	// Dispatch to appropriate operation handler
	switch req.Operation {
	case OperationRead:
		return e.handleRead(absPath)
	case OperationReadRange:
		return e.handleReadRange(absPath, req.Start, req.End)
	case OperationSearch:
		return e.handleSearch(absPath, req.Pattern)
	case OperationReplaceLine:
		return e.handleReplaceLine(absPath, req.Start, req.Replacement)
	case OperationReplaceRange:
		return e.handleReplaceRange(absPath, req.Start, req.End, req.Replacement)
	case OperationInsertAfter:
		return e.handleInsertAfter(absPath, req.Start, req.Replacement)
	case OperationDeleteRange:
		return e.handleDeleteRange(absPath, req.Start, req.End)
	case OperationPreviewPatch:
		return e.handlePreviewPatch(absPath, req.Patch)
	case OperationApplyPatch:
		return e.handleApplyPatch(absPath, req.Patch)
	default:
		return EditResponse{
			Success: false,
			Error:   fmt.Sprintf("unsupported operation: %s", req.Operation),
		}, nil
	}
}

// validatePath ensures the path is within the workspace and returns absolute path
func (e *Editor) validatePath(path string) (string, error) {
	// If path is relative, make it relative to workspace root
	if !filepath.IsAbs(path) {
		path = filepath.Join(e.workspaceRoot, path)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolving path: %w", err)
	}

	if !strings.HasPrefix(absPath, e.workspaceRoot) {
		return "", fmt.Errorf("%w: %s", ErrPathOutsideRoot, path)
	}

	return absPath, nil
}

// isWriteOperation returns true if the operation modifies the file
func (e *Editor) isWriteOperation(op Operation) bool {
	switch op {
	case OperationReplaceLine, OperationReplaceRange,
		OperationInsertAfter, OperationDeleteRange,
		OperationApplyPatch:
		return true
	default:
		return false
	}
}

// withScanner provides a streaming file reader that processes line by line
func (e *Editor) withScanner(path string, fn func(i int, line []byte) error) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrFileNotFound
		}
		if os.IsPermission(err) {
			return ErrPermissionDenied
		}
		return fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if err := fn(lineNum, scanner.Bytes()); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}

	return scanner.Err()
}

// validateRange checks if the line range is valid
func (e *Editor) validateRange(start, end *int, maxLines int) error {
	if start == nil {
		return fmt.Errorf("%w: start line is required", ErrInvalidRange)
	}

	if *start < 1 {
		return fmt.Errorf("%w: start line must be >= 1, got %d", ErrInvalidRange, *start)
	}

	if end != nil {
		if *end < *start {
			return fmt.Errorf("%w: end line (%d) must be >= start line (%d)", ErrInvalidRange, *end, *start)
		}
		if *end > maxLines {
			return fmt.Errorf("%w: end line (%d) exceeds file length (%d)", ErrInvalidRange, *end, maxLines)
		}
	} else if *start > maxLines {
		return fmt.Errorf("%w: start line (%d) exceeds file length (%d)", ErrInvalidRange, *start, maxLines)
	}

	return nil
}

// countLines efficiently counts the number of lines in a file
func (e *Editor) countLines(path string) (int, error) {
	count := 0
	err := e.withScanner(path, func(i int, line []byte) error {
		count = i
		return nil
	})
	return count, err
}

// handleRead reads the entire file content
func (e *Editor) handleRead(path string) (EditResponse, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return EditResponse{Success: false, Error: ErrFileNotFound.Error()}, nil
		}
		if os.IsPermission(err) {
			return EditResponse{Success: false, Error: ErrPermissionDenied.Error()}, nil
		}
		return EditResponse{Success: false, Error: fmt.Sprintf("reading file: %v", err)}, nil
	}

	return EditResponse{
		Success: true,
		Content: string(content),
		Message: fmt.Sprintf("read %d bytes from %s", len(content), filepath.Base(path)),
	}, nil
}

// handleReadRange reads a specific range of lines
func (e *Editor) handleReadRange(path string, start, end *int) (EditResponse, error) {
	if start == nil {
		return EditResponse{Success: false, Error: "start line is required"}, nil
	}

	// Count total lines first
	totalLines, err := e.countLines(path)
	if err != nil {
		return EditResponse{Success: false, Error: err.Error()}, nil
	}

	// Validate range
	if err := e.validateRange(start, end, totalLines); err != nil {
		return EditResponse{Success: false, Error: err.Error()}, nil
	}

	endLine := totalLines
	if end != nil {
		endLine = *end
	}

	var lines []string
	err = e.withScanner(path, func(i int, line []byte) error {
		if i >= *start && i <= endLine {
			lines = append(lines, string(line))
		}
		if i > endLine {
			return io.EOF // Early termination
		}
		return nil
	})

	if err != nil {
		return EditResponse{Success: false, Error: err.Error()}, nil
	}

	content := strings.Join(lines, "\n")
	return EditResponse{
		Success: true,
		Content: content,
		Message: fmt.Sprintf("read lines %d-%d from %s (%d lines)", *start, endLine, filepath.Base(path), len(lines)),
	}, nil
}

// handleSearch searches for a pattern in the file
func (e *Editor) handleSearch(path, pattern string) (EditResponse, error) {
	if pattern == "" {
		return EditResponse{Success: false, Error: "search pattern is required"}, nil
	}

	// Compile regex pattern
	var regex *regexp.Regexp
	var err error

	// Check if pattern contains newlines - use multiline mode
	if strings.Contains(pattern, "\n") {
		regex, err = regexp.Compile("(?s)" + pattern)
	} else {
		regex, err = regexp.Compile(pattern)
	}

	if err != nil {
		// Try literal search if regex compilation fails
		return e.handleLiteralSearch(path, pattern)
	}

	var matches []Match
	const maxMatches = 500

	err = e.withScanner(path, func(i int, line []byte) error {
		lineStr := string(line)
		if regex.MatchString(lineStr) {
			matches = append(matches, Match{
				LineNumber: i,
				Line:       lineStr,
			})

			// Limit matches to prevent excessive output
			if len(matches) >= maxMatches {
				return io.EOF
			}
		}
		return nil
	})

	if err != nil && err != io.EOF {
		return EditResponse{Success: false, Error: err.Error()}, nil
	}

	message := fmt.Sprintf("found %d matches for pattern in %s", len(matches), filepath.Base(path))
	if len(matches) >= maxMatches {
		message += " (truncated)"
	}

	return EditResponse{
		Success: true,
		Matches: matches,
		Message: message,
	}, nil
}

// handleLiteralSearch performs literal string search as fallback
func (e *Editor) handleLiteralSearch(path, pattern string) (EditResponse, error) {
	var matches []Match
	const maxMatches = 500

	err := e.withScanner(path, func(i int, line []byte) error {
		lineStr := string(line)
		if strings.Contains(lineStr, pattern) {
			matches = append(matches, Match{
				LineNumber: i,
				Line:       lineStr,
			})

			if len(matches) >= maxMatches {
				return io.EOF
			}
		}
		return nil
	})

	if err != nil && err != io.EOF {
		return EditResponse{Success: false, Error: err.Error()}, nil
	}

	message := fmt.Sprintf("found %d literal matches in %s", len(matches), filepath.Base(path))
	if len(matches) >= maxMatches {
		message += " (truncated)"
	}

	return EditResponse{
		Success: true,
		Matches: matches,
		Message: message,
	}, nil
}
