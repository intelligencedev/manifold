package agentd

import (
	"context"
	"fmt"
	"strings"
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
	projects, err := a.projectsService.ListProjects(ctx, systemUserID)
	if err != nil {
		return "", fmt.Errorf("list projects: %w", err)
	}
	for _, project := range projects {
		if project.Name == projectName {
			return project.ID, nil
		}
	}

	project, err := a.projectsService.CreateProject(ctx, systemUserID, projectName)
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
