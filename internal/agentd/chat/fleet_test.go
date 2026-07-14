package chat

import (
	"testing"

	"manifold/internal/agent"
	"manifold/internal/fleet"
)

func TestAttachFleetCallbacksPreservesEngineCallbackAndPublishes(t *testing.T) {
	t.Parallel()

	bus := fleet.NewBus(4)
	eng := &agent.Engine{}
	called := false
	eng.OnToolStart = func(string, []byte, string) { called = true }

	AttachFleetCallbacks(bus, eng, FleetCallbackRequest{RunID: "run-1", SessionID: "sess-1", UserID: int64Ptr(9)})
	eng.OnToolStart("read_file", []byte(`{"path":"README.md"}`), "tool-1")

	if !called {
		t.Fatal("previous tool callback was not preserved")
	}
	events := bus.Recent(9)
	if len(events) != 1 || events[0].Kind != fleet.EventToolStart || events[0].RunID != "run-1" {
		t.Fatalf("events = %#v, want one run-1 tool-start event", events)
	}
}

func int64Ptr(value int64) *int64 {
	return &value
}
