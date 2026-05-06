package agentd

import (
	"testing"

	"manifold/internal/agent"
)

func TestChatActivityCollectorBuildsDelegatedActivity(t *testing.T) {
	t.Parallel()

	userID := int64(5)
	collector := newChatActivityCollector("sess-1", "run-1", &userID)
	collector.Handle(agent.AgentTrace{Type: "agent_start", Agent: "developer", Model: "gpt-5", CallID: "call-1", ParentCallID: "tool-1", Depth: 1, Content: "Write code"})
	collector.Handle(agent.AgentTrace{Type: "agent_thought_summary", Agent: "developer", CallID: "call-1", ThoughtSummary: "Planning"})
	collector.Handle(agent.AgentTrace{Type: "agent_delta", Agent: "developer", CallID: "call-1", Content: "hello"})
	collector.Handle(agent.AgentTrace{Type: "agent_tool_result", Agent: "developer", CallID: "call-1", Title: "Tool", Data: "ok", ToolID: "tool-1"})
	collector.Handle(agent.AgentTrace{Type: "agent_final", Agent: "developer", CallID: "call-1", Content: "hello world"})

	activities := collector.Snapshot()
	if len(activities) != 1 {
		t.Fatalf("expected 1 activity, got %d", len(activities))
	}
	activity := activities[0]
	if activity.RunID != "run-1" {
		t.Fatalf("expected run-1, got %q", activity.RunID)
	}
	if activity.ParentCallID != "tool-1" {
		t.Fatalf("expected parent call tool-1, got %q", activity.ParentCallID)
	}
	if activity.Status != "done" {
		t.Fatalf("expected done status, got %q", activity.Status)
	}
	if activity.Content != "hello world" {
		t.Fatalf("expected final content, got %q", activity.Content)
	}
	if len(activity.ThoughtSummaries) != 1 || activity.ThoughtSummaries[0] != "Planning" {
		t.Fatalf("unexpected thought summaries: %#v", activity.ThoughtSummaries)
	}
	if len(activity.Entries) != 1 || activity.Entries[0].Data != "ok" {
		t.Fatalf("unexpected entries: %#v", activity.Entries)
	}
}
