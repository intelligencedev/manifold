package belief

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"time"
)

const DefaultProjectID = "default"

// ObjectiveManifest is the Transit-backed MVP representation of an objective.
type ObjectiveManifest struct {
	ID                   string         `json:"id"`
	TenantID             int64          `json:"tenantId"`
	UserID               int64          `json:"userId"`
	ProjectID            string         `json:"projectId"`
	Title                string         `json:"title"`
	Status               string         `json:"status"`
	CreatedFromSessionID string         `json:"createdFromSessionId"`
	Metadata             map[string]any `json:"metadata,omitempty"`
	CreatedAt            time.Time      `json:"createdAt"`
	UpdatedAt            time.Time      `json:"updatedAt"`
}

// ObjectiveManifestKey returns the reserved Transit key for an objective manifest.
func ObjectiveManifestKey(projectID, objectiveID string) string {
	return "objective/project/" + NormalizeProjectID(projectID) + "/" + strings.TrimSpace(objectiveID) + "/manifest"
}

// ObjectiveSessionKey returns the reserved Transit key that maps a session to
// the objective it belongs to. Session IDs are hashed so arbitrary UI session
// names remain valid Transit keys.
func ObjectiveSessionKey(projectID, sessionID string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(sessionID)))
	return fmt.Sprintf("objective/project/%s/session/%x/objective", NormalizeProjectID(projectID), digest)
}

func ObjectiveActivePlanKey(projectID, objectiveID string) string {
	return "objective/project/" + NormalizeProjectID(projectID) + "/" + strings.TrimSpace(objectiveID) + "/working/active_plan"
}

func ObjectiveHandoffKey(projectID, objectiveID string) string {
	return "objective/project/" + NormalizeProjectID(projectID) + "/" + strings.TrimSpace(objectiveID) + "/working/handoff"
}

func ObjectiveOpenQuestionsKey(projectID, objectiveID string) string {
	return "objective/project/" + NormalizeProjectID(projectID) + "/" + strings.TrimSpace(objectiveID) + "/working/open_questions"
}

func NormalizeProjectID(projectID string) string {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return DefaultProjectID
	}
	return projectID
}
