package agent

import (
	"testing"

	"singularityio/internal/llm"
)

func TestBuildInitialLLMMessages(t *testing.T) {
	hist := []llm.Message{{Role: "user", Content: "prev"}}
	msgs := BuildInitialLLMMessages("sys", "now", hist)
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "system" || msgs[0].Content != "sys" {
		t.Fatalf("unexpected system msg: %#v", msgs[0])
	}
	if msgs[1].Content != "prev" || msgs[2].Content != "now" {
		t.Fatalf("unexpected history/user: %#v", msgs)
	}

	// no system or history
	msgs = BuildInitialLLMMessages("", "only", nil)
	if len(msgs) != 1 || msgs[0].Content != "only" {
		t.Fatalf("unexpected single message: %#v", msgs)
	}
}
