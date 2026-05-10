package matrixgw

import "testing"

func TestRouteMessage_MatchesCommandPrefix(t *testing.T) {
	route, ok := RouteMessage(RoomConfig{
		Mentions: map[string]string{
			"!razer": "razer",
		},
	}, "!razer status")
	if !ok {
		t.Fatalf("expected route match")
	}
	if route.Target != "razer" {
		t.Fatalf("unexpected target: %q", route.Target)
	}
	if route.Prompt != "status" {
		t.Fatalf("unexpected prompt: %q", route.Prompt)
	}
}

func TestRouteMessage_MatchesStandaloneMention(t *testing.T) {
	route, ok := RouteMessage(RoomConfig{
		Mentions: map[string]string{
			"@gpt_bot": "gpt",
		},
	}, "Hello @gpt_bot and @razer_bot. What are you up to?")
	if !ok {
		t.Fatalf("expected mention route match")
	}
	if route.Target != "gpt" {
		t.Fatalf("unexpected target: %q", route.Target)
	}
	if route.Prompt != "Hello @gpt_bot and @razer_bot. What are you up to?" {
		t.Fatalf("unexpected prompt: %q", route.Prompt)
	}
}

func TestRouteMessage_IgnoresPartialMention(t *testing.T) {
	_, ok := RouteMessage(RoomConfig{
		Mentions: map[string]string{
			"@gpt_bot": "gpt",
		},
	}, "Hello @gpt_bot_ops, are you there?")
	if ok {
		t.Fatalf("expected partial mention to be ignored")
	}
}

func TestRouteMessage_DefaultsWhenAllowed(t *testing.T) {
	route, ok := RouteMessage(RoomConfig{
		DefaultTarget:    "orchestrator",
		AllowUnmentioned: true,
	}, "what changed today?")
	if !ok {
		t.Fatalf("expected default route")
	}
	if route.Target != "orchestrator" {
		t.Fatalf("unexpected target: %q", route.Target)
	}
	if route.Prompt != "what changed today?" {
		t.Fatalf("unexpected prompt: %q", route.Prompt)
	}
}

func TestRouteMessage_RejectsUnmentionedWhenDisabled(t *testing.T) {
	_, ok := RouteMessage(RoomConfig{
		DefaultTarget:    "orchestrator",
		AllowUnmentioned: false,
	}, "what changed today?")
	if ok {
		t.Fatalf("expected unmatched message to be ignored")
	}
}

func TestRouteMessage_PrefersLongestAlias(t *testing.T) {
	route, ok := RouteMessage(RoomConfig{
		Mentions: map[string]string{
			"@gpt":     "gpt-short",
			"@gpt_bot": "gpt-long",
		},
	}, "hello @gpt_bot")
	if !ok {
		t.Fatalf("expected route match")
	}
	if route.Target != "gpt-long" {
		t.Fatalf("unexpected target: %q", route.Target)
	}
}
