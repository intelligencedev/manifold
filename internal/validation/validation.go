// Package validation provides common validation functions for IDs and paths.
// This package has no dependencies on other internal packages to avoid import cycles.
package validation

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ErrInvalidProjectID indicates the project_id value is malformed or attempts path traversal.
var ErrInvalidProjectID = errors.New("invalid project_id")

// ErrInvalidSessionID indicates the session_id value is malformed or attempts path traversal.
var ErrInvalidSessionID = errors.New("invalid session_id")

// ProjectID checks if a project ID is safe for use in filesystem paths.
// Returns cleaned project ID and error if validation fails.
func ProjectID(projectID string) (string, error) {
	if projectID == "" {
		return "", nil
	}

	// IDs must be a single path segment.
	if projectID == "." || projectID == ".." {
		return "", ErrInvalidProjectID
	}
	if strings.ContainsAny(projectID, `/\\`) {
		return "", ErrInvalidProjectID
	}

	cleanPID := filepath.Clean(projectID)
	if cleanPID != projectID ||
		strings.HasPrefix(cleanPID, "..") ||
		strings.Contains(cleanPID, string(os.PathSeparator)+"..") ||
		filepath.IsAbs(cleanPID) {
		return "", ErrInvalidProjectID
	}

	return cleanPID, nil
}

// SessionID checks if a session ID is safe for use as a single filesystem path segment.
func SessionID(sessionID string) (string, error) {
	if sessionID == "" {
		return "", nil
	}

	if sessionID == "." || sessionID == ".." {
		return "", ErrInvalidSessionID
	}
	if strings.ContainsAny(sessionID, `/\\`) {
		return "", ErrInvalidSessionID
	}

	cleanSID := filepath.Clean(sessionID)
	if cleanSID != sessionID ||
		strings.HasPrefix(cleanSID, "..") ||
		strings.Contains(cleanSID, string(os.PathSeparator)+"..") ||
		filepath.IsAbs(cleanSID) {
		return "", ErrInvalidSessionID
	}

	return cleanSID, nil
}
