package agentd

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"manifold/internal/agent"
	"manifold/internal/llm"
	"manifold/internal/persistence"
	"manifold/internal/persistence/databases"
	sqlitep "manifold/internal/persistence/sqlite"
	"manifold/internal/sandbox"
)

type recordingChatEventWriter struct {
	payloads []any
}

func (w *recordingChatEventWriter) write(payload any) { w.payloads = append(w.payloads, payload) }
func (w *recordingChatEventWriter) writeText(string)  {}

func TestConfigureCommonStreamCallbacksEmitsDeltaRollback(t *testing.T) {
	t.Parallel()

	eng := &agent.Engine{}
	writer := &recordingChatEventWriter{}
	configureCommonStreamCallbacks(eng, writer, false, false)

	if eng.OnStreamRollback == nil {
		t.Fatal("expected OnStreamRollback to be wired")
	}
	eng.OnStreamRollback(7)

	if len(writer.payloads) != 1 {
		t.Fatalf("expected one payload, got %#v", writer.payloads)
	}
	payload, ok := writer.payloads[0].(map[string]any)
	if !ok {
		t.Fatalf("expected map payload, got %#v", writer.payloads[0])
	}
	if payload["type"] != "delta_rollback" {
		t.Fatalf("expected delta_rollback event, got %#v", payload)
	}
	if payload["count"] != 7 {
		t.Fatalf("expected count 7, got %#v", payload["count"])
	}
}

func TestBuildChatJSONPayloadIncludesMatrixMessages(t *testing.T) {
	t.Parallel()

	outbox := sandbox.NewMatrixOutbox()
	outbox.Add("room-1", "hello")
	ctx := sandbox.WithMatrixOutbox(context.Background(), outbox)

	payload := buildChatJSONPayload("done", nil, nil, ctx, true)

	if payload["result"] != "done" {
		t.Fatalf("expected result payload, got %#v", payload["result"])
	}
	messages, ok := payload["matrix_messages"].([]sandbox.MatrixMessage)
	if !ok {
		t.Fatalf("expected matrix messages in payload, got %#v", payload["matrix_messages"])
	}
	if len(messages) != 1 || messages[0].RoomID != "room-1" || messages[0].Text != "hello" {
		t.Fatalf("unexpected matrix messages: %#v", messages)
	}
}

func TestBuildChatJSONPayloadIncludesImages(t *testing.T) {
	t.Parallel()

	images := []savedImage{{Name: "image-1.png", MIME: "image/png", URL: "/api/projects/p/files?path=images%2Fimage-1.png"}}
	payload := buildChatJSONPayload("done", images, nil, context.Background(), false)

	got, ok := payload["images"].([]savedImage)
	if !ok {
		t.Fatalf("expected images in payload, got %#v", payload["images"])
	}
	if len(got) != 1 || got[0].Name != "image-1.png" || got[0].MIME != "image/png" {
		t.Fatalf("unexpected images payload: %#v", got)
	}
}

func TestBuildChatStreamFinalPayloadOmitsMatrixMessagesWhenDisabled(t *testing.T) {
	t.Parallel()

	outbox := sandbox.NewMatrixOutbox()
	outbox.Add("room-1", "hello")
	ctx := sandbox.WithMatrixOutbox(context.Background(), outbox)

	payload := buildChatStreamFinalPayload("done", ctx, false)

	if payload["type"] != "final" || payload["data"] != "done" {
		t.Fatalf("unexpected stream payload: %#v", payload)
	}
	if _, ok := payload["matrix_messages"]; ok {
		t.Fatalf("expected matrix messages to be omitted: %#v", payload)
	}
}

func TestBuildChatStreamFinalPayloadIncludesDuration(t *testing.T) {
	t.Parallel()

	payload := buildChatStreamFinalPayload("done", context.Background(), false, 12437)

	if payload["durationMs"] != int64(12437) {
		t.Fatalf("expected durationMs in payload, got %#v", payload)
	}
}

func TestChatTurnCollectorResultTextAppendsImageSummary(t *testing.T) {
	t.Parallel()

	collector := &chatTurnCollector{
		savedImages: []savedImage{{Name: "image-1", URL: "/audio/image-1"}},
	}

	result := collector.resultText("base output")
	if result == "base output" {
		t.Fatalf("expected image summary to be appended")
	}
	if want := "Generated images:"; !contains(result, want) {
		t.Fatalf("expected %q in result %q", want, result)
	}
	if want := "/audio/image-1"; !contains(result, want) {
		t.Fatalf("expected %q in result %q", want, result)
	}
}

func TestApplyChatImagePromptPrefersInheritedContext(t *testing.T) {
	t.Parallel()

	runCtx := llm.WithImagePrompt(context.Background(), llm.ImagePromptOptions{Size: "2K"})
	ctx := applyChatImagePrompt(context.Background(), runCtx, chatRunRequest{Image: true, ImageSize: "1K"}, true)

	opts, ok := llm.ImagePromptFromContext(ctx)
	if !ok {
		t.Fatal("expected image prompt options in context")
	}
	if opts.Size != "2K" {
		t.Fatalf("expected inherited image size, got %q", opts.Size)
	}
}

func TestChatStoreModelPrefersOverride(t *testing.T) {
	t.Parallel()

	if got := chatStoreModel(nil, "team:gpt-4.1"); got != "team:gpt-4.1" {
		t.Fatalf("expected override model label, got %q", got)
	}
}

func TestStoreChatTurnWithHistoryKeepsAssistantAfterPrePersistedUser(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := databases.NewSQLiteChatStore(openAgentdTestSQLite(t))
	sessionID := "session-turn-order"
	if _, err := store.EnsureSession(ctx, nil, sessionID, "Turn Order"); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	startedAt := time.Now().UTC().Add(-2 * time.Hour)
	if err := store.AppendMessagesOnce(ctx, nil, sessionID, []persistence.ChatMessage{
		{ID: "user-1", SessionID: sessionID, Role: "user", Content: "first prompt", CreatedAt: startedAt},
	}, "first prompt", ""); err != nil {
		t.Fatalf("AppendMessagesOnce user-1: %v", err)
	}
	if err := store.AppendMessagesOnce(ctx, nil, sessionID, []persistence.ChatMessage{
		{ID: "user-2", SessionID: sessionID, Role: "user", Content: "second prompt", CreatedAt: startedAt.Add(time.Second)},
	}, "second prompt", ""); err != nil {
		t.Fatalf("AppendMessagesOnce user-2: %v", err)
	}

	durationMs := int64(500)
	if err := storeChatTurnWithHistory(ctx, store, chatTurnHistoryRecord{
		SessionID:          sessionID,
		UserMessageID:      "user-1",
		UserContent:        "first prompt",
		TurnMessages:       []llm.Message{{Role: "assistant", Content: "first response"}},
		FinalContent:       "first response",
		AssistantMessageID: "assistant-1",
		DurationMs:         &durationMs,
	}); err != nil {
		t.Fatalf("storeChatTurnWithHistory: %v", err)
	}

	messages, err := store.ListMessages(ctx, nil, sessionID, 0)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	got := make([]string, 0, len(messages))
	for _, message := range messages {
		got = append(got, message.ID)
	}
	want := []string{"user-1", "assistant-1", "user-2"}
	if len(got) != len(want) {
		t.Fatalf("message order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("message order = %v, want %v", got, want)
		}
	}
}

func openAgentdTestSQLite(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sqlitep.Open(context.Background(), sqlitep.Config{
		Path:          filepath.Join(t.TempDir(), "manifold.db"),
		BusyTimeoutMs: 10000,
		WAL:           true,
		MaxOpenConns:  1,
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})())
}
