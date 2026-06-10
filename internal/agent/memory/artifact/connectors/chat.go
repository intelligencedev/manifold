package connectors

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"manifold/internal/agent/memory/artifact"
	"manifold/internal/persistence"
)

const defaultChatCaptureLimit = 100

// ChatConnector captures chat messages for the active session.
type ChatConnector struct {
	Store persistence.ChatStore
	Limit int
}

// Kind returns the artifact kind captured by this connector.
func (ChatConnector) Kind() artifact.ArtifactKind { return artifact.ArtifactChatMessage }

// Capture captures messages for the requested chat session. Hints: sessionID, limit, since, until.
func (c ChatConnector) Capture(ctx context.Context, req artifact.CaptureRequest) ([]artifact.Artifact, error) {
	if c.Store == nil {
		return nil, artifact.ErrConnectorUnavailable
	}
	sessionID := strings.TrimSpace(req.Hints["sessionID"])
	if sessionID == "" {
		return nil, artifact.ErrConnectorUnavailable
	}
	limit := c.Limit
	if limit <= 0 {
		limit = defaultChatCaptureLimit
	}
	if hinted := strings.TrimSpace(req.Hints["limit"]); hinted != "" {
		parsed, err := strconv.Atoi(hinted)
		if err != nil {
			return nil, err
		}
		if parsed > 0 {
			limit = parsed
		}
	}
	since, err := parseOptionalRFC3339(req.Hints["since"])
	if err != nil {
		return nil, err
	}
	until, err := parseOptionalRFC3339(req.Hints["until"])
	if err != nil {
		return nil, err
	}
	userID := req.TenantID
	messages, err := c.Store.ListMessages(ctx, &userID, sessionID, limit)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	out := make([]artifact.Artifact, 0, len(messages))
	for i, msg := range messages {
		if !since.IsZero() && msg.CreatedAt.Before(since) {
			continue
		}
		if !until.IsZero() && msg.CreatedAt.After(until) {
			continue
		}
		excerpt := strings.TrimSpace(msg.Content)
		if excerpt == "" {
			continue
		}
		messageID := strings.TrimSpace(msg.ID)
		if messageID == "" {
			messageID = fmt.Sprintf("%d", i)
		}
		content := strings.TrimSpace(msg.Role) + "\n" + excerpt
		hash := sha256.Sum256([]byte(content))
		title := strings.TrimSpace(msg.Title)
		if title == "" {
			title = strings.TrimSpace(msg.Role)
			if title == "" {
				title = "chat"
			}
			title += " message"
		}
		authoredAt := msg.CreatedAt
		if authoredAt.IsZero() {
			authoredAt = now
		}
		out = append(out, artifact.Artifact{
			TenantID:    req.TenantID,
			Kind:        artifact.ArtifactChatMessage,
			ExternalID:  sessionID + ":" + messageID,
			URI:         chatMessageURI(sessionID, messageID),
			Title:       title,
			Excerpt:     truncateBytes(excerpt, maxArtifactExcerptBytes),
			ContentHash: fmt.Sprintf("%x", hash),
			AuthoredBy:  strings.TrimSpace(msg.Role),
			AuthoredAt:  authoredAt,
			CapturedAt:  now,
			Metadata: map[string]any{
				"sessionId": sessionID,
				"messageId": messageID,
				"role":      strings.TrimSpace(msg.Role),
				"scopeId":   strings.TrimSpace(req.ScopeID),
				"episodeId": strings.TrimSpace(req.EpisodeID),
				"toolId":    strings.TrimSpace(msg.ToolID),
			},
		})
	}
	return out, nil
}

func chatMessageURI(sessionID, messageID string) string {
	return "chat://" + url.PathEscape(sessionID) + "#" + url.QueryEscape(messageID)
}

func parseOptionalRFC3339(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, value)
}
