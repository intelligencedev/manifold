package skills

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoaderLoad_ParsesProjectSkills(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	// Simulate repo root.
	repo := filepath.Join(tmp, "repo")
	require.NoError(t, os.MkdirAll(filepath.Join(repo, ".git"), 0o755))

	// Repo skill.
	repoSkill := filepath.Join(repo, ".skills", "alpha")
	require.NoError(t, os.MkdirAll(repoSkill, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repoSkill, "SKILL.md"), []byte(`---
name: alpha
description: repo alpha
metadata:
  short-description: repo short
---

# Body
`), 0o644))

	loader := Loader{
		Workdir: repo,
	}

	out := loader.Load()
	require.Empty(t, out.Errors)
	require.Len(t, out.Skills, 1)
	require.Equal(t, "alpha", out.Skills[0].Name)
	require.Equal(t, "repo alpha", out.Skills[0].Description)
	require.Equal(t, "repo short", out.Skills[0].ShortDescription)
	require.Equal(t, ScopeRepo, out.Skills[0].Scope)
}

func TestLoaderLoad_RejectsMissingFrontmatter(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	repo := filepath.Join(tmp, "repo")
	require.NoError(t, os.MkdirAll(filepath.Join(repo, ".git"), 0o755))
	skillDir := filepath.Join(repo, ".skills", "oops")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# no frontmatter\n"), 0o644))

	loader := Loader{Workdir: repo}
	out := loader.Load()
	require.Len(t, out.Errors, 1)
}
