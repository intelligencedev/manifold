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
	"manifold/internal/transit"
)

const defaultTransitCaptureKeyLimit = 32

// TransitConnector captures explicitly referenced Transit memory keys.
type TransitConnector struct {
	Store   transit.Store
	MaxKeys int
}

// Kind returns the artifact kind captured by this connector.
func (TransitConnector) Kind() artifact.ArtifactKind { return artifact.ArtifactTransitKey }

// Capture captures explicit Transit keys. Hints: keys, key, transitKeys.
func (c TransitConnector) Capture(ctx context.Context, req artifact.CaptureRequest) ([]artifact.Artifact, error) {
	if c.Store == nil {
		return nil, artifact.ErrConnectorUnavailable
	}
	keys := transitKeysFromHints(req.Hints)
	if len(keys) == 0 {
		return nil, artifact.ErrConnectorUnavailable
	}
	maxKeys := c.MaxKeys
	if maxKeys <= 0 {
		maxKeys = defaultTransitCaptureKeyLimit
	}
	if len(keys) > maxKeys {
		keys = keys[:maxKeys]
	}
	records, err := c.Store.Get(ctx, req.TenantID, keys)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	out := make([]artifact.Artifact, 0, len(records))
	for _, record := range records {
		excerpt := transitExcerpt(record)
		content := record.KeyName + "\n" + record.Description + "\n" + record.Value + "\n" + strconv.FormatInt(record.Version, 10)
		hash := sha256.Sum256([]byte(content))
		authoredAt := record.UpdatedAt
		if authoredAt.IsZero() {
			authoredAt = record.CreatedAt
		}
		if authoredAt.IsZero() {
			authoredAt = now
		}
		out = append(out, artifact.Artifact{
			TenantID:    req.TenantID,
			Kind:        artifact.ArtifactTransitKey,
			ExternalID:  fmt.Sprintf("%s@v%d", record.KeyName, record.Version),
			URI:         transitKeyURI(record.KeyName, record.Version),
			Title:       record.KeyName,
			Excerpt:     truncateBytes(excerpt, maxArtifactExcerptBytes),
			ContentHash: fmt.Sprintf("%x", hash),
			AuthoredBy:  strconv.FormatInt(record.UpdatedBy, 10),
			AuthoredAt:  authoredAt,
			CapturedAt:  now,
			Metadata: map[string]any{
				"keyName":     record.KeyName,
				"version":     record.Version,
				"base64":      record.Base64,
				"embed":       record.Embed,
				"embedSource": record.EmbedSource,
				"createdBy":   record.CreatedBy,
				"updatedBy":   record.UpdatedBy,
				"scopeId":     strings.TrimSpace(req.ScopeID),
				"episodeId":   strings.TrimSpace(req.EpisodeID),
			},
		})
	}
	return out, nil
}

func transitKeysFromHints(hints map[string]string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, name := range []string{"keys", "key", "transitKeys"} {
		raw := strings.TrimSpace(hints[name])
		if raw == "" {
			continue
		}
		for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
			return r == ',' || r == '\n' || r == '\t' || r == ';'
		}) {
			key := strings.TrimSpace(part)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, key)
		}
	}
	return out
}

func transitExcerpt(record transit.Record) string {
	description := strings.TrimSpace(record.Description)
	if record.Base64 {
		return strings.TrimSpace(description + "\n\n[base64 value omitted]")
	}
	return strings.TrimSpace(description + "\n\n" + record.Value)
}

func transitKeyURI(key string, version int64) string {
	return "transit://" + url.PathEscape(key) + "?version=" + strconv.FormatInt(version, 10)
}
