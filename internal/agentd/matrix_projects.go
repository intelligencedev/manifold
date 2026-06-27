package agentd

import (
	"context"
	"fmt"
	"strings"

	"manifold/internal/projects"
)

const matrixRoomProjectPrefix = "Matrix Room "

func (a *app) ensureMatrixRoomProject(ctx context.Context, roomID string) (string, error) {
	trimmedRoomID := strings.TrimSpace(roomID)
	if trimmedRoomID == "" {
		return "", nil
	}
	if a.projectsService == nil {
		return "", fmt.Errorf("projects service unavailable")
	}

	projectName := matrixRoomProjectName(trimmedRoomID)
	matrixProjects, err := a.projectsService.ListProjectsByKindWithUsage(
		ctx,
		systemUserID,
		projects.ProjectKindMatrix,
		false,
	)
	if err != nil {
		return "", fmt.Errorf("list projects: %w", err)
	}
	for _, project := range matrixProjects {
		if project.Name == projectName {
			return project.ID, nil
		}
	}

	project, err := a.projectsService.CreateProjectKind(ctx, systemUserID, projectName, projects.ProjectKindMatrix)
	if err != nil {
		return "", fmt.Errorf("create project: %w", err)
	}
	return project.ID, nil
}

func matrixRoomProjectName(roomID string) string {
	trimmedRoomID := strings.TrimSpace(roomID)
	if trimmedRoomID == "" {
		return strings.TrimSpace(matrixRoomProjectPrefix)
	}
	return matrixRoomProjectPrefix + trimmedRoomID
}
