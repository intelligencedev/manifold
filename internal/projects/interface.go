package projects

import (
	"context"
	"io"
)

// ProjectService defines the interface for project storage operations.
// Implementations include filesystem-backed (Service) and S3-backed (S3Service).
type ProjectService interface {
	// CreateProject creates a new project for the given user.
	CreateProject(ctx context.Context, userID int64, name string) (Project, error)

	// DeleteProject removes a project and all its files.
	DeleteProject(ctx context.Context, userID int64, projectID string) error

	// ListProjects returns all projects for a user.
	ListProjects(ctx context.Context, userID int64) ([]Project, error)

	// ListTree lists entries directly under path within a project.
	ListTree(ctx context.Context, userID int64, projectID, path string) ([]FileEntry, error)

	// UploadFile writes a file into a project at the given path.
	UploadFile(ctx context.Context, userID int64, projectID, path, name string, r io.Reader) error

	// DeleteFile removes a file or directory from a project.
	DeleteFile(ctx context.Context, userID int64, projectID, path string) error

	// MovePath relocates a file or directory within a project.
	MovePath(ctx context.Context, userID int64, projectID, from, to string) error

	// CreateDir creates a directory within a project.
	CreateDir(ctx context.Context, userID int64, projectID, path string) error

	// ReadFile opens a file for reading.
	ReadFile(ctx context.Context, userID int64, projectID, path string) (io.ReadCloser, error)

	// EnableEncryption toggles at-rest encryption for project files.
	EnableEncryption(enable bool) error
}

// Ensure Service implements ProjectService.
var _ ProjectService = (*Service)(nil)
