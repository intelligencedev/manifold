package projects

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"manifold/internal/sandbox"
	"manifold/internal/validation"
)

// Project describes a per-user project stored on the filesystem.
type Project struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Generation       int64     `json:"generation"`
	SkillsGeneration int64     `json:"skillsGeneration"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
	Bytes            int64     `json:"bytes"`
	FileCount        int       `json:"fileCount"`
}

// FileEntry represents a single file or directory under a project.
type FileEntry struct {
	Path    string    `json:"path"`
	Name    string    `json:"name"`
	Type    string    `json:"type"` // file | dir
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mtime"`
}

// Service provides filesystem-backed project operations under a WORKDIR.
type Service struct {
	workdir string
}

// NewService creates a new filesystem-backed projects service.
func NewService(workdir string) *Service { return &Service{workdir: workdir} }

func (s *Service) userRoot(userID int64) string {
	return filepath.Join(s.workdir, "users", fmt.Sprint(userID), "projects")
}

func (s *Service) projectRoot(userID int64, projectID string) (string, error) {
	cleanPID, err := validation.ProjectID(projectID)
	if err != nil {
		return "", err
	}
	if cleanPID == "" {
		return "", fmt.Errorf("invalid project id")
	}
	return resolveUnderRoot(s.userRoot(userID), cleanPID)
}

func resolveUnderRoot(absBase, rel string) (string, error) {
	absRoot, err := filepath.Abs(absBase)
	if err != nil {
		return "", err
	}
	absPath, err := filepath.Abs(filepath.Join(absRoot, rel))
	if err != nil {
		return "", err
	}
	relToRoot, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return "", err
	}
	if relToRoot == "." || relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("invalid path")
	}
	return absPath, nil
}

type projectMeta struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Generation       int64     `json:"generation"`
	SkillsGeneration int64     `json:"skillsGeneration"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

func (s *Service) metaPath(root string) string {
	return filepath.Join(root, ".meta", "project.json")
}

func ensureDir(path string, perm fs.FileMode) error {
	if err := os.MkdirAll(path, perm); err != nil {
		return err
	}
	return nil
}

// sanitizeUnder trims a single leading slash or backslash (for URL style inputs)
// and validates the path remains under base using sandbox.SanitizeArg.
func sanitizeUnder(base, p string) (string, error) {
	pp := strings.TrimSpace(p)
	for len(pp) > 0 && (pp[0] == '/' || pp[0] == '\\') {
		pp = pp[1:]
	}
	if pp == "" {
		return ".", nil
	}
	return sandbox.SanitizeArg(base, pp)
}

// CreateProject provisions a new project directory for the given user.
func (s *Service) CreateProject(_ context.Context, userID int64, name string) (Project, error) {
	if strings.TrimSpace(name) == "" {
		name = "Untitled"
	}
	id := uuid.NewString()
	root, err := s.projectRoot(userID, id)
	if err != nil {
		return Project{}, err
	}
	if err := ensureDir(root, 0o755); err != nil {
		return Project{}, err
	}
	// Write metadata
	if err := ensureDir(filepath.Join(root, ".meta"), 0o755); err != nil {
		return Project{}, err
	}
	now := time.Now().UTC()
	meta := projectMeta{ID: id, Name: name, CreatedAt: now, UpdatedAt: now, Generation: 0, SkillsGeneration: 0}
	if b, err := json.MarshalIndent(meta, "", "  "); err == nil {
		_ = os.WriteFile(s.metaPath(root), b, 0o644)
	}
	// Seed helper files (best-effort)
	_ = os.WriteFile(filepath.Join(root, "README.md"), []byte("# Project\n\nThis directory is managed by the platform.\n"), 0o644)
	return Project{ID: id, Name: name, CreatedAt: now, UpdatedAt: now, Bytes: 0, FileCount: 0, Generation: 0, SkillsGeneration: 0}, nil
}

// DeleteProject recursively deletes the project directory for a user.
func (s *Service) DeleteProject(_ context.Context, userID int64, projectID string) error {
	root, err := s.projectRoot(userID, projectID)
	if err != nil {
		return err
	}
	if _, err := os.Stat(root); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return os.RemoveAll(root)
}

// ListProjects lists all projects for a user, computing size and file count.
func (s *Service) ListProjects(_ context.Context, userID int64) ([]Project, error) {
	base := s.userRoot(userID)
	entries, err := os.ReadDir(base)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Project{}, nil
		}
		return nil, err
	}
	out := make([]Project, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		root := filepath.Join(base, e.Name())
		p, ok := s.readProject(root)
		if !ok {
			// fallback from FS info
			info, _ := e.Info()
			t := time.Now().UTC()
			if info != nil {
				t = info.ModTime().UTC()
			}
			p = Project{ID: e.Name(), Name: e.Name(), CreatedAt: t, UpdatedAt: t}
		}
		bytes, files := s.computeUsage(root)
		p.Bytes, p.FileCount = bytes, files
		out = append(out, p)
	}
	// Sort by UpdatedAt desc then Name
	sort.Slice(out, func(i, j int) bool {
		if !out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func (s *Service) readProject(root string) (Project, bool) {
	b, err := os.ReadFile(s.metaPath(root))
	if err != nil {
		return Project{}, false
	}
	var m projectMeta
	if err := json.Unmarshal(b, &m); err != nil {
		return Project{}, false
	}
	return Project{ID: m.ID, Name: m.Name, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt, Generation: m.Generation, SkillsGeneration: m.SkillsGeneration}, true
}

func (s *Service) writeUpdatedAt(userID int64, projectID string, t time.Time, bumpGeneration bool, bumpSkills bool) {
	root, err := s.projectRoot(userID, projectID)
	if err != nil {
		return
	}
	b, err := os.ReadFile(s.metaPath(root))
	if err != nil {
		return
	}
	var m projectMeta
	if err := json.Unmarshal(b, &m); err != nil {
		return
	}
	m.UpdatedAt = t.UTC()
	if bumpGeneration {
		m.Generation++
	}
	if bumpSkills {
		m.SkillsGeneration++
	}
	if nb, err := json.MarshalIndent(m, "", "  "); err == nil {
		_ = os.WriteFile(s.metaPath(root), nb, 0o644)
	}
}

func (s *Service) computeUsage(root string) (int64, int) {
	var (
		bytes int64
		files int
	)
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		// Skip metadata dir
		if d.IsDir() && filepath.Base(p) == ".meta" {
			return filepath.SkipDir
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if !d.IsDir() {
			if info, err := d.Info(); err == nil {
				bytes += info.Size()
				files++
			}
		}
		return nil
	})
	return bytes, files
}

// ListTree lists entries directly under path within a project.
func (s *Service) ListTree(_ context.Context, userID int64, projectID, path string) ([]FileEntry, error) {
	base, err := s.projectRoot(userID, projectID)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(base); err != nil {
		return nil, err
	}
	rel, err := sanitizeUnder(base, path)
	if err != nil {
		return nil, err
	}
	full := filepath.Join(base, rel)
	entries, err := os.ReadDir(full)
	if err != nil {
		return nil, err
	}
	out := make([]FileEntry, 0, len(entries))
	for _, e := range entries {
		// skip metadata folder from listing
		if e.IsDir() && e.Name() == ".meta" && (rel == "." || rel == "" || rel == "/") {
			continue
		}
		if e.Type()&fs.ModeSymlink != 0 {
			// do not traverse symlinks
			continue
		}
		info, _ := e.Info()
		typ := "file"
		size := int64(0)
		mtime := time.Now().UTC()
		if info != nil {
			mtime = info.ModTime().UTC()
			if e.IsDir() {
				typ = "dir"
			} else {
				size = info.Size()
			}
		}
		p := e.Name()
		if rel != "." && rel != "" {
			p = filepath.ToSlash(filepath.Join(rel, e.Name()))
		}
		out = append(out, FileEntry{Path: p, Name: e.Name(), Type: typ, Size: size, ModTime: mtime})
	}
	// sort by name, dirs first
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type != out[j].Type {
			return out[i].Type == "dir"
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// UploadFile writes a file into a project under path with the given name.
func (s *Service) UploadFile(_ context.Context, userID int64, projectID, path, name string, r io.Reader) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("empty file name")
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("invalid file name")
	}
	base, err := s.projectRoot(userID, projectID)
	if err != nil {
		return err
	}
	rel, err := sanitizeUnder(base, path)
	if err != nil {
		return err
	}
	dir := filepath.Join(base, rel)
	if err := ensureDir(dir, 0o755); err != nil {
		return err
	}
	dst := filepath.Join(dir, name)
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return err
	}
	fullRel := filepath.ToSlash(filepath.Join(rel, name))
	bumpSkills := strings.HasPrefix(fullRel, ".manifold/skills/") || fullRel == ".manifold/skills" || fullRel == "manifold/skills"
	s.writeUpdatedAt(userID, projectID, time.Now().UTC(), true, bumpSkills)
	return nil
}

// DeleteFile removes a single filesystem entry within a project.
//
// If the path points to a file, it is deleted. If it points to a directory,
// the directory is removed recursively (like rm -r). Symlinks are never
// followed and will not be deleted.
func (s *Service) DeleteFile(_ context.Context, userID int64, projectID, path string) error {
	base, err := s.projectRoot(userID, projectID)
	if err != nil {
		return err
	}
	rel, err := sanitizeUnder(base, path)
	if err != nil {
		return err
	}
	if rel == "." || rel == "" {
		return fmt.Errorf("invalid path")
	}
	target := filepath.Join(base, rel)
	info, err := os.Lstat(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		// refuse to delete symlinks
		return fmt.Errorf("refusing to delete symlink")
	}
	// Delete files directly; delete directories recursively.
	if info.IsDir() {
		if err := os.RemoveAll(target); err != nil {
			return err
		}
	} else {
		if err := os.Remove(target); err != nil {
			return err
		}
	}
	fullRel := filepath.ToSlash(rel)
	bumpSkills := strings.HasPrefix(fullRel, ".manifold/skills/") || fullRel == ".manifold/skills" || fullRel == "manifold/skills"
	s.writeUpdatedAt(userID, projectID, time.Now().UTC(), true, bumpSkills)
	return nil
}

// MovePath relocates a file or directory within a project. The destination
// path must include the final filename (or directory name) and must not
// already exist. Moving into a descendant of the source directory is refused.
func (s *Service) MovePath(_ context.Context, userID int64, projectID, from, to string) error {
	base, err := s.projectRoot(userID, projectID)
	if err != nil {
		return err
	}
	srcRel, err := sanitizeUnder(base, from)
	if err != nil {
		return err
	}
	if srcRel == "." || srcRel == "" {
		return fmt.Errorf("invalid source path")
	}
	dstRel, err := sanitizeUnder(base, to)
	if err != nil {
		return err
	}
	if dstRel == "." || dstRel == "" {
		return fmt.Errorf("invalid destination path")
	}
	srcAbs := filepath.Join(base, srcRel)
	dstAbs := filepath.Join(base, dstRel)
	info, err := os.Lstat(srcAbs)
	if err != nil {
		return err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return fmt.Errorf("refusing to move symlink")
	}
	if _, err := os.Lstat(dstAbs); err == nil {
		return fmt.Errorf("destination exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if rel, err := filepath.Rel(srcAbs, dstAbs); err == nil {
		rel = filepath.ToSlash(rel)
		if rel == "." || rel == "" || !strings.HasPrefix(rel, "..") {
			return fmt.Errorf("cannot move into descendant")
		}
	}
	if err := ensureDir(filepath.Dir(dstAbs), 0o755); err != nil {
		return err
	}
	if err := os.Rename(srcAbs, dstAbs); err != nil {
		return err
	}
	fullSrc := filepath.ToSlash(srcRel)
	fullDst := filepath.ToSlash(dstRel)
	bumpSkills := strings.HasPrefix(fullSrc, ".manifold/skills/") || strings.HasPrefix(fullDst, ".manifold/skills/") || fullSrc == ".manifold/skills" || fullDst == ".manifold/skills" || strings.HasPrefix(fullSrc, "manifold/skills") || strings.HasPrefix(fullDst, "manifold/skills")
	s.writeUpdatedAt(userID, projectID, time.Now().UTC(), true, bumpSkills)
	return nil
}

// CreateDir creates a directory (and parents) within a project.
func (s *Service) CreateDir(_ context.Context, userID int64, projectID, path string) error {
	base, err := s.projectRoot(userID, projectID)
	if err != nil {
		return err
	}
	rel, err := sanitizeUnder(base, path)
	if err != nil {
		return err
	}
	dir := filepath.Join(base, rel)
	if err := ensureDir(dir, 0o755); err != nil {
		return err
	}
	fullRel := filepath.ToSlash(rel)
	bumpSkills := strings.HasPrefix(fullRel, ".manifold/skills/") || fullRel == ".manifold/skills" || strings.HasPrefix(fullRel, "manifold/skills")
	s.writeUpdatedAt(userID, projectID, time.Now().UTC(), true, bumpSkills)
	return nil
}

// ReadFile opens a file for reading and returns a reader for the raw file contents.
func (s *Service) ReadFile(_ context.Context, userID int64, projectID, path string) (io.ReadCloser, error) {
	base, err := s.projectRoot(userID, projectID)
	if err != nil {
		return nil, err
	}
	rel, err := sanitizeUnder(base, path)
	if err != nil {
		return nil, err
	}
	if rel == "." || rel == "" {
		return nil, fmt.Errorf("invalid path")
	}
	p := filepath.Join(base, rel)
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	return f, nil
}
