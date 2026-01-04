package databases

import (
	"context"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"manifold/internal/persistence"
)

// newMemoryProjectsStore returns an in-memory projects store for tests/dev.
func newMemoryProjectsStore() persistence.ProjectsStore {
	return &memProjectsStore{
		projects: make(map[string]persistence.Project),
		files:    make(map[string]map[string]persistence.ProjectFile),
	}
}

type memProjectsStore struct {
	mu       sync.RWMutex
	projects map[string]persistence.Project                // keyed by project ID
	files    map[string]map[string]persistence.ProjectFile // keyed by project ID, then path
}

func (s *memProjectsStore) Init(ctx context.Context) error {
	return nil
}

func (s *memProjectsStore) Create(ctx context.Context, userID int64, name string) (persistence.Project, error) {
	if strings.TrimSpace(name) == "" {
		name = "Untitled"
	}
	now := time.Now().UTC()
	p := persistence.Project{
		ID:             uuid.NewString(),
		UserID:         userID,
		Name:           name,
		CreatedAt:      now,
		UpdatedAt:      now,
		Revision:       1,
		Bytes:          0,
		FileCount:      0,
		StorageBackend: "filesystem",
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.projects[p.ID] = p
	s.files[p.ID] = make(map[string]persistence.ProjectFile)
	return p, nil
}

func (s *memProjectsStore) Get(ctx context.Context, userID int64, projectID string) (persistence.Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, ok := s.projects[projectID]
	if !ok {
		return persistence.Project{}, persistence.ErrNotFound
	}
	if p.UserID != userID {
		return persistence.Project{}, persistence.ErrForbidden
	}
	return p, nil
}

func (s *memProjectsStore) List(ctx context.Context, userID int64) ([]persistence.Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []persistence.Project
	for _, p := range s.projects {
		if p.UserID == userID {
			result = append(result, p)
		}
	}

	// Sort by UpdatedAt desc, then Name asc
	sort.Slice(result, func(i, j int) bool {
		if !result[i].UpdatedAt.Equal(result[j].UpdatedAt) {
			return result[i].UpdatedAt.After(result[j].UpdatedAt)
		}
		return result[i].Name < result[j].Name
	})

	if result == nil {
		result = []persistence.Project{}
	}
	return result, nil
}

func (s *memProjectsStore) Update(ctx context.Context, p persistence.Project) (persistence.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.projects[p.ID]
	if !ok {
		return persistence.Project{}, persistence.ErrNotFound
	}
	if existing.Revision != p.Revision {
		return persistence.Project{}, persistence.ErrRevisionConflict
	}

	// Update mutable fields
	existing.Name = p.Name
	existing.StorageBackend = p.StorageBackend
	existing.UpdatedAt = time.Now().UTC()
	existing.Revision++

	s.projects[p.ID] = existing
	return existing, nil
}

func (s *memProjectsStore) UpdateStats(ctx context.Context, projectID string, bytes int64, fileCount int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.projects[projectID]
	if !ok {
		return persistence.ErrNotFound
	}
	p.Bytes = bytes
	p.FileCount = fileCount
	p.UpdatedAt = time.Now().UTC()
	s.projects[projectID] = p
	return nil
}

func (s *memProjectsStore) Delete(ctx context.Context, userID int64, projectID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.projects[projectID]
	if !ok {
		return nil // Already deleted
	}
	if p.UserID != userID {
		return persistence.ErrForbidden
	}

	delete(s.projects, projectID)
	delete(s.files, projectID)
	return nil
}

// --- File Index Operations ---

func (s *memProjectsStore) IndexFile(ctx context.Context, f persistence.ProjectFile) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.files[f.ProjectID] == nil {
		s.files[f.ProjectID] = make(map[string]persistence.ProjectFile)
	}
	f.UpdatedAt = time.Now().UTC()
	s.files[f.ProjectID][f.Path] = f
	return nil
}

func (s *memProjectsStore) RemoveFileIndex(ctx context.Context, projectID, filePath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.files[projectID] != nil {
		delete(s.files[projectID], filePath)
	}
	return nil
}

func (s *memProjectsStore) RemoveFileIndexPrefix(ctx context.Context, projectID, pathPrefix string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.files[projectID] == nil {
		return nil
	}

	prefix := pathPrefix
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	for p := range s.files[projectID] {
		if p == pathPrefix || strings.HasPrefix(p, prefix) {
			delete(s.files[projectID], p)
		}
	}
	return nil
}

func (s *memProjectsStore) ListFiles(ctx context.Context, projectID, dirPath string) ([]persistence.ProjectFile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dirPath = normalizePath(dirPath)
	fileMap := s.files[projectID]
	if fileMap == nil {
		return []persistence.ProjectFile{}, nil
	}

	var result []persistence.ProjectFile

	for _, f := range fileMap {
		parent := parentDir(f.Path)
		if parent == dirPath {
			result = append(result, f)
		}
	}

	// Sort: directories first, then by name
	sort.Slice(result, func(i, j int) bool {
		if result[i].IsDir != result[j].IsDir {
			return result[i].IsDir
		}
		return result[i].Name < result[j].Name
	})

	if result == nil {
		result = []persistence.ProjectFile{}
	}
	return result, nil
}

func (s *memProjectsStore) GetFile(ctx context.Context, projectID, filePath string) (persistence.ProjectFile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	filePath = normalizePath(filePath)
	fileMap := s.files[projectID]
	if fileMap == nil {
		return persistence.ProjectFile{}, persistence.ErrNotFound
	}

	f, ok := fileMap[filePath]
	if !ok {
		return persistence.ProjectFile{}, persistence.ErrNotFound
	}
	return f, nil
}

// parentDir returns the parent directory of a path.
// For paths without "/", returns ".".
func parentDir(p string) string {
	p = normalizePath(p)
	if p == "." {
		return "."
	}
	dir := path.Dir(p)
	if dir == "" || dir == "/" {
		return "."
	}
	return dir
}
