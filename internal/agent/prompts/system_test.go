package prompts

import (
	"os"
	"path/filepath"
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

func TestCachedSkillsForProjectLoadsMetadata(t *testing.T) {
	projectDir := t.TempDir()
	skillPath := filepath.Join(projectDir, ".skills", "pdf-context-builder", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillPath), 0o755); err != nil {
		t.Fatalf("mkdir skills dir: %v", err)
	}
	content := strings.Join([]string{
		"---",
		"name: pdf-context-builder",
		"description: Extract text and structure from PDF files.",
		"metadata:",
		"  short-description: Build PDF context",
		"---",
		"# PDF Context Builder",
	}, "\n")
	if err := os.WriteFile(skillPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	cached, err := CachedSkillsForProject(projectDir)
	if err != nil {
		t.Fatalf("CachedSkillsForProject: %v", err)
	}
	if cached == nil || len(cached.Skills) != 1 {
		t.Fatalf("expected one cached skill, got %#v", cached)
	}
	if cached.Skills[0].Name != "pdf-context-builder" {
		t.Fatalf("unexpected skill name: %q", cached.Skills[0].Name)
	}
	if !strings.Contains(cached.RenderedPrompt, "## Skills") {
		t.Fatalf("expected rendered prompt, got %q", cached.RenderedPrompt)
	}
}
