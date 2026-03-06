package projects

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServiceCRUD(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	svc := NewService(tmp, "")
	userID := int64(1)

	ctx := context.TODO()
	// Create project
	p, err := svc.CreateProject(ctx, userID, "My Project")
	if err != nil {
		t.Fatalf("CreateProject error: %v", err)
	}
	if p.ID == "" || p.Name != "My Project" {
		t.Fatalf("unexpected project: %+v", p)
	}
	// Metadata exists
	meta := filepath.Join(tmp, "users", "1", "projects", p.ID, ".meta", "project.json")
	if _, err := os.Stat(meta); err != nil {
		t.Fatalf("meta not found: %v", err)
	}

	// List projects
	list, err := svc.ListProjects(ctx, userID)
	if err != nil {
		t.Fatalf("ListProjects error: %v", err)
	}
	if len(list) != 1 || list[0].ID != p.ID {
		t.Fatalf("unexpected list: %+v", list)
	}

	// Create dir and upload file
	if err := svc.CreateDir(ctx, userID, p.ID, "/data"); err != nil {
		t.Fatalf("CreateDir error: %v", err)
	}
	body := strings.NewReader("hello")
	if err := svc.UploadFile(ctx, userID, p.ID, "/data", "a.txt", body); err != nil {
		t.Fatalf("UploadFile error: %v", err)
	}

	// List tree
	entries, err := svc.ListTree(ctx, userID, p.ID, "/data")
	if err != nil {
		t.Fatalf("ListTree error: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "a.txt" || entries[0].Type != "file" {
		t.Fatalf("unexpected entries: %+v", entries)
	}

	// Delete file
	if err := svc.DeleteFile(ctx, userID, p.ID, "/data/a.txt"); err != nil {
		t.Fatalf("DeleteFile error: %v", err)
	}
	entries, err = svc.ListTree(ctx, userID, p.ID, "/data")
	if err != nil {
		t.Fatalf("ListTree error: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty dir after delete, got: %+v", entries)
	}

	// Create nested folder and file, then delete the folder
	if err := svc.CreateDir(ctx, userID, p.ID, "/data/dir"); err != nil {
		t.Fatalf("CreateDir nested error: %v", err)
	}
	body2 := strings.NewReader("nested")
	if err := svc.UploadFile(ctx, userID, p.ID, "/data/dir", "n.txt", body2); err != nil {
		t.Fatalf("UploadFile nested error: %v", err)
	}
	// Sanity check listing
	entries, err = svc.ListTree(ctx, userID, p.ID, "/data")
	if err != nil {
		t.Fatalf("ListTree after nested create error: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "dir" || entries[0].Type != "dir" {
		t.Fatalf("unexpected entries after nested create: %+v", entries)
	}
	// Delete directory recursively
	if err := svc.DeleteFile(ctx, userID, p.ID, "/data/dir"); err != nil {
		t.Fatalf("DeleteFile directory error: %v", err)
	}
	entries, err = svc.ListTree(ctx, userID, p.ID, "/data")
	if err != nil {
		t.Fatalf("ListTree after dir delete error: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty dir after dir delete, got: %+v", entries)
	}

	// Delete project
	if err := svc.DeleteProject(ctx, userID, p.ID); err != nil {
		t.Fatalf("DeleteProject error: %v", err)
	}
	root := filepath.Join(tmp, "users", "1", "projects", p.ID)
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("project root should be gone, stat err=%v", err)
	}
}

func TestServiceMove(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	svc := NewService(tmp, "")
	userID := int64(7)

	ctx := context.TODO()
	proj, err := svc.CreateProject(ctx, userID, "Mover")
	if err != nil {
		t.Fatalf("CreateProject error: %v", err)
	}
	root := filepath.Join(tmp, "users", "7", "projects", proj.ID)

	if err := svc.CreateDir(ctx, userID, proj.ID, "/src"); err != nil {
		t.Fatalf("CreateDir error: %v", err)
	}
	if err := svc.UploadFile(ctx, userID, proj.ID, "/src", "a.txt", strings.NewReader("alpha")); err != nil {
		t.Fatalf("UploadFile error: %v", err)
	}

	// Move file to project root
	if err := svc.MovePath(ctx, userID, proj.ID, "/src/a.txt", "a.txt"); err != nil {
		t.Fatalf("MovePath file error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "a.txt")); err != nil {
		t.Fatalf("expected file at destination: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "src", "a.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected source file removed, stat err=%v", err)
	}

	// Prepare a directory move
	if err := svc.CreateDir(ctx, userID, proj.ID, "/src/assets"); err != nil {
		t.Fatalf("CreateDir nested error: %v", err)
	}
	if err := svc.UploadFile(ctx, userID, proj.ID, "/src/assets", "nested.txt", strings.NewReader("body")); err != nil {
		t.Fatalf("UploadFile nested error: %v", err)
	}
	if err := svc.MovePath(ctx, userID, proj.ID, "/src/assets", "assets"); err != nil {
		t.Fatalf("MovePath dir error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "assets", "nested.txt")); err != nil {
		t.Fatalf("expected moved directory contents: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "src", "assets")); !os.IsNotExist(err) {
		t.Fatalf("expected source directory removed, stat err=%v", err)
	}

	// Moving into a descendant should error
	if err := svc.MovePath(ctx, userID, proj.ID, "assets", "assets/subdir/assets"); err == nil {
		t.Fatalf("expected error when moving directory into descendant")
	}

	// Destination collision should error
	if err := svc.UploadFile(ctx, userID, proj.ID, ".", "a.txt", strings.NewReader("dupe")); err != nil {
		t.Fatalf("UploadFile collision prep error: %v", err)
	}
	if err := svc.MovePath(ctx, userID, proj.ID, "assets/nested.txt", "a.txt"); err == nil {
		t.Fatalf("expected error when destination exists")
	}
}

func TestCreateProject_SeedsDefaultSkills(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	sourceSkills := filepath.Join(tmp, "source", ".skills")
	if err := os.MkdirAll(filepath.Join(sourceSkills, "skill-a", "references"), 0o755); err != nil {
		t.Fatalf("MkdirAll source skills error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceSkills, "skill-a", "SKILL.md"), []byte("---\nname: skill-a\ndescription: test skill\n---\n\n# Skill A\n"), 0o644); err != nil {
		t.Fatalf("WriteFile SKILL.md error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceSkills, "skill-a", "references", "ref.txt"), []byte("reference"), 0o644); err != nil {
		t.Fatalf("WriteFile reference error: %v", err)
	}

	svc := NewService(tmp, sourceSkills)
	userID := int64(9)

	ctx := context.TODO()
	p, err := svc.CreateProject(ctx, userID, "With Skills")
	if err != nil {
		t.Fatalf("CreateProject error: %v", err)
	}
	if p.SkillsGeneration != 1 {
		t.Fatalf("expected SkillsGeneration=1, got %d", p.SkillsGeneration)
	}

	projectRoot := filepath.Join(tmp, "users", "9", "projects", p.ID)
	skillFile := filepath.Join(projectRoot, ".skills", "skill-a", "SKILL.md")
	if _, err := os.Stat(skillFile); err != nil {
		t.Fatalf("seeded SKILL.md not found: %v", err)
	}
	referenceFile := filepath.Join(projectRoot, ".skills", "skill-a", "references", "ref.txt")
	b, err := os.ReadFile(referenceFile)
	if err != nil {
		t.Fatalf("seeded reference file not found: %v", err)
	}
	if string(b) != "reference" {
		t.Fatalf("unexpected seeded reference contents: %q", string(b))
	}

	metaPath := filepath.Join(projectRoot, ".meta", "project.json")
	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("ReadFile project meta error: %v", err)
	}
	var meta struct {
		SkillsGeneration int64 `json:"skillsGeneration"`
	}
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		t.Fatalf("Unmarshal project meta error: %v", err)
	}
	if meta.SkillsGeneration != 1 {
		t.Fatalf("expected meta skillsGeneration=1, got %d", meta.SkillsGeneration)
	}
}

func TestCreateProject_NoDefaultSkills(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	svc := NewService(tmp, "")

	p, err := svc.CreateProject(context.TODO(), 11, "No Skills")
	if err != nil {
		t.Fatalf("CreateProject error: %v", err)
	}

	projectRoot := filepath.Join(tmp, "users", "11", "projects", p.ID)
	if _, err := os.Stat(filepath.Join(projectRoot, ".skills")); !os.IsNotExist(err) {
		t.Fatalf("expected no .skills directory, stat err=%v", err)
	}
	if p.SkillsGeneration != 0 {
		t.Fatalf("expected SkillsGeneration=0, got %d", p.SkillsGeneration)
	}
}
