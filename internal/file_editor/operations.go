package file_editor

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// editRange is the core atomic editing primitive that all range operations use
func (e *Editor) editRange(path string, start, end int, cb func(w io.Writer) error) error {
	// Create temporary file in same directory for atomic rename
	tmpFile, err := os.CreateTemp(filepath.Dir(path), ".edit-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer tmpFile.Close()
	defer os.Remove(tmpPath) // Cleanup on error

	// First pass: copy lines before start
	err = e.withScanner(path, func(i int, line []byte) error {
		if i < start {
			_, err := tmpFile.Write(append(line, '\n'))
			return err
		}
		if i == start {
			return io.EOF // Signal to stop scanning
		}
		return nil
	})
	if err != nil && err != io.EOF {
		return fmt.Errorf("copying lines before range: %w", err)
	}

	// Let callback write replacement content
	if err := cb(tmpFile); err != nil {
		return fmt.Errorf("writing replacement content: %w", err)
	}

	// Second pass: copy lines after end
	err = e.withScanner(path, func(i int, line []byte) error {
		if i > end {
			_, err := tmpFile.Write(append(line, '\n'))
			return err
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("copying lines after range: %w", err)
	}

	// Sync to ensure data is written
	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("syncing temp file: %w", err)
	}

	// Close the temp file before rename
	tmpFile.Close()

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("atomic rename: %w", err)
	}

	return nil
}

// handleReplaceLine replaces a single line
func (e *Editor) handleReplaceLine(path string, start *int, replacement string) (EditResponse, error) {
	if start == nil {
		return EditResponse{Success: false, Error: "start line is required"}, nil
	}

	// Count total lines to validate range
	totalLines, err := e.countLines(path)
	if err != nil {
		return EditResponse{Success: false, Error: err.Error()}, nil
	}

	if err := e.validateRange(start, start, totalLines); err != nil {
		return EditResponse{Success: false, Error: err.Error()}, nil
	}

	// Replace the single line
	err = e.editRange(path, *start, *start, func(w io.Writer) error {
		_, err := w.Write([]byte(replacement + "\n"))
		return err
	})

	if err != nil {
		return EditResponse{Success: false, Error: err.Error()}, nil
	}

	return EditResponse{
		Success: true,
		Message: fmt.Sprintf("replaced line %d in %s", *start, filepath.Base(path)),
	}, nil
}

// handleReplaceRange replaces a range of lines
func (e *Editor) handleReplaceRange(path string, start, end *int, replacement string) (EditResponse, error) {
	if start == nil {
		return EditResponse{Success: false, Error: "start line is required"}, nil
	}

	// Count total lines to validate range
	totalLines, err := e.countLines(path)
	if err != nil {
		return EditResponse{Success: false, Error: err.Error()}, nil
	}

	endLine := *start
	if end != nil {
		endLine = *end
	}

	if err := e.validateRange(start, &endLine, totalLines); err != nil {
		return EditResponse{Success: false, Error: err.Error()}, nil
	}

	// Replace the range
	err = e.editRange(path, *start, endLine, func(w io.Writer) error {
		if replacement != "" {
			_, err := w.Write([]byte(replacement + "\n"))
			return err
		}
		return nil // Empty replacement = deletion
	})

	if err != nil {
		return EditResponse{Success: false, Error: err.Error()}, nil
	}

	return EditResponse{
		Success: true,
		Message: fmt.Sprintf("replaced lines %d-%d in %s", *start, endLine, filepath.Base(path)),
	}, nil
}

// handleInsertAfter inserts content after a specific line
func (e *Editor) handleInsertAfter(path string, start *int, content string) (EditResponse, error) {
	if start == nil {
		return EditResponse{Success: false, Error: "start line is required"}, nil
	}

	// Count total lines to validate line exists
	totalLines, err := e.countLines(path)
	if err != nil {
		return EditResponse{Success: false, Error: err.Error()}, nil
	}

	if *start < 0 || *start > totalLines {
		return EditResponse{
			Success: false,
			Error:   fmt.Sprintf("line %d does not exist (file has %d lines)", *start, totalLines),
		}, nil
	}

	// Insert after the specified line (range from start+1 to start, which is empty)
	err = e.editRange(path, *start+1, *start, func(w io.Writer) error {
		_, err := w.Write([]byte(content + "\n"))
		return err
	})

	if err != nil {
		return EditResponse{Success: false, Error: err.Error()}, nil
	}

	return EditResponse{
		Success: true,
		Message: fmt.Sprintf("inserted content after line %d in %s", *start, filepath.Base(path)),
	}, nil
}

// handleDeleteRange deletes a range of lines
func (e *Editor) handleDeleteRange(path string, start, end *int) (EditResponse, error) {
	if start == nil {
		return EditResponse{Success: false, Error: "start line is required"}, nil
	}

	// Count total lines to validate range
	totalLines, err := e.countLines(path)
	if err != nil {
		return EditResponse{Success: false, Error: err.Error()}, nil
	}

	endLine := *start
	if end != nil {
		endLine = *end
	}

	if err := e.validateRange(start, &endLine, totalLines); err != nil {
		return EditResponse{Success: false, Error: err.Error()}, nil
	}

	// Delete the range (empty callback = no content written)
	err = e.editRange(path, *start, endLine, func(w io.Writer) error {
		return nil // Write nothing = deletion
	})

	if err != nil {
		return EditResponse{Success: false, Error: err.Error()}, nil
	}

	return EditResponse{
		Success: true,
		Message: fmt.Sprintf("deleted lines %d-%d from %s", *start, endLine, filepath.Base(path)),
	}, nil
}

// handlePreviewPatch generates a preview of what a patch would do without applying it
func (e *Editor) handlePreviewPatch(path, patchContent string) (EditResponse, error) {
	if patchContent == "" {
		return EditResponse{Success: false, Error: "patch content is required"}, nil
	}

	// Read current file content
	originalContent, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return EditResponse{Success: false, Error: ErrFileNotFound.Error()}, nil
		}
		return EditResponse{Success: false, Error: fmt.Sprintf("reading file: %v", err)}, nil
	}

	// Use diffmatchpatch to parse and apply patch for preview
	dmp := diffmatchpatch.New()
	patches, err := dmp.PatchFromText(patchContent)
	if err != nil {
		return EditResponse{Success: false, Error: fmt.Sprintf("parsing patch: %v", err)}, nil
	}

	// Apply patch to get the result
	newContent, results := dmp.PatchApply(patches, string(originalContent))

	// Check if any patches failed to apply
	for i, success := range results {
		if !success {
			return EditResponse{Success: false, Error: fmt.Sprintf("patch %d failed to apply", i)}, nil
		}
	}

	// Generate a clean diff for preview
	diffs := dmp.DiffMain(string(originalContent), newContent, false)
	diffText := dmp.DiffPrettyText(diffs)

	return EditResponse{
		Success: true,
		Diff:    diffText,
		Message: fmt.Sprintf("patch preview for %s", filepath.Base(path)),
	}, nil
}

// handleApplyPatch applies a unified diff patch to the file
func (e *Editor) handleApplyPatch(path, patchContent string) (EditResponse, error) {
	if patchContent == "" {
		return EditResponse{Success: false, Error: "patch content is required"}, nil
	}

	// Create a temporary directory for safe patch application
	tmpDir, err := os.MkdirTemp("", "patch-apply-*")
	if err != nil {
		return EditResponse{Success: false, Error: fmt.Sprintf("creating temp dir: %v", err)}, nil
	}
	defer os.RemoveAll(tmpDir)

	// Copy file to temp directory
	tmpFile := filepath.Join(tmpDir, filepath.Base(path))
	if err := copyFile(path, tmpFile); err != nil {
		return EditResponse{Success: false, Error: fmt.Sprintf("copying file: %v", err)}, nil
	}

	// Write patch to temp file
	patchFile := filepath.Join(tmpDir, "changes.patch")
	if err := os.WriteFile(patchFile, []byte(patchContent), 0644); err != nil {
		return EditResponse{Success: false, Error: fmt.Sprintf("writing patch file: %v", err)}, nil
	}

	// Apply patch using the patch command for maximum compatibility
	cmd := exec.Command("patch", "--batch", "--silent", tmpFile, patchFile)
	cmd.Dir = tmpDir

	if output, err := cmd.CombinedOutput(); err != nil {
		return EditResponse{
			Success: false,
			Error:   fmt.Sprintf("patch failed: %v\nOutput: %s", err, string(output)),
		}, nil
	}

	// Copy the patched file back atomically
	if err := copyFile(tmpFile, path); err != nil {
		return EditResponse{Success: false, Error: fmt.Sprintf("copying patched file back: %v", err)}, nil
	}

	return EditResponse{
		Success: true,
		Message: fmt.Sprintf("patch applied successfully to %s", filepath.Base(path)),
	}, nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	return destFile.Sync()
}
