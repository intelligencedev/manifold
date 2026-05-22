package agentd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillSearchToolFindsRelevantSkills(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
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
	// Paths must be project-relative (no absolute paths leaked to agents).
	if filepath.IsAbs(matches[0].Path) {
		t.Fatalf("expected relative path, got absolute: %q", matches[0].Path)
	}
	if matches[0].SkillID == "" || matches[0].Source != "project" {
		t.Fatalf("expected project skill id/source, got %#v", matches[0])
	}
}

func TestSkillSearchToolLoadsExactSkillByName(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
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
	// Paths must be project-relative (no absolute paths leaked to agents).
	if filepath.IsAbs(matches[0].Path) {
		t.Fatalf("expected relative path, got absolute: %q", matches[0].Path)
	}
}

func TestSkillSearchToolFindsUniversalSkills(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	projectDir := t.TempDir()
	writeSkillFileAt(t, filepath.Join(home, ".agents", "skills"), "global-skill", "Global workflow.")

	tool := newSkillSearchTool(projectDir)
	raw, err := json.Marshal(skillSearchInput{Names: []string{"global-skill"}})
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
	if matches[0].Source != "agents" || matches[0].SkillID == "" {
		t.Fatalf("expected agents skill metadata, got %#v", matches[0])
	}
	if filepath.IsAbs(matches[0].Path) {
		t.Fatalf("expected non-absolute display path, got %q", matches[0].Path)
	}
}

func TestSkillSearchToolCanReturnAllSkills(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	projectDir := t.TempDir()
	writeSkillFile(t, projectDir, "project-skill", "Project workflow.")
	writeSkillFileAt(t, filepath.Join(home, ".manifold", "skills"), "manifold-skill", "Manifold workflow.")
	writeSkillFileAt(t, filepath.Join(home, ".agents", "skills"), "agents-skill", "Agents workflow.")

	tool := newSkillSearchTool(projectDir)
	raw, err := json.Marshal(skillSearchInput{All: true})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	result, err := tool.Call(context.Background(), raw)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	matches := result.([]skillSearchResult)
	if len(matches) != 3 {
		t.Fatalf("expected all three skills, got %#v", matches)
	}
	want := []string{"project-skill", "manifold-skill", "agents-skill"}
	for i, name := range want {
		if matches[i].Name != name {
			t.Fatalf("expected match %d to be %q, got %#v", i, name, matches)
		}
		if matches[i].SkillID == "" || matches[i].Path == "" || matches[i].Source == "" {
			t.Fatalf("expected complete metadata, got %#v", matches[i])
		}
	}
}

func TestSkillReadToolReadsSkillFiles(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	writeSkillFile(t, projectDir, "deploy-runbook", "Deploy the application safely.")
	refPath := filepath.Join(projectDir, "skills", "deploy-runbook", "references", "ref.txt")
	if err := os.MkdirAll(filepath.Dir(refPath), 0o755); err != nil {
		t.Fatalf("mkdir ref dir: %v", err)
	}
	if err := os.WriteFile(refPath, []byte("reference"), 0o644); err != nil {
		t.Fatalf("write ref: %v", err)
	}

	tool := newSkillReadTool(projectDir)
	raw, err := json.Marshal(skillReadInput{SkillID: "project:deploy-runbook", Paths: []string{"SKILL.md", "references/ref.txt"}})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	result, err := tool.Call(context.Background(), raw)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	read := result.(skillReadResult)
	if !read.OK || len(read.Files) != 2 {
		t.Fatalf("expected two readable files, got %#v", read)
	}
	if !strings.Contains(read.Files[0].Content, "# deploy-runbook") || read.Files[1].Content != "reference" {
		t.Fatalf("unexpected contents: %#v", read.Files)
	}
}

func TestSkillReadToolRejectsUnsafePaths(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	writeSkillFile(t, projectDir, "deploy-runbook", "Deploy the application safely.")
	symlinkPath := filepath.Join(projectDir, "skills", "deploy-runbook", "link.txt")
	if err := os.Symlink(filepath.Join(projectDir, "skills", "deploy-runbook", "SKILL.md"), symlinkPath); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	tool := newSkillReadTool(projectDir)
	for _, input := range []skillReadInput{
		{SkillID: "project:deploy-runbook", Path: "../secret.txt"},
		{SkillID: "project:deploy-runbook", Path: "/etc/passwd"},
		{SkillID: "project:deploy-runbook", Path: "link.txt"},
		{SkillID: "project:missing", Path: "SKILL.md"},
	} {
		raw, err := json.Marshal(input)
		if err != nil {
			t.Fatalf("marshal input: %v", err)
		}
		result, err := tool.Call(context.Background(), raw)
		if err != nil {
			t.Fatalf("Call: %v", err)
		}
		read := result.(skillReadResult)
		if read.OK {
			t.Fatalf("expected unsafe read to fail for %#v: %#v", input, read)
		}
	}
}

func writeSkillFile(t *testing.T, projectDir, name, description string) {
	t.Helper()
	writeSkillFileAt(t, filepath.Join(projectDir, "skills"), name, description)
}

func writeSkillFileAt(t *testing.T, root, name, description string) {
	t.Helper()
	skillPath := filepath.Join(root, name, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillPath), 0o755); err != nil {
		t.Fatalf("mkdir skills dir: %v", err)
	}
	content := []byte("---\nname: " + name + "\ndescription: " + description + "\n---\n# " + name + "\n")
	if err := os.WriteFile(skillPath, content, 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
}
