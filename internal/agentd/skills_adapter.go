package agentd

import (
	"context"
	"io"

	"manifold/internal/projects"
	"manifold/internal/skills"
)

// ProjectsServiceAdapter adapts projects.ProjectService to skills.SkillsProjectService.
// This adapter breaks the import cycle between skills and projects packages by
// providing a translation layer in the agentd package which can import both.
type ProjectsServiceAdapter struct {
	svc projects.ProjectService
}

// NewProjectsServiceAdapter creates a new adapter wrapping a projects.ProjectService.
func NewProjectsServiceAdapter(svc projects.ProjectService) *ProjectsServiceAdapter {
	return &ProjectsServiceAdapter{svc: svc}
}

// ListTreeForSkills implements skills.SkillsProjectService by delegating to the underlying
// projects.ProjectService.ListTree method and converting the result types.
func (a *ProjectsServiceAdapter) ListTreeForSkills(ctx context.Context, userID int64, projectID, path string) ([]skills.SkillsFileEntry, error) {
	entries, err := a.svc.ListTree(ctx, userID, projectID, path)
	if err != nil {
		return nil, err
	}

	// Convert projects.FileEntry to skills.SkillsFileEntry
	result := make([]skills.SkillsFileEntry, len(entries))
	for i, entry := range entries {
		result[i] = skills.SkillsFileEntry{
			Path: entry.Path,
			Name: entry.Name,
			Type: entry.Type,
			Size: entry.Size,
		}
	}
	return result, nil
}

// ReadFile implements skills.SkillsProjectService by delegating to the underlying
// projects.ProjectService.ReadFile method.
func (a *ProjectsServiceAdapter) ReadFile(ctx context.Context, userID int64, projectID, path string) (io.ReadCloser, error) {
	return a.svc.ReadFile(ctx, userID, projectID, path)
}
