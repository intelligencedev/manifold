package agentd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSkillSearchToolFindsRelevantSkills(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	writeSkillFile(t, projectDir, "pdf-context-builder", "Extract text and structure from PDF files.")
	writeSkillFile(t, projectDir, "deploy-runbook", "Deploy the application safely.")

	tool := newSkillSearchTool(projectDir)
	raw, err := json.Marshal(skillSearchInput{Query: "pdf extraction"})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	result, err := tool.Call(context.Background(), raw)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	matches, ok := result.([]skillSearchResult)
	if !ok {
		t.Fatalf("unexpected result type: %T", result)
	}
	if len(matches) == 0 || matches[0].Name != "pdf-context-builder" {
		t.Fatalf("expected pdf-context-builder first, got %#v", matches)
	}
}

func TestSkillSearchToolLoadsExactSkillByName(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	writeSkillFile(t, projectDir, "deploy-runbook", "Deploy the application safely.")

	tool := newSkillSearchTool(projectDir)
	raw, err := json.Marshal(skillSearchInput{Names: []string{"deploy-runbook"}})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	result, err := tool.Call(context.Background(), raw)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	matches := result.([]skillSearchResult)
	if len(matches) != 1 {
		t.Fatalf("expected one match, got %#v", matches)
	}
	if !matches[0].Exact {
		t.Fatalf("expected exact match, got %#v", matches[0])
	}
	if matches[0].Path == "" {
		t.Fatalf("expected skill path, got %#v", matches[0])
	}
}

func writeSkillFile(t *testing.T, projectDir, name, description string) {
	t.Helper()
	skillPath := filepath.Join(projectDir, ".skills", name, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillPath), 0o755); err != nil {
		t.Fatalf("mkdir skills dir: %v", err)
	}
	content := []byte("---\nname: " + name + "\ndescription: " + description + "\n---\n# " + name + "\n")
	if err := os.WriteFile(skillPath, content, 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
}
