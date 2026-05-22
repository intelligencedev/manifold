package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromDirLoadsProjectSkills(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectDir := t.TempDir()
	writeTestSkill(t, filepath.Join(projectDir, "skills"), "project-skill", "Project skill")

	outcome := LoadFromDir(projectDir)
	if len(outcome.Errors) != 0 {
		t.Fatalf("unexpected errors: %#v", outcome.Errors)
	}
	if len(outcome.Skills) != 1 {
		t.Fatalf("expected one skill, got %#v", outcome.Skills)
	}
	got := outcome.Skills[0]
	if got.Name != "project-skill" || got.Source != SourceProject || got.SkillID == "" {
		t.Fatalf("unexpected skill metadata: %#v", got)
	}
	if filepath.IsAbs(got.Path) {
		t.Fatalf("expected display path to be relative, got %q", got.Path)
	}
}

func TestLoadFromDirLoadsUniversalSkillsAndDeduplicatesByPrecedence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	projectDir := t.TempDir()

	writeTestSkill(t, filepath.Join(home, ".agents", "skills"), "shared-skill", "Agents skill")
	writeTestSkill(t, filepath.Join(home, ".agents", "skills"), "agents-only", "Agents only")
	writeTestSkill(t, filepath.Join(home, ".manifold", "skills"), "shared-skill", "Manifold skill")
	writeTestSkill(t, filepath.Join(home, ".manifold", "skills"), "manifold-only", "Manifold only")
	writeTestSkill(t, filepath.Join(projectDir, "skills"), "shared-skill", "Project skill")

	outcome := LoadFromDir(projectDir)
	if len(outcome.Errors) != 0 {
		t.Fatalf("unexpected errors: %#v", outcome.Errors)
	}
	byName := make(map[string]Metadata)
	for _, md := range outcome.Skills {
		byName[md.Name] = md
		if filepath.IsAbs(md.Path) {
			t.Fatalf("expected display path to be relative, got %#v", md)
		}
		if md.SkillDir == "" || md.SkillID == "" {
			t.Fatalf("expected internal resolution metadata, got %#v", md)
		}
	}
	if len(byName) != 3 {
		t.Fatalf("expected three deduplicated skills, got %#v", outcome.Skills)
	}
	if byName["shared-skill"].Source != SourceProject {
		t.Fatalf("expected project skill to win, got %#v", byName["shared-skill"])
	}
	if byName["manifold-only"].Source != SourceManifold {
		t.Fatalf("expected manifold skill, got %#v", byName["manifold-only"])
	}
	if byName["agents-only"].Source != SourceAgents {
		t.Fatalf("expected agents skill, got %#v", byName["agents-only"])
	}
}

func TestUniversalFingerprintChangesWhenUniversalSkillChanges(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSkill(t, filepath.Join(home, ".manifold", "skills"), "one", "One")
	first := UniversalFingerprint()
	writeTestSkill(t, filepath.Join(home, ".manifold", "skills"), "two", "Two")
	second := UniversalFingerprint()
	if first == second {
		t.Fatalf("expected fingerprint to change after universal skill update")
	}
}

func writeTestSkill(t *testing.T, root, name, description string) {
	t.Helper()
	path := filepath.Join(root, name, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	content := "---\nname: " + name + "\ndescription: " + description + "\n---\n# " + name + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
}
