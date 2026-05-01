package agentd

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestParseProcessLogEntryPreservesAttributesAndTags(t *testing.T) {
	t.Parallel()

	entry := parseProcessLogEntry([]byte(`{"time":"2026-05-01T12:00:00Z","level":"debug","message":"llm_request","service":"manifold","trace_id":"trace-1","span_id":"span-1","prompt":"[{\"role\":\"system\",\"content\":\"sys\"}]","model":"gpt-5","component":"planner"}`))

	if entry.ID == "" {
		t.Fatal("expected stable log id")
	}
	if entry.Attributes["prompt"] == "" {
		t.Fatalf("expected prompt attribute to be preserved, got %+v", entry.Attributes)
	}
	if entry.Attributes["model"] != "gpt-5" {
		t.Fatalf("expected model attribute, got %+v", entry.Attributes)
	}
	if containsString(entry.Tags, "prompt:[{\"role\":\"system\",\"content\":\"sys\"}]") {
		t.Fatalf("prompt payload should not be promoted to tags: %+v", entry.Tags)
	}
	if !containsString(entry.Tags, "service:manifold") {
		t.Fatalf("expected service tag, got %+v", entry.Tags)
	}
	if !containsString(entry.Tags, "model:gpt-5") {
		t.Fatalf("expected compact attribute tag, got %+v", entry.Tags)
	}
}

func TestProcessLogDetailFindsEntryByID(t *testing.T) {
	t.Parallel()

	provider := newProcessLogMetrics(10)
	now := time.Now().UTC()
	_, _ = provider.Write([]byte(fmt.Sprintf(`{"time":%q,"level":"info","message":"first"}`,
		now.Add(-time.Minute).Format(time.RFC3339Nano))))
	_, _ = provider.Write([]byte(fmt.Sprintf(`{"time":%q,"level":"error","message":"second","request_id":"req-2"}`,
		now.Format(time.RFC3339Nano))))

	logs, _, err := provider.Logs(context.Background(), time.Hour, 10)
	if err != nil {
		t.Fatalf("Logs returned error: %v", err)
	}
	if len(logs) < 2 {
		t.Fatalf("expected logs to be stored, got %d", len(logs))
	}

	detail, _, err := provider.LogDetail(context.Background(), time.Hour, logs[0].ID)
	if err != nil {
		t.Fatalf("LogDetail returned error: %v", err)
	}
	if detail == nil {
		t.Fatal("expected matching log detail")
	}
	if detail.ID != logs[0].ID {
		t.Fatalf("expected matching id %q, got %q", logs[0].ID, detail.ID)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
