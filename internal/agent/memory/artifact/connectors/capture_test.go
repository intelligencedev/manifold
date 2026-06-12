package connectors

import (
	"context"
	"testing"
	"time"

	"manifold/internal/agent/memory/artifact"
	"manifold/internal/persistence"
	"manifold/internal/transit"
)

func TestChatConnectorCapturesSessionMessages(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	store := &fakeChatStore{messages: []persistence.ChatMessage{
		{ID: "old", SessionID: "session-1", Role: "user", Content: "old message", CreatedAt: now.Add(-time.Hour)},
		{ID: "new", SessionID: "session-1", Role: "assistant", Content: "we chose postgres jsonb", CreatedAt: now},
	}}
	connector := ChatConnector{Store: store}
	items, err := connector.Capture(context.Background(), artifact.CaptureRequest{
		TenantID: 7,
		ScopeID:  "scope-1",
		Hints: map[string]string{
			"sessionID": "session-1",
			"since":     now.Add(-time.Minute).Format(time.RFC3339),
		},
	})
	if err != nil {
		t.Fatalf("Capture() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one captured message, got %d", len(items))
	}
	item := items[0]
	if item.Kind != artifact.ArtifactChatMessage || item.ExternalID != "session-1:new" || item.ContentHash == "" {
		t.Fatalf("unexpected chat artifact: %#v", item)
	}
	if item.Excerpt != "we chose postgres jsonb" || item.Metadata["sessionId"] != "session-1" {
		t.Fatalf("unexpected chat artifact content: %#v", item)
	}
}

func TestTransitConnectorCapturesExplicitKeys(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	store := &fakeTransitStore{records: map[string]transit.Record{
		"project/demo/decision": {
			TenantID:    7,
			KeyName:     "project/demo/decision",
			Description: "Decision note",
			Value:       "Use causal grounding for lifecycle transitions.",
			Version:     3,
			UpdatedBy:   42,
			UpdatedAt:   now,
		},
	}}
	connector := TransitConnector{Store: store}
	items, err := connector.Capture(context.Background(), artifact.CaptureRequest{
		TenantID: 7,
		Hints:    map[string]string{"keys": "project/demo/decision, project/demo/decision"},
	})
	if err != nil {
		t.Fatalf("Capture() error = %v", err)
	}
	if len(store.keys) != 1 || store.keys[0] != "project/demo/decision" {
		t.Fatalf("expected deduped requested keys, got %#v", store.keys)
	}
	if len(items) != 1 {
		t.Fatalf("expected one transit artifact, got %d", len(items))
	}
	item := items[0]
	if item.Kind != artifact.ArtifactTransitKey || item.ExternalID != "project/demo/decision@v3" || item.ContentHash == "" {
		t.Fatalf("unexpected transit artifact: %#v", item)
	}
	if item.AuthoredBy != "42" || item.Metadata["version"] != int64(3) {
		t.Fatalf("unexpected transit metadata: %#v", item)
	}
}

type fakeChatStore struct {
	messages []persistence.ChatMessage
}

func (f *fakeChatStore) Init(context.Context) error { return nil }
func (f *fakeChatStore) EnsureSession(context.Context, *int64, string, string) (persistence.ChatSession, error) {
	return persistence.ChatSession{}, nil
}
func (f *fakeChatStore) EnsureSessionKind(context.Context, *int64, string, string, string) (persistence.ChatSession, error) {
	return persistence.ChatSession{}, nil
}
func (f *fakeChatStore) ListSessions(context.Context, *int64) ([]persistence.ChatSession, error) {
	return nil, nil
}
func (f *fakeChatStore) ListSessionsByKind(context.Context, *int64, string) ([]persistence.ChatSession, error) {
	return nil, nil
}
func (f *fakeChatStore) GetSession(context.Context, *int64, string) (persistence.ChatSession, error) {
	return persistence.ChatSession{}, nil
}
func (f *fakeChatStore) CreateSession(context.Context, *int64, string) (persistence.ChatSession, error) {
	return persistence.ChatSession{}, nil
}
func (f *fakeChatStore) CreateSessionKind(context.Context, *int64, string, string) (persistence.ChatSession, error) {
	return persistence.ChatSession{}, nil
}
func (f *fakeChatStore) RenameSession(context.Context, *int64, string, string) (persistence.ChatSession, error) {
	return persistence.ChatSession{}, nil
}
func (f *fakeChatStore) SetSessionProject(context.Context, *int64, string, string) (persistence.ChatSession, error) {
	return persistence.ChatSession{}, nil
}
func (f *fakeChatStore) SetSessionMemorySettings(context.Context, *int64, string, bool, bool, bool) (persistence.ChatSession, error) {
	return persistence.ChatSession{}, nil
}
func (f *fakeChatStore) SetSessionActiveTarget(context.Context, *int64, string, string, string) (persistence.ChatSession, error) {
	return persistence.ChatSession{}, nil
}
func (f *fakeChatStore) SetSessionPinned(context.Context, *int64, string, bool) (persistence.ChatSession, error) {
	return persistence.ChatSession{}, nil
}
func (f *fakeChatStore) DeleteSession(context.Context, *int64, string) error { return nil }
func (f *fakeChatStore) ListMessages(_ context.Context, _ *int64, sessionID string, limit int) ([]persistence.ChatMessage, error) {
	out := []persistence.ChatMessage{}
	for _, msg := range f.messages {
		if msg.SessionID == sessionID {
			out = append(out, msg)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}
func (f *fakeChatStore) DeleteMessage(context.Context, *int64, string, string) error { return nil }
func (f *fakeChatStore) DeleteMessagesAfter(context.Context, *int64, string, string, bool) error {
	return nil
}
func (f *fakeChatStore) AppendMessages(context.Context, *int64, string, []persistence.ChatMessage, string, string) error {
	return nil
}
func (f *fakeChatStore) AppendMessagesOnce(context.Context, *int64, string, []persistence.ChatMessage, string, string) error {
	return nil
}
func (f *fakeChatStore) UpdateSummary(context.Context, *int64, string, string, int) error {
	return nil
}

type fakeTransitStore struct {
	records map[string]transit.Record
	keys    []string
}

func (f *fakeTransitStore) Init(context.Context) error { return nil }
func (f *fakeTransitStore) Create(context.Context, int64, int64, []transit.CreateMemoryItem) ([]transit.Record, error) {
	return nil, nil
}
func (f *fakeTransitStore) Get(_ context.Context, _ int64, keys []string) ([]transit.Record, error) {
	f.keys = append([]string(nil), keys...)
	out := []transit.Record{}
	for _, key := range keys {
		if record, ok := f.records[key]; ok {
			out = append(out, record)
		}
	}
	return out, nil
}
func (f *fakeTransitStore) Update(context.Context, int64, int64, transit.UpdateMemoryRequest) (transit.Record, error) {
	return transit.Record{}, nil
}
func (f *fakeTransitStore) Delete(context.Context, int64, []string) error { return nil }
func (f *fakeTransitStore) ListKeys(context.Context, int64, transit.ListRequest) ([]transit.Metadata, error) {
	return nil, nil
}
func (f *fakeTransitStore) ListRecent(context.Context, int64, transit.ListRequest) ([]transit.Metadata, error) {
	return nil, nil
}
func (f *fakeTransitStore) SearchText(context.Context, int64, transit.SearchRequest) ([]transit.SearchCandidate, error) {
	return nil, nil
}
