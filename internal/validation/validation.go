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
	return validatePathSegment(projectID, ErrInvalidProjectID)
}

// SessionID checks if a session ID is safe for use as a single filesystem path segment.
func SessionID(sessionID string) (string, error) {
	return validatePathSegment(sessionID, ErrInvalidSessionID)
}

func validatePathSegment(value string, invalidErr error) (string, error) {
	if value == "" {
		return "", nil
	}

	// IDs must be a single path segment.
	if value == "." || value == ".." {
		return "", invalidErr
	}
	if strings.ContainsAny(value, `/\\`) {
		return "", invalidErr
	}

	clean := filepath.Clean(value)
	if clean != value ||
		strings.HasPrefix(clean, "..") ||
		strings.Contains(clean, string(os.PathSeparator)+"..") ||
		filepath.IsAbs(clean) {
		return "", invalidErr
	}

	return clean, nil
}
