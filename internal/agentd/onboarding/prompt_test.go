package onboarding

import "testing"

func TestPromptIDsAreSystemScoped(t *testing.T) {
	t.Parallel()

	systemPrompt, systemVersion := PromptIDs(0, "default", "v1")
	if systemPrompt != "default" || systemVersion != "default-v1" {
		t.Fatalf("system IDs = %q, %q", systemPrompt, systemVersion)
	}
	userPrompt, userVersion := PromptIDs(42, "default", "v1")
	if userPrompt != "default-42" || userVersion != "default-42-v1" {
		t.Fatalf("user IDs = %q, %q", userPrompt, userVersion)
	}
}
