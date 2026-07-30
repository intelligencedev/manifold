package lexminify

import (
	"strings"
	"testing"
	"unicode/utf8"

	"manifold/internal/llm"
)

func TestOffIsNoop(t *testing.T) {
	in := "Please note that the system must never delete without approval."
	if got := MinifyString(in, Off); got != in {
		t.Fatalf("level 0 should be identity, got %q", got)
	}
}

func TestL1RemovesFiller(t *testing.T) {
	in := "Please note that we need results in order to proceed."
	got := MinifyString(in, L1Fillers)
	if strings.Contains(strings.ToLower(got), "please note that") {
		t.Fatalf("filler not removed: %q", got)
	}
	if !strings.Contains(got, "to proceed") && !strings.Contains(got, "proceed") {
		t.Fatalf("expected content retained: %q", got)
	}
	if utf8.RuneCountInString(got) >= utf8.RuneCountInString(in) {
		t.Fatalf("expected shorter output, before=%d after=%d out=%q", len(in), len(got), got)
	}
}

func TestNeverDropNegations(t *testing.T) {
	in := "The service must not delete the database without approval and can never run alone."
	got := MinifyString(in, L3Telegram)
	lower := strings.ToLower(got)
	for _, keep := range []string{"must", "not", "never"} {
		if !strings.Contains(lower, keep) && !strings.Contains(lower, "w/o") {
			// "without" may become w/o; "not"/"must"/"never" must remain.
			if keep != "without" {
				t.Fatalf("protected word %q dropped from %q", keep, got)
			}
		}
	}
	if !strings.Contains(lower, "not") {
		t.Fatalf("negation 'not' dropped: %q", got)
	}
	if !strings.Contains(lower, "must") {
		t.Fatalf("polarity 'must' dropped: %q", got)
	}
	if !strings.Contains(lower, "never") {
		t.Fatalf("polarity 'never' dropped: %q", got)
	}
}

func TestProtectsCodeJSONURLUUID(t *testing.T) {
	in := "Run `npm install` then POST https://example.com/v1/widgets " +
		"with uuid 550e8400-e29b-41d4-a716-446655440000 and payload " +
		"```json\n{\"foo\": \"bar\", \"count\": 12}\n``` " +
		"in order to finish."
	got := MinifyString(in, L4Vowels)
	if !strings.Contains(got, "`npm install`") {
		t.Fatalf("inline code mangled: %q", got)
	}
	if !strings.Contains(got, "https://example.com/v1/widgets") {
		t.Fatalf("url mangled: %q", got)
	}
	if !strings.Contains(got, "550e8400-e29b-41d4-a716-446655440000") {
		t.Fatalf("uuid mangled: %q", got)
	}
	if !strings.Contains(got, "```json\n{\"foo\": \"bar\", \"count\": 12}\n```") {
		t.Fatalf("fenced json mangled: %q", got)
	}
}

func TestVowelSkeleton(t *testing.T) {
	in := "compression algorithm representatives"
	got := MinifyString(in, L4Vowels)
	if got == in {
		t.Fatalf("expected vowel strip, got identical")
	}
	// short words unchanged; long words skeletonized, first letter kept.
	if !strings.Contains(got, "cmprssn") && !strings.Contains(got, "c") {
		t.Fatalf("unexpected skeleton: %q", got)
	}
}

func TestMessagesZonesMinifySystemByDefault(t *testing.T) {
	msgs := []llm.Message{
		{Role: "system", Content: "You are a helpful assistant. Please note that tools exist."},
		{Role: "user", Content: "[CONVERSATION HISTORY]\nThe old question was about databases in order to migrate."},
		{Role: "assistant", Content: "I recommended the migration path in order to reduce risk."},
		{Role: "user", Content: "[RUNTIME CONTEXT]\nPlease note that memory says the user prefers Go.\n\n[CURRENT REQUEST]\nImplement the feature without breaking tests."},
	}
	res := MinifyMessages(msgs, Options{Level: L3Telegram})
	sys := res.Messages[0].Content
	if !strings.HasPrefix(sys, LexMinifyAdvisoryMarker) {
		t.Fatalf("system should start with unminified advisory, got %q", sys)
	}
	if !strings.Contains(sys, LexMinifyAdvisory) {
		t.Fatalf("system missing full plain-text advisory: %q", sys)
	}
	body := stripLexMinifyAdvisory(sys)
	if body == msgs[0].Content {
		t.Fatalf("system body should minify under default zones")
	}
	if strings.Contains(strings.ToLower(body), "please note that") {
		t.Fatalf("system filler not removed: %q", body)
	}
	if res.Messages[1].Content == msgs[1].Content {
		t.Fatalf("history should minify")
	}
	if res.Messages[2].Content == msgs[2].Content {
		t.Fatalf("assistant history should minify")
	}
	cur := res.Messages[3].Content
	if !strings.Contains(cur, "[CURRENT REQUEST]") {
		t.Fatalf("current request marker lost")
	}
	// current request stays light under default zones
	if !strings.Contains(cur, "Implement the feature without breaking tests.") &&
		!strings.Contains(cur, "without") {
		t.Fatalf("current request over-minified: %q", cur)
	}
	if strings.Contains(cur, "Please note that memory") {
		t.Fatalf("runtime filler not removed: %q", cur)
	}
}

func TestSystemPromptZoneCanBeDisabled(t *testing.T) {
	msgs := []llm.Message{
		{Role: "system", Content: "You are a helpful assistant. Please note that tools exist."},
		{Role: "developer", Content: "Please note that responses must stay concise."},
		{Role: "user", Content: "Please note that we need a plan in order to ship."},
		{Role: "assistant", Content: "I proposed steps in order to reduce risk."},
		{Role: "user", Content: "[CURRENT REQUEST]\nDo the thing."},
	}
	// Omit ZoneSystemPrompt while still minifying history.
	zones := ZoneRuntimeContext | ZoneHistory | ZoneAssistant
	res := MinifyMessages(msgs, Options{Level: L3Telegram, Zones: zones})
	if res.Messages[0].Content != msgs[0].Content {
		t.Fatalf("system should be unchanged without ZoneSystemPrompt")
	}
	if strings.Contains(res.Messages[0].Content, LexMinifyAdvisoryMarker) {
		t.Fatalf("advisory must not appear without ZoneSystemPrompt")
	}
	if res.Messages[1].Content != msgs[1].Content {
		t.Fatalf("developer should be unchanged without ZoneSystemPrompt")
	}
	if res.Messages[2].Content == msgs[2].Content {
		t.Fatalf("user history should still minify")
	}
	if res.Messages[3].Content == msgs[3].Content {
		t.Fatalf("assistant history should still minify")
	}
}

func TestMessagesZonesMinifyToolByDefault(t *testing.T) {
	msgs := []llm.Message{
		{Role: "user", Content: "Older ask about storage."},
		{Role: "assistant", Content: "I will look that up in order to answer."},
		{Role: "tool", ToolID: "call_1", Content: "Please note that the search found documents in order to summarize. {\"total\": 2, \"items\": [\"a\", \"b\"]}"},
		{Role: "user", Content: "[CURRENT REQUEST]\nSummarize findings."},
	}
	res := MinifyMessages(msgs, Options{Level: L3Telegram})
	tool := res.Messages[2].Content
	if tool == msgs[2].Content {
		t.Fatalf("tool payload should minify under default zones")
	}
	if strings.Contains(strings.ToLower(tool), "please note that") {
		t.Fatalf("tool filler not removed: %q", tool)
	}
	// Structured JSON fragment must remain intact via protect spans.
	if !strings.Contains(tool, `{"total": 2, "items": ["a", "b"]}`) {
		t.Fatalf("structured tool JSON mangled: %q", tool)
	}
}

func TestToolZoneCanBeDisabled(t *testing.T) {
	msgs := []llm.Message{
		{Role: "tool", ToolID: "call_1", Content: "Please note that the lookup succeeded in order to continue."},
		{Role: "user", Content: "Please note that we still need history compression in order to ship."},
		{Role: "assistant", Content: "I proposed a path in order to reduce risk."},
		{Role: "user", Content: "[CURRENT REQUEST]\nContinue."},
	}
	// Keep other defaults but drop ZoneTool.
	zones := ZoneRuntimeContext | ZoneHistory | ZoneAssistant | ZoneSystemPrompt
	res := MinifyMessages(msgs, Options{Level: L3Telegram, Zones: zones})
	if res.Messages[0].Content != msgs[0].Content {
		t.Fatalf("tool should be unchanged without ZoneTool: got %q", res.Messages[0].Content)
	}
	if res.Messages[1].Content == msgs[1].Content {
		t.Fatalf("user history should still minify")
	}
	if res.Messages[2].Content == msgs[2].Content {
		t.Fatalf("assistant history should still minify")
	}
}

func TestDeterministic(t *testing.T) {
	in := "In order to compress the prompt, please note that stopwords help approximately 40%."
	a := MinifyString(in, L4Vowels)
	b := MinifyString(in, L4Vowels)
	if a != b {
		t.Fatalf("non-deterministic: %q vs %q", a, b)
	}
}

func TestUserLiteralPromptNeverMinified(t *testing.T) {
	const userBody = "We are noticing the user's prompt is being minified. The actual message sent by the user should NEVER be minified in order to preserve intent."
	msgs := []llm.Message{
		{Role: "system", Content: "You are a helpful assistant. Please note that tools exist."},
		{Role: "user", Content: "[RUNTIME CONTEXT]\nPlease note that memory says the user prefers concise Go code in order to ship.\n\n[CURRENT REQUEST]\n" + userBody},
	}
	// Even with ZoneCurrentRequest intentionally enabled and a high cap, the live
	// user body must remain byte-identical after minification.
	res := MinifyMessages(msgs, Options{
		Level:                  L5Aggressive,
		Zones:                  DefaultZones | ZoneCurrentRequest,
		CurrentRequestMaxLevel: L5Aggressive,
	})
	cur := res.Messages[1].Content
	if !strings.Contains(cur, userBody) {
		t.Fatalf("user literal prompt was minified:\n--- got ---\n%s\n--- want substring ---\n%s", cur, userBody)
	}
	// Runtime context still minifies.
	if strings.Contains(cur, "Please note that memory") {
		t.Fatalf("runtime filler not removed: %q", cur)
	}
	if !strings.Contains(cur, "[CURRENT REQUEST]") {
		t.Fatalf("current request marker lost")
	}
}

func TestAlwaysMarkedCurrentRequestPreservesBody(t *testing.T) {
	const userBody = "Implement the feature without breaking tests in order to ship safely."
	// Shape produced by BuildInitialLLMMessages after always-annotate change.
	content := "[CURRENT REQUEST]\nThis is the user's current message. Respond to THIS message only.\n---\n" + userBody
	msgs := []llm.Message{
		{Role: "user", Content: "[RUNTIME CONTEXT]\nPlease note that prior summary is bulky in order to waste tokens.\n\n" + content},
	}
	res := MinifyMessages(msgs, Options{Level: L4Vowels})
	got := res.Messages[0].Content
	if !strings.Contains(got, userBody) {
		t.Fatalf("annotated user body minified: %q", got)
	}
	if strings.Contains(strings.ToLower(got), "please note that prior summary") {
		t.Fatalf("runtime context still contains filler: %q", got)
	}
}

func TestSystemAdvisoryUnminifiedAndSingle(t *testing.T) {
	msgs := []llm.Message{
		{Role: "system", Content: "You are a helpful assistant. Please note that tools exist."},
		{Role: "developer", Content: "Please note that responses must stay concise."},
		{Role: "user", Content: "[CURRENT REQUEST]\nDo the thing."},
	}
	res := MinifyMessages(msgs, Options{Level: L5Aggressive})
	sys := res.Messages[0].Content
	if !strings.HasPrefix(strings.TrimSpace(sys), LexMinifyAdvisoryMarker) {
		t.Fatalf("advisory must lead the first system message: %q", sys)
	}
	// Advisory body must remain byte-identical (never minified).
	if !strings.Contains(sys, LexMinifyAdvisory) {
		t.Fatalf("advisory text was altered or minified:\n--- got ---\n%s", sys)
	}
	if count := strings.Count(sys, LexMinifyAdvisoryMarker); count != 1 {
		t.Fatalf("expected single advisory marker in system, got %d", count)
	}
	// Second prefix message is minified but does not get a second advisory.
	dev := res.Messages[1].Content
	if strings.Contains(dev, LexMinifyAdvisoryMarker) {
		t.Fatalf("developer must not receive a second advisory: %q", dev)
	}
	if strings.Contains(strings.ToLower(dev), "please note that") {
		t.Fatalf("developer filler not removed: %q", dev)
	}
	// Re-running minify on already-annotated output must not stack notices.
	again := MinifyMessages(res.Messages, Options{Level: L5Aggressive})
	if count := strings.Count(again.Messages[0].Content, LexMinifyAdvisoryMarker); count != 1 {
		t.Fatalf("re-minify stacked advisories: count=%d content=%q", count, again.Messages[0].Content)
	}
	if !strings.Contains(again.Messages[0].Content, LexMinifyAdvisory) {
		t.Fatalf("re-minify altered advisory text")
	}
}

func TestSystemAdvisoryAbsentWhenFeatureOff(t *testing.T) {
	msgs := []llm.Message{
		{Role: "system", Content: "You are a helpful assistant. Please note that tools exist."},
		{Role: "user", Content: "[CURRENT REQUEST]\nHi"},
	}
	res := MinifyMessages(msgs, Options{Level: Off})
	if res.Messages[0].Content != msgs[0].Content {
		t.Fatalf("level off should not rewrite system")
	}
	if strings.Contains(res.Messages[0].Content, LexMinifyAdvisoryMarker) {
		t.Fatalf("advisory must not appear when level is off")
	}
}
