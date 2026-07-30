package chat

import (
	"encoding/json"
	"testing"
)

func TestRunRequestAcceptsLegacyMemoryFields(t *testing.T) {
	var req RunRequest
	if err := json.Unmarshal([]byte(`{"prompt":"hello","bot_id":"writer","memory_enabled":true}`), &req); err != nil {
		t.Fatal(err)
	}
	if req.RouteTarget != "writer" || req.MemoryEnabled == nil || !*req.MemoryEnabled {
		t.Fatalf("request = %#v", req)
	}
}
