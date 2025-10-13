package projects

import (
    "os"
    "path/filepath"
    "strings"
    "testing"
)

func TestServiceCRUD(t *testing.T) {
    t.Parallel()
    tmp := t.TempDir()
    svc := NewService(tmp)
    userID := int64(1)

    // Create project
    p, err := svc.CreateProject(nil, userID, "My Project")
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
    list, err := svc.ListProjects(nil, userID)
    if err != nil {
        t.Fatalf("ListProjects error: %v", err)
    }
    if len(list) != 1 || list[0].ID != p.ID {
        t.Fatalf("unexpected list: %+v", list)
    }

    // Create dir and upload file
    if err := svc.CreateDir(nil, userID, p.ID, "/data"); err != nil {
        t.Fatalf("CreateDir error: %v", err)
    }
    body := strings.NewReader("hello")
    if err := svc.UploadFile(nil, userID, p.ID, "/data", "a.txt", body); err != nil {
        t.Fatalf("UploadFile error: %v", err)
    }

    // List tree
    entries, err := svc.ListTree(nil, userID, p.ID, "/data")
    if err != nil {
        t.Fatalf("ListTree error: %v", err)
    }
    if len(entries) != 1 || entries[0].Name != "a.txt" || entries[0].Type != "file" {
        t.Fatalf("unexpected entries: %+v", entries)
    }

    // Delete file
    if err := svc.DeleteFile(nil, userID, p.ID, "/data/a.txt"); err != nil {
        t.Fatalf("DeleteFile error: %v", err)
    }
    entries, err = svc.ListTree(nil, userID, p.ID, "/data")
    if err != nil {
        t.Fatalf("ListTree error: %v", err)
    }
    if len(entries) != 0 {
        t.Fatalf("expected empty dir after delete, got: %+v", entries)
    }

    // Delete project
    if err := svc.DeleteProject(nil, userID, p.ID); err != nil {
        t.Fatalf("DeleteProject error: %v", err)
    }
    root := filepath.Join(tmp, "users", "1", "projects", p.ID)
    if _, err := os.Stat(root); !os.IsNotExist(err) {
        t.Fatalf("project root should be gone, stat err=%v", err)
    }
}

