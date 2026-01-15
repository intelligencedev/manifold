//go:build enterprise
// +build enterprise

package workspaces

import (
	"bytes"
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

// FileDecrypter provides decryption for project files.
// It is used by EphemeralWorkspaceManager to decrypt files downloaded from S3
// when project encryption is enabled.
type FileDecrypter interface {
	// DecryptProjectFile decrypts a file's content for a specific project.
	// Returns the plaintext content or the original data if not encrypted.
	DecryptProjectFile(ctx context.Context, userID int64, projectID string, data []byte) ([]byte, error)
}

// FileEncrypter provides encryption for project files.
type FileEncrypter interface {
	// EncryptProjectFile encrypts a file's content for a specific project.
	// Returns the encrypted content and a flag indicating if encryption was applied.
	EncryptProjectFile(ctx context.Context, userID int64, projectID string, data []byte) ([]byte, bool, error)
}

// EphemeralWorkspaceManager implements WorkspaceManager using ephemeral local
// directories that sync with S3 object storage.
type EphemeralWorkspaceManager struct {
	store     objectstore.ObjectStore
	workdir   string // root for ephemeral workspace dirs
	keyPrefix string // S3 key prefix for project files
	decrypter FileDecrypter
	encrypter FileEncrypter
	mu        sync.RWMutex
	active    map[string]*workspaceState // session -> state
}

type workspaceState struct {
	ws       Workspace
	manifest *SyncManifest
	// generation tracking for reuse
	generation       int64
	skillsGeneration int64
	dirtyPaths       map[string]bool
	lastChangedPaths []string
}

// activeState returns the active workspace state for the provided session key.
// Callers must not mutate the returned state without holding the lock.
func (m *EphemeralWorkspaceManager) activeState(userID int64, projectID, sessionID string) (*workspaceState, bool) {
	key := sessionKey(userID, projectID, sessionID)
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.active[key]
	return s, ok
}

// SyncManifest tracks file state for change detection.
type SyncManifest struct {
	// Version is the manifest format version.
	Version int `json:"version"`
	// CheckoutTime is when the workspace was last checked out from S3.
	CheckoutTime time.Time `json:"checkoutTime"`
	// Generation is the project generation captured at checkout.
	Generation int64 `json:"generation"`
	// SkillsGeneration is the skills subtree generation captured at checkout.
	SkillsGeneration int64 `json:"skillsGeneration"`
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

// SetDecrypter sets the file decrypter for handling encrypted project files.
// This should be called after creating the manager if encryption is enabled.
func (m *EphemeralWorkspaceManager) SetDecrypter(d FileDecrypter) {
	m.decrypter = d
	m.encrypter = nil
	if d == nil {
		return
	}
	if encrypter, ok := d.(FileEncrypter); ok {
		m.encrypter = encrypter
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

func (m *EphemeralWorkspaceManager) projectPrefix(userID int64, projectID string) string {
	base := fmt.Sprintf("users/%d/projects/%s", userID, projectID)
	if m.keyPrefix != "" {
		return strings.TrimSuffix(m.keyPrefix, "/") + "/" + base
	}
	return base
}

func (m *EphemeralWorkspaceManager) metaKey(userID int64, projectID string) string {
	return fmt.Sprintf("%s/.meta/project.json", m.projectPrefix(userID, projectID))
}

type projectMeta struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Generation       int64     `json:"generation"`
	SkillsGeneration int64     `json:"skillsGeneration"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

func (m *EphemeralWorkspaceManager) fetchProjectMeta(ctx context.Context, userID int64, projectID string) (projectMeta, error) {
	metaKey := m.metaKey(userID, projectID)
	reader, _, err := m.store.Get(ctx, metaKey)
	if err != nil {
		return projectMeta{}, err
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return projectMeta{}, err
	}

	var meta projectMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return projectMeta{}, err
	}
	return meta, nil
}

func (m *EphemeralWorkspaceManager) updateProjectMeta(ctx context.Context, userID int64, projectID string, bumpGeneration bool, bumpSkills bool) (projectMeta, error) {
	meta, err := m.fetchProjectMeta(ctx, userID, projectID)
	if err != nil {
		return projectMeta{}, err
	}

	meta.UpdatedAt = time.Now().UTC()
	if bumpGeneration {
		meta.Generation++
	}
	if bumpSkills {
		meta.SkillsGeneration++
	}

	buf, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return projectMeta{}, err
	}

	_, err = m.store.Put(ctx, m.metaKey(userID, projectID), bytes.NewReader(buf), objectstore.PutOptions{ContentType: "application/json"})
	return meta, err
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

	remoteMeta, remoteErr := m.fetchProjectMeta(ctx, userID, cleanPID)

	// Check if workspace is already active
	m.mu.RLock()
	state, ok := m.active[key]
	m.mu.RUnlock()

	if ok {
		// Workspace already active - check if we can reuse it
		if remoteErr != nil || state.manifest == nil {
			// Cannot fetch remote metadata, optimistically reuse existing workspace
			return state.ws, nil
		}
		// Check if cached workspace is still current
		if state.manifest.Generation >= remoteMeta.Generation && state.manifest.SkillsGeneration >= remoteMeta.SkillsGeneration {
			return state.ws, nil
		}
		// Workspace is stale, need to re-hydrate (fall through)
	}

	// Create local workspace directory with inline path containment validation.
	// We use filepath.Rel to verify the path stays under workdir (CodeQL recognizes this pattern).
	localPath := m.workspacePath(userID, cleanPID, cleanSID)
	absWorkdir, err := filepath.Abs(m.workdir)
	if err != nil {
		return Workspace{}, fmt.Errorf("resolve workdir: %w", err)
	}
	absLocalPath, err := filepath.Abs(localPath)
	if err != nil {
		return Workspace{}, fmt.Errorf("resolve workspace path: %w", err)
	}
	relPath, err := filepath.Rel(absWorkdir, absLocalPath)
	if err != nil || relPath == ".." || strings.HasPrefix(relPath, ".."+string(os.PathSeparator)) {
		return Workspace{}, fmt.Errorf("workspace path escapes workdir")
	}
	// Use the validated absolute path for all operations
	safePath := absLocalPath

	if err := os.MkdirAll(safePath, 0o755); err != nil {
		return Workspace{}, fmt.Errorf("create workspace dir: %w", err)
	}

	ws.BaseDir = safePath

	// Download files from S3
	manifest, err := m.hydrate(ctx, userID, cleanPID, safePath, remoteMeta)
	if err != nil {
		// Clean up on failure using the validated safe path
		_ = os.RemoveAll(safePath)
		return Workspace{}, fmt.Errorf("hydrate workspace: %w", err)
	}

	// Track active workspace
	m.mu.Lock()
	m.active[key] = &workspaceState{
		ws:               ws,
		manifest:         manifest,
		generation:       manifest.Generation,
		skillsGeneration: manifest.SkillsGeneration,
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
func (m *EphemeralWorkspaceManager) hydrate(ctx context.Context, userID int64, projectID, localPath string, meta projectMeta) (*SyncManifest, error) {
	// Inline path containment check using filepath.Rel (CodeQL recognizes this pattern).
	absWorkdir, err := filepath.Abs(m.workdir)
	if err != nil {
		return nil, fmt.Errorf("resolve workdir: %w", err)
	}
	absLocalPath, err := filepath.Abs(localPath)
	if err != nil {
		return nil, fmt.Errorf("resolve localPath: %w", err)
	}
	relPath, err := filepath.Rel(absWorkdir, absLocalPath)
	if err != nil || relPath == ".." || strings.HasPrefix(relPath, ".."+string(os.PathSeparator)) {
		return nil, fmt.Errorf("workspace path escapes workdir")
	}

	// If metadata was not provided (e.g., fetch failed), best-effort fetch now.
	if meta.ID == "" {
		if fetched, err := m.fetchProjectMeta(ctx, userID, projectID); err == nil {
			meta = fetched
		}
	}

	prefix := m.s3KeyPrefix(userID, projectID)

	manifest := &SyncManifest{
		Version:          1,
		CheckoutTime:     time.Now().UTC(),
		Generation:       meta.Generation,
		SkillsGeneration: meta.SkillsGeneration,
		Files:            make(map[string]FileManifest),
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

			// Read all data first (needed for potential decryption)
			data, err := io.ReadAll(reader)
			reader.Close()
			if err != nil {
				return nil, fmt.Errorf("read object %s: %w", obj.Key, err)
			}

			// Decrypt if decrypter is available
			if m.decrypter != nil {
				data, err = m.decrypter.DecryptProjectFile(ctx, userID, projectID, data)
				if err != nil {
					return nil, fmt.Errorf("decrypt %s: %w", obj.Key, err)
				}
			}

			// Write decrypted content to local file
			f, err := os.Create(localFile)
			if err != nil {
				return nil, fmt.Errorf("create local file %s: %w", localFile, err)
			}

			hash := sha256.New()
			writer := io.MultiWriter(f, hash)
			_, err = writer.Write(data)
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

// MarkDirty records paths as dirty for a workspace session (best-effort tracking).
func (m *EphemeralWorkspaceManager) MarkDirty(ws Workspace, paths []string) {
	if ws.ProjectID == "" || ws.BaseDir == "" {
		return
	}
	key := sessionKey(ws.UserID, ws.ProjectID, ws.SessionID)
	m.mu.Lock()
	state, ok := m.active[key]
	if ok {
		if state.dirtyPaths == nil {
			state.dirtyPaths = make(map[string]bool)
		}
		for _, p := range paths {
			clean := filepath.ToSlash(strings.TrimSpace(p))
			if clean == "" || clean == "." || clean == "/" {
				continue
			}
			state.dirtyPaths[clean] = true
		}
	}
	m.mu.Unlock()
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
	changedPaths := make(map[string]bool)
	changedList := make([]string, 0)

	// Walk local workspace and upload changes
	err := filepath.WalkDir(ws.BaseDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip internal metadata directory.
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

		changedPaths[relPath] = true
		changedList = append(changedList, relPath)

		// Upload to S3
		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open file %s: %w", relPath, err)
		}
		defer f.Close()

		s3Key := prefix + "/" + relPath
		contentType := detectContentType(relPath)
		reader := io.Reader(f)
		if m.encrypter != nil {
			data, err := io.ReadAll(f)
			if err != nil {
				return fmt.Errorf("read file %s: %w", relPath, err)
			}
			encrypted, didEncrypt, err := m.encrypter.EncryptProjectFile(ctx, ws.UserID, ws.ProjectID, data)
			if err != nil {
				return fmt.Errorf("encrypt file %s: %w", relPath, err)
			}
			if didEncrypt {
				contentType = "application/octet-stream"
			}
			reader = bytes.NewReader(encrypted)
		}
		etag, err := m.store.Put(ctx, s3Key, reader, objectstore.PutOptions{
			ContentType: contentType,
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
				changedPaths[relPath] = true
				changedList = append(changedList, relPath)
				log.Debug().Str("file", relPath).Msg("workspace_file_deleted")
			}
		}
	}

	// Incorporate any explicitly marked dirty paths
	m.mu.RLock()
	if state != nil {
		for p := range state.dirtyPaths {
			changedPaths[p] = true
		}
	}
	m.mu.RUnlock()

	if len(changedPaths) == 0 {
		// No changes detected; clear dirty markers
		m.mu.Lock()
		if s, ok := m.active[key]; ok {
			s.dirtyPaths = nil
		}
		m.mu.Unlock()
		return nil
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

	// Update project metadata (generation counters)
	bumpSkills := false
	for p := range changedPaths {
		if strings.HasPrefix(p, ".manifold/skills") || strings.HasPrefix(p, "manifold/skills") {
			bumpSkills = true
			break
		}
	}
	meta, metaErr := m.updateProjectMeta(ctx, ws.UserID, ws.ProjectID, true, bumpSkills)
	if metaErr != nil {
		log.Warn().Err(metaErr).Msg("workspace_meta_update_failed")
	}
	if bumpSkills {
		if globalSkillsInvalidator != nil {
			globalSkillsInvalidator(ws.ProjectID)
		}
	}

	// Update state
	m.mu.Lock()
	if s, ok := m.active[key]; ok {
		s.manifest = manifest
		if metaErr == nil {
			s.generation = meta.Generation
			s.skillsGeneration = meta.SkillsGeneration
			s.manifest.Generation = meta.Generation
			s.manifest.SkillsGeneration = meta.SkillsGeneration
		}
		s.dirtyPaths = nil
		s.lastChangedPaths = changedList
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

	// Inline path containment check using filepath.Rel (CodeQL recognizes this pattern).
	absWorkdir, err := filepath.Abs(m.workdir)
	if err != nil {
		return fmt.Errorf("resolve workdir: %w", err)
	}
	absBaseDir, err := filepath.Abs(ws.BaseDir)
	if err != nil {
		return fmt.Errorf("resolve baseDir: %w", err)
	}
	relPath, err := filepath.Rel(absWorkdir, absBaseDir)
	if err != nil || relPath == ".." || strings.HasPrefix(relPath, ".."+string(os.PathSeparator)) {
		log.Error().
			Str("baseDir", ws.BaseDir).
			Str("workdir", m.workdir).
			Msg("cleanup_path_outside_workdir")
		return fmt.Errorf("workspace path escapes workdir")
	}

	// Remove the workspace directory using the validated absolute path
	if err := os.RemoveAll(absBaseDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove workspace: %w", err)
	}

	log.Info().
		Int64("userID", ws.UserID).
		Str("projectID", ws.ProjectID).
		Str("sessionID", ws.SessionID).
		Msg("workspace_cleanup_complete")

	return nil
}

// LastChangedPaths returns the last set of changed paths recorded for a workspace session.
func (m *EphemeralWorkspaceManager) LastChangedPaths(ws Workspace) []string {
	if ws.ProjectID == "" || ws.BaseDir == "" {
		return nil
	}
	key := sessionKey(ws.UserID, ws.ProjectID, ws.SessionID)
	m.mu.RLock()
	defer m.mu.RUnlock()
	if s, ok := m.active[key]; ok {
		out := make([]string, len(s.lastChangedPaths))
		copy(out, s.lastChangedPaths)
		return out
	}
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
