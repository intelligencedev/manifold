package main

import "testing"

func TestPulseMessagesIgnoresMissingQueuedMessages(t *testing.T) {
	t.Parallel()

	messages := pulseMessages(nil, "!room:test")
	if len(messages) != 0 {
		t.Fatalf("expected no queued messages, got %#v", messages)
	}
}

func TestPulseMessagesFiltersAndNormalizesQueuedMessages(t *testing.T) {
	t.Parallel()

	messages := pulseMessages([]manifoldMatrixMessage{
		{Text: "  first  "},
		{RoomID: "!other:test", Text: "second"},
		{RoomID: " ", Text: "   "},
	}, "!room:test")
	if len(messages) != 2 {
		t.Fatalf("expected 2 queued messages, got %d", len(messages))
	}
	if messages[0].RoomID != "!room:test" || messages[0].Text != "first" {
		t.Fatalf("unexpected first message: %#v", messages[0])
	}
	if messages[1].RoomID != "!other:test" || messages[1].Text != "second" {
		t.Fatalf("unexpected second message: %#v", messages[1])
	}
}
