package skills

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func bundleV1() fstest.MapFS {
	return fstest.MapFS{
		"alpha/SKILL.md":        {Data: []byte("---\nname: alpha\ndescription: A\n---\nv1")},
		"alpha/references/x.md": {Data: []byte("ref v1")},
		"alpha/scripts/run.sh":  {Data: []byte("#!/bin/sh\necho hi")},
		"beta/SKILL.md":         {Data: []byte("---\nname: beta\ndescription: B\n---\nb")},
	}
}

func read(t *testing.T, parts ...string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(parts...))
	if err != nil {
		t.Fatalf("read %v: %v", parts, err)
	}
	return string(b)
}

func TestInstallBundledSkillsFreshCopy(t *testing.T) {
	dest := t.TempDir()
	names, err := InstallBundledSkills(bundleV1(), dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "alpha" || names[1] != "beta" {
		t.Fatalf("names=%v", names)
	}
	if got := read(t, dest, "alpha", "SKILL.md"); got != "---\nname: alpha\ndescription: A\n---\nv1" {
		t.Fatalf("alpha SKILL.md=%q", got)
	}
	if got := read(t, dest, "alpha", "references", "x.md"); got != "ref v1" {
		t.Fatalf("ref=%q", got)
	}
	// .sh files are made executable.
	info, err := os.Stat(filepath.Join(dest, "alpha", "scripts", "run.sh"))
	if err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("run.sh should be executable, mode=%v err=%v", info.Mode(), err)
	}
}

func TestInstallBundledSkillsSkipsUnchanged(t *testing.T) {
	dest := t.TempDir()
	if _, err := InstallBundledSkills(bundleV1(), dest); err != nil {
		t.Fatal(err)
	}
	// A user/local marker inside a bundled skill must survive a no-op reinstall
	// (proving unchanged skills are not rewritten).
	sentinel := filepath.Join(dest, "alpha", "SKILL.md")
	if err := os.WriteFile(sentinel, []byte("LOCALLY EDITED"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallBundledSkills(bundleV1(), dest); err != nil {
		t.Fatal(err)
	}
	if got := read(t, dest, "alpha", "SKILL.md"); got != "LOCALLY EDITED" {
		t.Fatalf("unchanged skill must not be rewritten, got %q", got)
	}
}

func TestInstallBundledSkillsOverwritesChanged(t *testing.T) {
	dest := t.TempDir()
	if _, err := InstallBundledSkills(bundleV1(), dest); err != nil {
		t.Fatal(err)
	}
	// Edit locally, then ship a changed bundle: the change must overwrite.
	_ = os.WriteFile(filepath.Join(dest, "alpha", "SKILL.md"), []byte("STALE"), 0o644)

	v2 := bundleV1()
	v2["alpha/SKILL.md"] = &fstest.MapFile{Data: []byte("v2 content")}
	delete(v2, "alpha/references/x.md") // a file removed from the skill
	if _, err := InstallBundledSkills(v2, dest); err != nil {
		t.Fatal(err)
	}
	if got := read(t, dest, "alpha", "SKILL.md"); got != "v2 content" {
		t.Fatalf("changed skill not overwritten, got %q", got)
	}
	if _, err := os.Stat(filepath.Join(dest, "alpha", "references", "x.md")); !os.IsNotExist(err) {
		t.Fatal("removed-from-bundle file should be gone after reinstall")
	}
}

func TestInstallBundledSkillsRemovesDroppedAndKeepsUserSkills(t *testing.T) {
	dest := t.TempDir()
	if _, err := InstallBundledSkills(bundleV1(), dest); err != nil {
		t.Fatal(err)
	}
	// A user-authored skill that the bundle never managed.
	userDir := filepath.Join(dest, "mine")
	_ = os.MkdirAll(userDir, 0o755)
	_ = os.WriteFile(filepath.Join(userDir, "SKILL.md"), []byte("mine"), 0o644)

	// Next release drops beta.
	v2 := fstest.MapFS{"alpha/SKILL.md": {Data: []byte("a")}}
	names, err := InstallBundledSkills(v2, dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "alpha" {
		t.Fatalf("names=%v", names)
	}
	if _, err := os.Stat(filepath.Join(dest, "beta")); !os.IsNotExist(err) {
		t.Fatal("dropped bundled skill should be removed")
	}
	if got := read(t, dest, "mine", "SKILL.md"); got != "mine" {
		t.Fatalf("user skill must be preserved, got %q", got)
	}
}

func TestInstallBundledSkillsRecopiesMissingDest(t *testing.T) {
	dest := t.TempDir()
	if _, err := InstallBundledSkills(bundleV1(), dest); err != nil {
		t.Fatal(err)
	}
	// User deletes the dir but the manifest still records the hash.
	if err := os.RemoveAll(filepath.Join(dest, "alpha")); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallBundledSkills(bundleV1(), dest); err != nil {
		t.Fatal(err)
	}
	if got := read(t, dest, "alpha", "SKILL.md"); got == "" {
		t.Fatal("missing dest should be re-copied")
	}
}
