package agents

import (
	"context"
	"encoding/json"
	"fmt"
	neturl "net/url"
	"strings"

	"github.com/google/uuid"

	"manifold/internal/llm"
	"manifold/internal/sandbox"
)

type delegatedRunScope struct {
	SessionID        string
	EphemeralSession bool
	ProjectID        string
	ObjectiveID      string
	RoomID           string
}

func resolveDelegatedRunScope(ctx context.Context, sessionID, projectID, objectiveID, roomID string) delegatedRunScope {
	resolvedSessionID, ephemeral := resolveDelegatedSessionID(ctx, sessionID)
	return delegatedRunScope{
		SessionID:        resolvedSessionID,
		EphemeralSession: ephemeral,
		ProjectID:        contextString(ctx, strings.TrimSpace(projectID), sandbox.ProjectIDFromContext),
		ObjectiveID:      contextString(ctx, strings.TrimSpace(objectiveID), sandbox.ObjectiveIDFromContext),
		RoomID:           contextString(ctx, strings.TrimSpace(roomID), sandbox.RoomIDFromContext),
	}
}

func resolveDelegatedSessionID(ctx context.Context, sessionID string) (string, bool) {
	sessionID = strings.TrimSpace(sessionID)
	fromContext := false
	if sessionID == "" {
		if ctxSID, ok := sandbox.SessionIDFromContext(ctx); ok {
			sessionID = ctxSID
			fromContext = true
		}
	}
	if sessionID == "" {
		return uuid.NewString(), true
	}
	if fromContext {
		return sessionID, false
	}
	if _, err := uuid.Parse(sessionID); err != nil {
		sessionID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(sessionID)).String()
	}
	return sessionID, false
}

func contextString(ctx context.Context, explicit string, lookup func(context.Context) (string, bool)) string {
	if explicit != "" {
		return explicit
	}
	value, ok := lookup(ctx)
	if !ok {
		return ""
	}
	return value
}

func delegatedRunBody(prompt string, history []llm.Message, scope delegatedRunScope) []byte {
	body := map[string]any{"prompt": prompt, "session_id": scope.SessionID}
	if scope.EphemeralSession {
		body["ephemeral_session"] = true
	}
	if len(history) > 0 {
		body["history"] = history
	}
	if scope.ProjectID != "" {
		body["project_id"] = scope.ProjectID
	}
	if scope.ObjectiveID != "" {
		body["objective_id"] = scope.ObjectiveID
	}
	if scope.RoomID != "" {
		body["room_id"] = scope.RoomID
	}
	data, _ := json.Marshal(body)
	return data
}

func agentRunURL(baseURL string, params map[string]string) string {
	u, _ := neturl.Parse(fmt.Sprintf("%s/agent/run", baseURL))
	q := u.Query()
	q.Set("stream", "0")
	for key, value := range params {
		if strings.TrimSpace(value) != "" {
			q.Set(key, value)
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func decodeRunPayload(data []byte) map[string]any {
	var payload map[string]any
	_ = json.Unmarshal(data, &payload)
	if raw, ok := payload["result"].(string); ok && strings.HasPrefix(strings.TrimSpace(raw), "{") {
		var decoded any
		if err := json.Unmarshal([]byte(raw), &decoded); err == nil {
			payload["result"] = decoded
		}
	}
	return payload
}
