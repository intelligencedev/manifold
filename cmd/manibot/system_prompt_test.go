package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMatrixSystemPrompt_Default(t *testing.T) {
	oldPrompt := os.Getenv("MANIFOLD_SYSTEM_PROMPT")
	oldFile := os.Getenv("MANIFOLD_SYSTEM_PROMPT_FILE")
	defer func() {
		_ = os.Setenv("MANIFOLD_SYSTEM_PROMPT", oldPrompt)
		_ = os.Setenv("MANIFOLD_SYSTEM_PROMPT_FILE", oldFile)
	}()

	_ = os.Unsetenv("MANIFOLD_SYSTEM_PROMPT")
	_ = os.Unsetenv("MANIFOLD_SYSTEM_PROMPT_FILE")

	prompt, err := loadMatrixSystemPrompt()
	if err != nil {
		t.Fatalf("loadMatrixSystemPrompt() error: %v", err)
	}
	if prompt != defaultMatrixSystemPrompt {
		t.Fatalf("expected default prompt")
	}
}

func TestLoadMatrixSystemPrompt_FromEnv(t *testing.T) {
	oldPrompt := os.Getenv("MANIFOLD_SYSTEM_PROMPT")
	oldFile := os.Getenv("MANIFOLD_SYSTEM_PROMPT_FILE")
	defer func() {
		_ = os.Setenv("MANIFOLD_SYSTEM_PROMPT", oldPrompt)
		_ = os.Setenv("MANIFOLD_SYSTEM_PROMPT_FILE", oldFile)
	}()

	_ = os.Unsetenv("MANIFOLD_SYSTEM_PROMPT_FILE")
	_ = os.Setenv("MANIFOLD_SYSTEM_PROMPT", "custom matrix prompt")

	prompt, err := loadMatrixSystemPrompt()
	if err != nil {
		t.Fatalf("loadMatrixSystemPrompt() error: %v", err)
	}
	if !strings.HasPrefix(prompt, "custom matrix prompt") {
		t.Fatalf("unexpected prompt: %q", prompt)
	}
	if !strings.Contains(prompt, "When multiple bots are tagged, answer only for yourself.") {
		t.Fatalf("expected multi-bot guidance in prompt: %q", prompt)
	}
}

func TestLoadMatrixSystemPrompt_FromFile(t *testing.T) {
	oldPrompt := os.Getenv("MANIFOLD_SYSTEM_PROMPT")
	oldFile := os.Getenv("MANIFOLD_SYSTEM_PROMPT_FILE")
	defer func() {
		_ = os.Setenv("MANIFOLD_SYSTEM_PROMPT", oldPrompt)
		_ = os.Setenv("MANIFOLD_SYSTEM_PROMPT_FILE", oldFile)
	}()

	dir := t.TempDir()
	path := filepath.Join(dir, "matrix-prompt.txt")
	if err := os.WriteFile(path, []byte("prompt from file"), 0o644); err != nil {
		t.Fatalf("write prompt file: %v", err)
	}
	_ = os.Setenv("MANIFOLD_SYSTEM_PROMPT", "ignored")
	_ = os.Setenv("MANIFOLD_SYSTEM_PROMPT_FILE", path)

	prompt, err := loadMatrixSystemPrompt()
	if err != nil {
		t.Fatalf("loadMatrixSystemPrompt() error: %v", err)
	}
	if !strings.HasPrefix(prompt, "prompt from file") {
		t.Fatalf("unexpected prompt: %q", prompt)
	}
	if !strings.Contains(prompt, "Do not claim to speak for another bot") {
		t.Fatalf("expected multi-bot guidance in prompt: %q", prompt)
	}
}

func TestAppendMatrixPromptSuffix_DoesNotDuplicateGuidance(t *testing.T) {
	prompt := appendMatrixPromptSuffix("custom matrix prompt" + matrixMultiBotPromptSuffix)
	if strings.Count(prompt, "When multiple bots are tagged, answer only for yourself.") != 1 {
		t.Fatalf("expected multi-bot guidance once, got %q", prompt)
	}
}

func TestManifoldPromptRequestIncludesSystemPrompt(t *testing.T) {
	body := manifoldPromptRequest{
		Prompt:       "hello",
		RoomID:       "!room:example.com",
		SessionID:    "matrix:room",
		ProjectID:    "project-123",
		SystemPrompt: "matrix persona",
	}

	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(data) == "" || !json.Valid(data) {
		t.Fatalf("invalid json: %s", string(data))
	}
	if got := string(data); !containsAll(got, []string{"system_prompt", "matrix persona", "!room:example.com"}) {
		t.Fatalf("unexpected payload: %s", got)
	}
}

func containsAll(s string, parts []string) bool {
	for _, part := range parts {
		if !strings.Contains(s, part) {
			return false
		}
	}
	return true
}
