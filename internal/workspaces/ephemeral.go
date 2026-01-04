package workspaces

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"manifold/internal/config"
	"manifold/internal/objectstore"
)

// EphemeralWorkspaceManager implements WorkspaceManager using ephemeral local
// directories that sync with S3 object storage.
type EphemeralWorkspaceManager struct {
	store     objectstore.ObjectStore
	workdir   string // root for ephemeral workspace dirs
	keyPrefix string // S3 key prefix for project files
	mu        sync.RWMutex
	active    map[string]*workspaceState // session -> state
}

type workspaceState struct {
	ws       Workspace
	manifest *SyncManifest
}

// SyncManifest tracks file state for change detection.
type SyncManifest struct {
	// Version is the manifest format version.
	Version int `json:"version"`
	// CheckoutTime is when the workspace was last checked out from S3.
	CheckoutTime time.Time `json:"checkoutTime"`
	// Files maps relative paths to file metadata.
	Files map[string]FileManifest `json:"files"`
}

// FileManifest contains metadata about a single file.
type FileManifest struct {
	// Size is the file size in bytes.
	Size int64 `json:"size"`
	// SHA256 is the hex-encoded SHA256 hash of the file contents.
	SHA256 string `json:"sha256"`
	// ETag is the S3 ETag from the last sync.
	ETag string `json:"etag"`
	// LastModified is the S3 last modified time.
	LastModified time.Time `json:"lastModified"`
}

// NewEphemeralManager creates an EphemeralWorkspaceManager.
func NewEphemeralManager(store objectstore.ObjectStore, cfg *config.Config) *EphemeralWorkspaceManager {
	root := cfg.Projects.Workspace.Root
	if root == "" {
		root = filepath.Join(cfg.Workdir, "sandboxes")
	}

	return &EphemeralWorkspaceManager{
		store:     store,
		workdir:   root,
		keyPrefix: cfg.Projects.S3.Prefix,
		active:    make(map[string]*workspaceState),
	}
}

// Mode returns "ephemeral".
func (m *EphemeralWorkspaceManager) Mode() string {
	return "ephemeral"
}

// sessionKey creates a unique key for tracking active sessions.
func sessionKey(userID int64, projectID, sessionID string) string {
	return fmt.Sprintf("%d:%s:%s", userID, projectID, sessionID)
}

// workspacePath builds the local ephemeral workspace path.
// It ensures that the resulting path is confined within m.workdir.
func (m *EphemeralWorkspaceManager) workspacePath(userID int64, projectID, sessionID string) string {
	// Normalize the configured workdir to an absolute path.
	absRoot, err := filepath.Abs(filepath.Clean(m.workdir))
	if err != nil {
		// On unexpected failure, fall back to the raw workdir.
		absRoot = m.workdir
	}

	// Build the base directory for this user's project sessions.
	baseRoot := filepath.Join(absRoot, "users", fmt.Sprint(userID), "projects", projectID, "sessions")

	// Append the sessionID as the final path element.
	wsPath := filepath.Join(baseRoot, sessionID)

	// Resolve the workspace path to an absolute path and ensure it stays under absRoot.
	absWS, err := filepath.Abs(wsPath)
	if err != nil {
		// On error, return the root to avoid pointing outside the workspace tree.
		return absRoot
	}

	rootWithSep := absRoot
	if !strings.HasSuffix(rootWithSep, string(os.PathSeparator)) {
		rootWithSep += string(os.PathSeparator)
	}

	if !strings.HasPrefix(absWS, rootWithSep) && absWS != absRoot {
		// If, despite prior validation, the path escapes the root, log and fall back.
		log.Error().
			Str("workspacePath", absWS).
			Str("workdir", absRoot).
			Msg("workspacePath computed path outside workdir; falling back to workdir")
		return absRoot
	}

	return absWS
}

// s3KeyPrefix returns the S3 key prefix for a project's files.
func (m *EphemeralWorkspaceManager) s3KeyPrefix(userID int64, projectID string) string {
	base := fmt.Sprintf("users/%d/projects/%s/files", userID, projectID)
	if m.keyPrefix != "" {
		return m.keyPrefix + "/" + base
	}
	return base
}

// Checkout creates an ephemeral workspace by downloading files from S3.
func (m *EphemeralWorkspaceManager) Checkout(ctx context.Context, userID int64, projectID, sessionID string) (Workspace, error) {
	ws := Workspace{
		UserID:    userID,
		ProjectID: projectID,
		SessionID: sessionID,
		Mode:      "ephemeral",
	}

	// Empty project ID means no workspace scoping
	if projectID == "" {
		return ws, nil
	}

	// Validate project ID format
	cleanPID, err := ValidateProjectID(projectID)
	if err != nil {
		return Workspace{}, err
	}
	ws.ProjectID = cleanPID

	// Generate session ID if empty
	if sessionID == "" {
		sessionID = fmt.Sprintf("ses-%d", time.Now().UnixNano())
	}

	cleanSID, err := ValidateSessionID(sessionID)
	if err != nil {
		return Workspace{}, err
	}
	ws.SessionID = cleanSID

	key := sessionKey(userID, cleanPID, cleanSID)

	// Check if workspace is already active
	m.mu.RLock()
	if state, ok := m.active[key]; ok {
		m.mu.RUnlock()
		return state.ws, nil
	}
	m.mu.RUnlock()

	// Create local workspace directory
	localPath := m.workspacePath(userID, cleanPID, cleanSID)
	if err := ensurePathWithin(m.workdir, localPath); err != nil {
		return Workspace{}, err
	}
	if err := os.MkdirAll(localPath, 0o755); err != nil {
		return Workspace{}, fmt.Errorf("create workspace dir: %w", err)
	}

	ws.BaseDir = localPath

	// Download files from S3
	manifest, err := m.hydrate(ctx, userID, cleanPID, localPath)
	if err != nil {
		// Clean up on failure (defense-in-depth: only delete if still under workdir)
		if ensurePathWithin(m.workdir, localPath) == nil {
			_ = os.RemoveAll(localPath)
		}
		return Workspace{}, fmt.Errorf("hydrate workspace: %w", err)
	}

	// Track active workspace
	m.mu.Lock()
	m.active[key] = &workspaceState{
		ws:       ws,
		manifest: manifest,
	}
	m.mu.Unlock()

	log.Info().
		Int64("userID", userID).
		Str("projectID", cleanPID).
		Str("sessionID", cleanSID).
		Str("baseDir", localPath).
		Int("files", len(manifest.Files)).
		Msg("workspace_checkout_complete")

	return ws, nil
}

// hydrate downloads all project files from S3 to the local workspace.
func (m *EphemeralWorkspaceManager) hydrate(ctx context.Context, userID int64, projectID, localPath string) (*SyncManifest, error) {
	// Defense-in-depth: ensure the provided localPath is within the configured workdir
	if err := ensurePathWithin(m.workdir, localPath); err != nil {
		return nil, fmt.Errorf("invalid workspace path: %w", err)
	}

	prefix := m.s3KeyPrefix(userID, projectID)

	manifest := &SyncManifest{
		Version:      1,
		CheckoutTime: time.Now().UTC(),
		Files:        make(map[string]FileManifest),
	}

	absLocalPath, err := filepath.Abs(localPath)
	if err != nil {
		return nil, fmt.Errorf("resolve localPath: %w", err)
	}

	// List all objects under the project prefix
	var continuationToken string
	for {
		result, err := m.store.List(ctx, objectstore.ListOptions{
			Prefix:            prefix + "/",
			MaxKeys:           1000,
			ContinuationToken: continuationToken,
		})
		if err != nil {
			if errors.Is(err, objectstore.ErrNotFound) {
				// No files yet - that's OK for a new project
				break
			}
			return nil, fmt.Errorf("list objects: %w", err)
		}

		// Download each object
		for _, obj := range result.Objects {
			// Skip directory markers
			if strings.HasSuffix(obj.Key, "/") {
				continue
			}

			// Extract relative path from the key
			relPathRaw := strings.TrimPrefix(obj.Key, prefix+"/")
			if relPathRaw == "" || relPathRaw == obj.Key {
				continue
			}
			relPath, err := sanitizeObjectRelPath(relPathRaw)
			if err != nil {
				return nil, fmt.Errorf("invalid object key %q: %w", obj.Key, err)
			}

			localFile, err := safeJoinUnder(absLocalPath, relPath)
			if err != nil {
				return nil, fmt.Errorf("invalid relPath %q: %w", relPath, err)
			}

			// Create parent directories
			if err := os.MkdirAll(filepath.Dir(localFile), 0o755); err != nil {
				return nil, fmt.Errorf("create parent dir for %s: %w", relPath, err)
			}

			// Download the file
			reader, attrs, err := m.store.Get(ctx, obj.Key)
			if err != nil {
				return nil, fmt.Errorf("get object %s: %w", obj.Key, err)
			}

			f, err := os.Create(localFile)
			if err != nil {
				reader.Close()
				return nil, fmt.Errorf("create local file %s: %w", localFile, err)
			}

			hash := sha256.New()
			writer := io.MultiWriter(f, hash)
			_, err = io.Copy(writer, reader)
			reader.Close()
			f.Close()

			if err != nil {
				return nil, fmt.Errorf("copy object %s: %w", obj.Key, err)
			}

			// Record in manifest
			manifest.Files[relPath] = FileManifest{
				Size:         attrs.Size,
				SHA256:       hex.EncodeToString(hash.Sum(nil)),
				ETag:         attrs.ETag,
				LastModified: attrs.LastModified,
			}
		}

		if !result.IsTruncated {
			break
		}
		continuationToken = result.NextContinuationToken
	}

	// Write manifest to workspace (use absLocalPath for normalized path)
	manifestPath := filepath.Join(absLocalPath, ".meta", "sync-manifest.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		return nil, fmt.Errorf("create .meta dir: %w", err)
	}

	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal manifest: %w", err)
	}

	if err := os.WriteFile(manifestPath, manifestData, 0o644); err != nil {
		return nil, fmt.Errorf("write manifest: %w", err)
	}

	return manifest, nil
}

func ensurePathWithin(base, candidate string) error {
	absBase, err := filepath.Abs(base)
	if err != nil {
		return fmt.Errorf("resolve base: %w", err)
	}
	absCandidate, err := filepath.Abs(candidate)
	if err != nil {
		return fmt.Errorf("resolve candidate: %w", err)
	}
	rel, err := filepath.Rel(absBase, absCandidate)
	if err != nil {
		return fmt.Errorf("resolve relative: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("invalid path")
	}
	return nil
}

func sanitizeObjectRelPath(relPath string) (string, error) {
	p := strings.TrimSpace(relPath)
	if p == "" {
		return "", fmt.Errorf("empty path")
	}
	// Reject any backslashes to avoid platform-specific traversal surprises.
	if strings.Contains(p, "\\") {
		return "", fmt.Errorf("invalid path")
	}
	// Clean using slash semantics (S3 keys are slash-delimited).
	clean := path.Clean(p)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return "", fmt.Errorf("invalid path")
	}
	return clean, nil
}

func safeJoinUnder(absRoot, relSlashPath string) (string, error) {
	localFile := filepath.Join(absRoot, filepath.FromSlash(relSlashPath))
	absLocalFile, err := filepath.Abs(localFile)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absRoot, absLocalFile)
	if err != nil {
		return "", err
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("invalid path")
	}
	return absLocalFile, nil
}

// Commit uploads changed files from the workspace back to S3.
func (m *EphemeralWorkspaceManager) Commit(ctx context.Context, ws Workspace) error {
	if ws.ProjectID == "" || ws.BaseDir == "" {
		return nil
	}

	key := sessionKey(ws.UserID, ws.ProjectID, ws.SessionID)

	m.mu.RLock()
	state, ok := m.active[key]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("workspace not found")
	}

	// Load manifest (may have been updated)
	manifest := state.manifest
	if manifest == nil {
		manifest = &SyncManifest{
			Version: 1,
			Files:   make(map[string]FileManifest),
		}
	}

	prefix := m.s3KeyPrefix(ws.UserID, ws.ProjectID)

	// Track files seen during walk for deletion detection
	seenFiles := make(map[string]bool)

	// Walk local workspace and upload changes
	err := filepath.WalkDir(ws.BaseDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip .meta directory
		if d.IsDir() && d.Name() == ".meta" {
			return filepath.SkipDir
		}

		if d.IsDir() {
			return nil
		}

		// Skip symlinks
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(ws.BaseDir, path)
		if err != nil {
			return err
		}
		relPath = filepath.ToSlash(relPath)
		seenFiles[relPath] = true

		// Check if file changed
		info, err := d.Info()
		if err != nil {
			return err
		}

		// Compute current hash
		currentHash, err := hashFile(path)
		if err != nil {
			return fmt.Errorf("hash file %s: %w", relPath, err)
		}

		// Check manifest for previous state
		prev, existed := manifest.Files[relPath]
		if existed && prev.SHA256 == currentHash {
			// No change
			return nil
		}

		// Upload to S3
		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open file %s: %w", relPath, err)
		}
		defer f.Close()

		s3Key := prefix + "/" + relPath
		etag, err := m.store.Put(ctx, s3Key, f, objectstore.PutOptions{
			ContentType: detectContentType(relPath),
		})
		if err != nil {
			return fmt.Errorf("upload %s: %w", relPath, err)
		}

		// Update manifest
		manifest.Files[relPath] = FileManifest{
			Size:         info.Size(),
			SHA256:       currentHash,
			ETag:         etag,
			LastModified: time.Now().UTC(),
		}

		log.Debug().
			Str("file", relPath).
			Bool("new", !existed).
			Msg("workspace_file_uploaded")

		return nil
	})

	if err != nil {
		return fmt.Errorf("walk workspace: %w", err)
	}

	// Delete files that were removed locally
	for relPath := range manifest.Files {
		if !seenFiles[relPath] {
			s3Key := prefix + "/" + relPath
			if err := m.store.Delete(ctx, s3Key); err != nil {
				log.Warn().Err(err).Str("key", s3Key).Msg("failed_to_delete_s3_object")
			} else {
				delete(manifest.Files, relPath)
				log.Debug().Str("file", relPath).Msg("workspace_file_deleted")
			}
		}
	}

	// Update manifest
	manifest.CheckoutTime = time.Now().UTC()
	manifestPath := filepath.Join(ws.BaseDir, ".meta", "sync-manifest.json")
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	if err := os.WriteFile(manifestPath, manifestData, 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	// Update state
	m.mu.Lock()
	if s, ok := m.active[key]; ok {
		s.manifest = manifest
	}
	m.mu.Unlock()

	log.Info().
		Int64("userID", ws.UserID).
		Str("projectID", ws.ProjectID).
		Str("sessionID", ws.SessionID).
		Int("files", len(manifest.Files)).
		Msg("workspace_commit_complete")

	return nil
}

// Cleanup removes the ephemeral workspace from disk.
func (m *EphemeralWorkspaceManager) Cleanup(ctx context.Context, ws Workspace) error {
	if ws.ProjectID == "" || ws.BaseDir == "" {
		return nil
	}

	key := sessionKey(ws.UserID, ws.ProjectID, ws.SessionID)

	m.mu.Lock()
	delete(m.active, key)
	m.mu.Unlock()

	// Defense-in-depth: only remove if path is within workdir
	if err := ensurePathWithin(m.workdir, ws.BaseDir); err != nil {
		log.Error().
			Err(err).
			Str("baseDir", ws.BaseDir).
			Str("workdir", m.workdir).
			Msg("cleanup_path_outside_workdir")
		return fmt.Errorf("invalid workspace path: %w", err)
	}

	// Remove the workspace directory
	if err := os.RemoveAll(ws.BaseDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove workspace: %w", err)
	}

	log.Info().
		Int64("userID", ws.UserID).
		Str("projectID", ws.ProjectID).
		Str("sessionID", ws.SessionID).
		Msg("workspace_cleanup_complete")

	return nil
}

// hashFile computes the SHA256 hash of a file.
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// detectContentType guesses MIME type from file extension.
func detectContentType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
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
