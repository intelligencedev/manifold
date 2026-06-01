package agentd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"manifold/internal/agent/memory/belief"
	"manifold/internal/persistence"
	transitdomain "manifold/internal/transit"
)

func (a *app) resolveChatObjectiveID(ctx context.Context, owner int64, req chatRunRequest) string {
	projectID := belief.NormalizeProjectID(req.ProjectID)
	sessionID := strings.TrimSpace(req.SessionID)
	objectiveID := strings.TrimSpace(req.ObjectiveID)
	mappingKey := belief.ObjectiveSessionKey(projectID, sessionID)

	if objectiveID == "" && a.transitService != nil {
		records, err := a.transitService.GetMemory(ctx, owner, []string{mappingKey})
		if err == nil && len(records) > 0 {
			objectiveID = strings.TrimSpace(records[0].Value)
		} else if err != nil {
			log.Warn().Err(err).Str("key", mappingKey).Msg("belief_objective_mapping_lookup_failed")
		}
	}
	if objectiveID == "" {
		objectiveID = uuid.NewSHA1(uuid.NameSpaceOID, fmt.Appendf(nil, "%d/%s/%s", owner, projectID, sessionID)).String()
	}

	if a.cfg != nil && a.cfg.BeliefMemory.Enabled {
		a.persistChatObjectiveManifest(ctx, owner, projectID, sessionID, objectiveID, mappingKey)
	}
	return objectiveID
}

func (a *app) persistChatObjectiveManifest(ctx context.Context, owner int64, projectID, sessionID, objectiveID, mappingKey string) {
	if a.transitService == nil {
		return
	}
	manifestKey := belief.ObjectiveManifestKey(projectID, objectiveID)
	existing, err := a.transitService.GetMemory(ctx, owner, []string{mappingKey, manifestKey})
	if err != nil {
		log.Warn().Err(err).Msg("belief_objective_manifest_lookup_failed")
		return
	}
	seen := make(map[string]bool, len(existing))
	for _, record := range existing {
		seen[record.KeyName] = true
	}

	embed := false
	items := make([]transitdomain.CreateMemoryItem, 0, 2)
	if !seen[mappingKey] {
		items = append(items, transitdomain.CreateMemoryItem{
			KeyName:     mappingKey,
			Description: "Session to shared belief objective mapping",
			Value:       objectiveID,
			Embed:       &embed,
		})
	}
	if !seen[manifestKey] {
		now := time.Now().UTC()
		manifest := belief.ObjectiveManifest{
			ID:                   objectiveID,
			TenantID:             owner,
			UserID:               owner,
			ProjectID:            projectID,
			Title:                "Chat objective " + objectiveID,
			Status:               "active",
			CreatedFromSessionID: sessionID,
			Metadata:             map[string]any{"source": "chat_run"},
			CreatedAt:            now,
			UpdatedAt:            now,
		}
		value, err := json.Marshal(manifest)
		if err != nil {
			log.Warn().Err(err).Msg("belief_objective_manifest_encode_failed")
			return
		}
		items = append(items, transitdomain.CreateMemoryItem{
			KeyName:     manifestKey,
			Description: "Shared belief objective manifest",
			Value:       string(value),
			Embed:       &embed,
		})
	}
	if len(items) == 0 {
		return
	}
	if _, err := a.transitService.CreateMemory(ctx, owner, owner, items); err != nil && !errors.Is(err, persistence.ErrRevisionConflict) {
		log.Warn().Err(err).Msg("belief_objective_manifest_create_failed")
	}
}
