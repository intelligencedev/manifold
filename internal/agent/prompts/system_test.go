package prompts

import (
	"strings"
	"testing"
)

func TestDefaultSystemPrompt_IncludesHTMLRenderingInstructions(t *testing.T) {
	prompt := DefaultSystemPrompt("/tmp/workdir", "")

	expected := []string{
		"To render HTML in chat, emit raw HTML in the markdown body.",
		"Do not fence or indent renderable HTML unless the user wants source code only.",
		"top-level div and inline styles.",
		"Never include <script>, event handlers, forms, iframes, or external embeds.",
		"emit raw HTML first, then a fenced html block.",
	}

	for _, want := range expected {
		if !strings.Contains(prompt, want) {
			t.Fatalf("default system prompt missing HTML instruction: %q", want)
		}
	}
}

func TestDefaultSystemPrompt_AppendsOverrideAfterBaseInstructions(t *testing.T) {
	override := "Custom orchestrator instructions."
	prompt := DefaultSystemPrompt("/tmp/workdir", override)

	if !strings.Contains(prompt, "To render HTML in chat") {
		t.Fatal("expected base HTML rendering instructions to be preserved when override is set")
	}
	if !strings.Contains(prompt, override) {
		t.Fatalf("expected override to be included in system prompt: %q", override)
	}
	if strings.Index(prompt, override) < strings.Index(prompt, "HTML Rendering:") {
		t.Fatal("expected override to be appended after base instructions")
	}
}
