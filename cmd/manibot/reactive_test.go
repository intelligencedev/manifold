package main

import (
	"strings"
	"testing"

	"github.com/matrix-org/gomatrix"
)

func TestShouldRespondAfterLease_WhenConversationAdvancedSkips(t *testing.T) {
	t.Parallel()

	trigger := gomatrix.Event{ID: "$trigger", Type: "m.room.message", Sender: "@user:test", Timestamp: 1000, Content: map[string]any{"body": "hello"}}
	recent := []gomatrix.Event{
		trigger,
		{ID: "$later", Type: "m.room.message", Sender: "@other:test", Timestamp: 2000, Content: map[string]any{"body": "I answered this"}},
	}

	if shouldRespondAfterLease(trigger, recent, "@bot:test", false) {
		t.Fatalf("expected later room activity to suppress a reactive response")
	}
}

func TestShouldRespondAfterLease_ExplicitTagStillResponds(t *testing.T) {
	t.Parallel()

	trigger := gomatrix.Event{ID: "$trigger", Type: "m.room.message", Sender: "@user:test", Timestamp: 1000, Content: map[string]any{"body": "@bot help"}}
	recent := []gomatrix.Event{
		trigger,
		{ID: "$later", Type: "m.room.message", Sender: "@other:test", Timestamp: 2000, Content: map[string]any{"body": "I answered this"}},
	}

	if !shouldRespondAfterLease(trigger, recent, "@bot:test", true) {
		t.Fatalf("expected explicit bot address to survive the wait-and-recheck path")
	}
}

func TestBuildReactivePromptIncludesRecentContext(t *testing.T) {
	t.Parallel()

	prompt := buildReactivePrompt(
		"@bot what changed?",
		"what changed?",
		[]gomatrix.Event{{Type: "m.room.message", Sender: "@user:test", Content: map[string]any{"body": "first"}}},
	)

	checks := []string{
		"[matrix reactive mode]",
		"Recent messages (oldest first):",
		"@user:test: first",
		"Current Matrix message:",
		"Extracted prompt for you:",
	}
	for _, check := range checks {
		if !strings.Contains(prompt, check) {
			t.Fatalf("expected prompt to contain %q, got %q", check, prompt)
		}
	}
}
