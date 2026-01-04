package projects

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"manifold/internal/config"
	"manifold/internal/objectstore"
)

// S3Service implements ProjectService using S3-compatible object storage.
type S3Service struct {
	store     objectstore.ObjectStore
	keyPrefix string

	// Project metadata cache for fast listing
	mu       sync.RWMutex
	cache    map[int64][]Project // userID -> projects
	cacheTTL time.Duration
	lastSync map[int64]time.Time
}

// NewS3Service creates a new S3-backed projects service.
func NewS3Service(store objectstore.ObjectStore, cfg config.S3Config) *S3Service {
	prefix := cfg.Prefix
	if prefix == "" {
		prefix = "workspaces"
	}
	return &S3Service{
		store:     store,
		keyPrefix: strings.TrimSuffix(prefix, "/"),
		cache:     make(map[int64][]Project),
		cacheTTL:  5 * time.Minute,
		lastSync:  make(map[int64]time.Time),
	}
}

// userPrefix returns the S3 key prefix for a user's projects.
func (s *S3Service) userPrefix(userID int64) string {
	return fmt.Sprintf("%s/users/%d/projects", s.keyPrefix, userID)
}

// projectPrefix returns the S3 key prefix for a project.
func (s *S3Service) projectPrefix(userID int64, projectID string) string {
	return fmt.Sprintf("%s/%s", s.userPrefix(userID), projectID)
}

// filesPrefix returns the S3 key prefix for a project's files.
func (s *S3Service) filesPrefix(userID int64, projectID string) string {
	return fmt.Sprintf("%s/files", s.projectPrefix(userID, projectID))
}

// metaKey returns the S3 key for a project's metadata.
func (s *S3Service) metaKey(userID int64, projectID string) string {
	return fmt.Sprintf("%s/.meta/project.json", s.projectPrefix(userID, projectID))
}

// CreateProject creates a new project in S3.
func (s *S3Service) CreateProject(ctx context.Context, userID int64, name string) (Project, error) {
	if strings.TrimSpace(name) == "" {
		name = "Untitled"
	}

	id := uuid.NewString()
	now := time.Now().UTC()

	meta := projectMeta{
		ID:        id,
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Store project metadata
	metaBytes, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return Project{}, fmt.Errorf("marshal metadata: %w", err)
	}

	metaKey := s.metaKey(userID, id)
	_, err = s.store.Put(ctx, metaKey, bytes.NewReader(metaBytes), objectstore.PutOptions{
		ContentType: "application/json",
	})
	if err != nil {
		return Project{}, fmt.Errorf("put metadata: %w", err)
	}

	// Create README.md as initial file
	readme := []byte("# Project\n\nThis directory is managed by the platform.\n")
	readmeKey := fmt.Sprintf("%s/README.md", s.filesPrefix(userID, id))
	_, err = s.store.Put(ctx, readmeKey, bytes.NewReader(readme), objectstore.PutOptions{
		ContentType: "text/markdown",
	})
	if err != nil {
		log.Warn().Err(err).Str("projectID", id).Msg("failed to create README.md")
	}

	// Invalidate cache
	s.mu.Lock()
	delete(s.cache, userID)
	s.mu.Unlock()

	return Project{
		ID:        id,
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
		Bytes:     int64(len(readme)),
		FileCount: 1,
	}, nil
}

// DeleteProject removes all objects under the project prefix.
func (s *S3Service) DeleteProject(ctx context.Context, userID int64, projectID string) error {
	prefix := s.projectPrefix(userID, projectID)

	// List and delete all objects under the project
	var continuationToken string
	for {
		result, err := s.store.List(ctx, objectstore.ListOptions{
			Prefix:            prefix + "/",
			MaxKeys:           1000,
			ContinuationToken: continuationToken,
		})
		if err != nil {
			if errors.Is(err, objectstore.ErrNotFound) {
				break
			}
			return fmt.Errorf("list objects: %w", err)
		}

		for _, obj := range result.Objects {
			if err := s.store.Delete(ctx, obj.Key); err != nil {
				log.Warn().Err(err).Str("key", obj.Key).Msg("failed to delete object")
			}
		}

		if !result.IsTruncated {
			break
		}
		continuationToken = result.NextContinuationToken
	}

	// Invalidate cache
	s.mu.Lock()
	delete(s.cache, userID)
	s.mu.Unlock()

	return nil
}

// ListProjects returns all projects for a user.
func (s *S3Service) ListProjects(ctx context.Context, userID int64) ([]Project, error) {
	// Check cache
	s.mu.RLock()
	if projects, ok := s.cache[userID]; ok {
		if time.Since(s.lastSync[userID]) < s.cacheTTL {
			s.mu.RUnlock()
			return projects, nil
		}
	}
	s.mu.RUnlock()

	// List all projects by looking for .meta/project.json files
	prefix := s.userPrefix(userID) + "/"

	projectIDs := make(map[string]bool)
	var continuationToken string

	for {
		result, err := s.store.List(ctx, objectstore.ListOptions{
			Prefix:            prefix,
			Delimiter:         "/",
			MaxKeys:           1000,
			ContinuationToken: continuationToken,
		})
		if err != nil {
			if errors.Is(err, objectstore.ErrNotFound) {
				return []Project{}, nil
			}
			return nil, fmt.Errorf("list projects: %w", err)
		}

		// Extract project IDs from common prefixes
		for _, pfx := range result.CommonPrefixes {
			// prefix format: workspaces/users/123/projects/uuid/
			parts := strings.Split(strings.TrimSuffix(pfx, "/"), "/")
			if len(parts) > 0 {
				projectIDs[parts[len(parts)-1]] = true
			}
		}

		if !result.IsTruncated {
			break
		}
		continuationToken = result.NextContinuationToken
	}

	// Load metadata for each project
	projects := make([]Project, 0, len(projectIDs))
	for pid := range projectIDs {
		p, err := s.loadProject(ctx, userID, pid)
		if err != nil {
			log.Warn().Err(err).Str("projectID", pid).Msg("failed to load project metadata")
			continue
		}
		projects = append(projects, p)
	}

	// Sort by UpdatedAt desc then Name
	sort.Slice(projects, func(i, j int) bool {
		if !projects[i].UpdatedAt.Equal(projects[j].UpdatedAt) {
			return projects[i].UpdatedAt.After(projects[j].UpdatedAt)
		}
		return projects[i].Name < projects[j].Name
	})

	// Update cache
	s.mu.Lock()
	s.cache[userID] = projects
	s.lastSync[userID] = time.Now()
	s.mu.Unlock()

	return projects, nil
}

// loadProject loads a single project's metadata and computes usage.
func (s *S3Service) loadProject(ctx context.Context, userID int64, projectID string) (Project, error) {
	metaKey := s.metaKey(userID, projectID)
	reader, _, err := s.store.Get(ctx, metaKey)
	if err != nil {
		if errors.Is(err, objectstore.ErrNotFound) {
			// Return minimal project info if metadata is missing
			return Project{
				ID:        projectID,
				Name:      projectID,
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			}, nil
		}
		return Project{}, err
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return Project{}, err
	}

	var meta projectMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return Project{}, err
	}

	// Compute usage
	bytes, files := s.computeUsage(ctx, userID, projectID)

	return Project{
		ID:        meta.ID,
		Name:      meta.Name,
		CreatedAt: meta.CreatedAt,
		UpdatedAt: meta.UpdatedAt,
		Bytes:     bytes,
		FileCount: files,
	}, nil
}

// computeUsage calculates total bytes and file count for a project.
func (s *S3Service) computeUsage(ctx context.Context, userID int64, projectID string) (int64, int) {
	prefix := s.filesPrefix(userID, projectID) + "/"
	var totalBytes int64
	var fileCount int

	var continuationToken string
	for {
		result, err := s.store.List(ctx, objectstore.ListOptions{
			Prefix:            prefix,
			MaxKeys:           1000,
			ContinuationToken: continuationToken,
		})
		if err != nil {
			break
		}

		for _, obj := range result.Objects {
			// Skip directory markers
			if strings.HasSuffix(obj.Key, "/") {
				continue
			}
			totalBytes += obj.Size
			fileCount++
		}

		if !result.IsTruncated {
			break
		}
		continuationToken = result.NextContinuationToken
	}

	return totalBytes, fileCount
}

// ListTree lists entries directly under path within a project.
func (s *S3Service) ListTree(ctx context.Context, userID int64, projectID, treePath string) ([]FileEntry, error) {
	// Normalize path
	treePath = strings.TrimSpace(treePath)
	for len(treePath) > 0 && (treePath[0] == '/' || treePath[0] == '\\') {
		treePath = treePath[1:]
	}
	if treePath == "" || treePath == "." {
		treePath = ""
	}

	// Build prefix for listing
	filesPrefix := s.filesPrefix(userID, projectID)
	prefix := filesPrefix + "/"
	if treePath != "" {
		prefix = filesPrefix + "/" + treePath + "/"
	}

	result, err := s.store.List(ctx, objectstore.ListOptions{
		Prefix:    prefix,
		Delimiter: "/",
		MaxKeys:   1000,
	})
	if err != nil {
		if errors.Is(err, objectstore.ErrNotFound) {
			return []FileEntry{}, nil
		}
		return nil, fmt.Errorf("list tree: %w", err)
	}

	entries := make([]FileEntry, 0, len(result.Objects)+len(result.CommonPrefixes))

	// Add directories from common prefixes
	for _, pfx := range result.CommonPrefixes {
		// Extract directory name from prefix
		relPath := strings.TrimPrefix(pfx, filesPrefix+"/")
		relPath = strings.TrimSuffix(relPath, "/")
		name := path.Base(relPath)

		// Skip .meta directory at root
		if treePath == "" && name == ".meta" {
			continue
		}

		entries = append(entries, FileEntry{
			Path:    relPath,
			Name:    name,
			Type:    "dir",
			Size:    0,
			ModTime: time.Now().UTC(),
		})
	}

	// Add files
	for _, obj := range result.Objects {
		// Skip directory markers
		if strings.HasSuffix(obj.Key, "/") {
			continue
		}

		relPath := strings.TrimPrefix(obj.Key, filesPrefix+"/")
		name := path.Base(relPath)

		// Skip .meta files at root
		if treePath == "" && strings.HasPrefix(name, ".meta") {
			continue
		}

		entries = append(entries, FileEntry{
			Path:    relPath,
			Name:    name,
			Type:    "file",
			Size:    obj.Size,
			ModTime: obj.LastModified,
		})
	}

	// Sort: dirs first, then by name
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Type != entries[j].Type {
			return entries[i].Type == "dir"
		}
		return entries[i].Name < entries[j].Name
	})

	return entries, nil
}

// UploadFile writes a file into a project.
func (s *S3Service) UploadFile(ctx context.Context, userID int64, projectID, filePath, name string, r io.Reader) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("empty file name")
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("invalid file name")
	}

	// Normalize path
	filePath = strings.TrimSpace(filePath)
	for len(filePath) > 0 && (filePath[0] == '/' || filePath[0] == '\\') {
		filePath = filePath[1:]
	}
	if filePath == "" || filePath == "." {
		filePath = ""
	}

	// Build S3 key
	var key string
	if filePath == "" {
		key = fmt.Sprintf("%s/%s", s.filesPrefix(userID, projectID), name)
	} else {
		key = fmt.Sprintf("%s/%s/%s", s.filesPrefix(userID, projectID), filePath, name)
	}

	// Detect content type
	contentType := detectContentType(name)

	_, err := s.store.Put(ctx, key, r, objectstore.PutOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("put file: %w", err)
	}

	// Update project metadata
	s.updateProjectTime(ctx, userID, projectID)

	return nil
}

// DeleteFile removes a file or directory from a project.
func (s *S3Service) DeleteFile(ctx context.Context, userID int64, projectID, filePath string) error {
	// Normalize path
	filePath = strings.TrimSpace(filePath)
	for len(filePath) > 0 && (filePath[0] == '/' || filePath[0] == '\\') {
		filePath = filePath[1:]
	}
	if filePath == "" || filePath == "." {
		return fmt.Errorf("invalid path")
	}

	key := fmt.Sprintf("%s/%s", s.filesPrefix(userID, projectID), filePath)

	// Check if this is a directory (has objects with this prefix)
	result, err := s.store.List(ctx, objectstore.ListOptions{
		Prefix:  key + "/",
		MaxKeys: 1,
	})
	if err != nil && !errors.Is(err, objectstore.ErrNotFound) {
		return fmt.Errorf("check path: %w", err)
	}

	if len(result.Objects) > 0 || len(result.CommonPrefixes) > 0 {
		// It's a directory, delete all contents recursively
		var continuationToken string
		for {
			result, err := s.store.List(ctx, objectstore.ListOptions{
				Prefix:            key + "/",
				MaxKeys:           1000,
				ContinuationToken: continuationToken,
			})
			if err != nil {
				break
			}

			for _, obj := range result.Objects {
				if err := s.store.Delete(ctx, obj.Key); err != nil {
					log.Warn().Err(err).Str("key", obj.Key).Msg("failed to delete object")
				}
			}

			if !result.IsTruncated {
				break
			}
			continuationToken = result.NextContinuationToken
		}
	} else {
		// It's a file, delete directly
		if err := s.store.Delete(ctx, key); err != nil && !errors.Is(err, objectstore.ErrNotFound) {
			return fmt.Errorf("delete file: %w", err)
		}
	}

	// Update project metadata
	s.updateProjectTime(ctx, userID, projectID)

	return nil
}

// MovePath relocates a file or directory within a project.
func (s *S3Service) MovePath(ctx context.Context, userID int64, projectID, from, to string) error {
	// Normalize paths
	from = normalizePath(from)
	to = normalizePath(to)

	if from == "" || from == "." {
		return fmt.Errorf("invalid source path")
	}
	if to == "" || to == "." {
		return fmt.Errorf("invalid destination path")
	}

	// Check if destination exists
	toKey := fmt.Sprintf("%s/%s", s.filesPrefix(userID, projectID), to)
	exists, err := s.store.Exists(ctx, toKey)
	if err != nil {
		return fmt.Errorf("check destination: %w", err)
	}
	if exists {
		return fmt.Errorf("destination exists")
	}

	fromKey := fmt.Sprintf("%s/%s", s.filesPrefix(userID, projectID), from)

	// Check if source is a directory
	result, err := s.store.List(ctx, objectstore.ListOptions{
		Prefix:  fromKey + "/",
		MaxKeys: 1,
	})
	if err != nil && !errors.Is(err, objectstore.ErrNotFound) {
		return fmt.Errorf("check source: %w", err)
	}

	if len(result.Objects) > 0 || len(result.CommonPrefixes) > 0 {
		// Moving a directory - copy all contents
		var continuationToken string
		for {
			result, err := s.store.List(ctx, objectstore.ListOptions{
				Prefix:            fromKey + "/",
				MaxKeys:           1000,
				ContinuationToken: continuationToken,
			})
			if err != nil {
				break
			}

			for _, obj := range result.Objects {
				relPath := strings.TrimPrefix(obj.Key, fromKey+"/")
				newKey := toKey + "/" + relPath

				if err := s.store.Copy(ctx, obj.Key, newKey); err != nil {
					return fmt.Errorf("copy %s: %w", obj.Key, err)
				}
				if err := s.store.Delete(ctx, obj.Key); err != nil {
					log.Warn().Err(err).Str("key", obj.Key).Msg("failed to delete source after copy")
				}
			}

			if !result.IsTruncated {
				break
			}
			continuationToken = result.NextContinuationToken
		}
	} else {
		// Moving a single file
		if err := s.store.Copy(ctx, fromKey, toKey); err != nil {
			return fmt.Errorf("copy file: %w", err)
		}
		if err := s.store.Delete(ctx, fromKey); err != nil {
			log.Warn().Err(err).Str("key", fromKey).Msg("failed to delete source after copy")
		}
	}

	// Update project metadata
	s.updateProjectTime(ctx, userID, projectID)

	return nil
}

// CreateDir creates a "directory" in S3 by creating a placeholder object.
func (s *S3Service) CreateDir(ctx context.Context, userID int64, projectID, dirPath string) error {
	// Normalize path
	dirPath = normalizePath(dirPath)
	if dirPath == "" || dirPath == "." {
		return nil // Root already exists
	}

	// In S3, directories are implicit. We create a placeholder with trailing slash.
	key := fmt.Sprintf("%s/%s/", s.filesPrefix(userID, projectID), dirPath)
	_, err := s.store.Put(ctx, key, bytes.NewReader(nil), objectstore.PutOptions{})
	if err != nil {
		return fmt.Errorf("create dir marker: %w", err)
	}

	// Update project metadata
	s.updateProjectTime(ctx, userID, projectID)

	return nil
}

// ReadFile opens a file for reading.
func (s *S3Service) ReadFile(ctx context.Context, userID int64, projectID, filePath string) (io.ReadCloser, error) {
	// Normalize path
	filePath = normalizePath(filePath)
	if filePath == "" || filePath == "." {
		return nil, fmt.Errorf("invalid path")
	}

	key := fmt.Sprintf("%s/%s", s.filesPrefix(userID, projectID), filePath)
	reader, _, err := s.store.Get(ctx, key)
	if err != nil {
		if errors.Is(err, objectstore.ErrNotFound) {
			return nil, fmt.Errorf("file not found: %s", filePath)
		}
		return nil, fmt.Errorf("get file: %w", err)
	}

	return reader, nil
}

// EnableEncryption is not supported for S3 backend (use S3 SSE instead).
func (s *S3Service) EnableEncryption(enable bool) error {
	if enable {
		log.Warn().Msg("s3_projects: application-level encryption not supported, use S3 SSE instead")
	}
	return nil
}

// updateProjectTime updates the project's UpdatedAt timestamp.
func (s *S3Service) updateProjectTime(ctx context.Context, userID int64, projectID string) {
	metaKey := s.metaKey(userID, projectID)

	// Read current metadata
	reader, _, err := s.store.Get(ctx, metaKey)
	if err != nil {
		return
	}

	data, err := io.ReadAll(reader)
	reader.Close()
	if err != nil {
		return
	}

	var meta projectMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return
	}

	// Update timestamp
	meta.UpdatedAt = time.Now().UTC()
	newData, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return
	}

	_, _ = s.store.Put(ctx, metaKey, bytes.NewReader(newData), objectstore.PutOptions{
		ContentType: "application/json",
	})

	// Invalidate cache
	s.mu.Lock()
	delete(s.cache, userID)
	s.mu.Unlock()
}

// normalizePath cleans up a path for S3 usage.
func normalizePath(p string) string {
	p = strings.TrimSpace(p)
	for len(p) > 0 && (p[0] == '/' || p[0] == '\\') {
		p = p[1:]
	}
	if p == "" || p == "." {
		return ""
	}
	return p
}

// detectContentType guesses MIME type from file extension.
func detectContentType(name string) string {
	ext := strings.ToLower(path.Ext(name))
	switch ext {
	case ".json":
		return "application/json"
	case ".txt":
		return "text/plain"
	case ".html", ".htm":
		return "text/html"
	case ".css":
		return "text/css"
	case ".js":
		return "application/javascript"
	case ".md":
		return "text/markdown"
	case ".yaml", ".yml":
		return "text/yaml"
	case ".go":
		return "text/x-go"
	case ".py":
		return "text/x-python"
	case ".rs":
		return "text/x-rust"
	case ".ts":
		return "text/typescript"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".pdf":
		return "application/pdf"
	case ".zip":
		return "application/zip"
	default:
		return "application/octet-stream"
	}
}

// Ensure S3Service implements ProjectService.
var _ ProjectService = (*S3Service)(nil)
