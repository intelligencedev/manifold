package projects

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	crand "crypto/rand"
	"encoding/base64"
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
	"manifold/internal/workspaces"
)

// Project describes a per-user project stored on the filesystem.
type Project struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Bytes     int64     `json:"bytes"`
	FileCount int       `json:"fileCount"`
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
	// encryption controls
	encrypt     bool
	keyProvider KeyProvider
	// Legacy: masterKey is kept for backward compatibility when no KeyProvider is set.
	// New deployments should use KeyProvider instead.
	masterKey []byte
}

// NewService creates a new filesystem-backed projects service.
func NewService(workdir string) *Service { return &Service{workdir: workdir} }

// SetKeyProvider configures a KeyProvider for envelope encryption.
// When set, this replaces the legacy masterKey-based encryption.
func (s *Service) SetKeyProvider(kp KeyProvider) {
	s.keyProvider = kp
}

// GetKeyProvider returns the configured KeyProvider, or nil if not set.
func (s *Service) GetKeyProvider() KeyProvider {
	return s.keyProvider
}

// EnableEncryption toggles at-rest encryption for project file I/O.
// If a KeyProvider is configured, it will be used for key management.
// Otherwise, falls back to legacy file-based master key under ${WORKDIR}/.keystore/master.key.
func (s *Service) EnableEncryption(enable bool) error {
	if enable {
		// If KeyProvider is already set, use it
		if s.keyProvider != nil {
			s.encrypt = true
			return nil
		}
		// Legacy fallback: use file-based master key
		if len(s.masterKey) == 0 {
			mk, err := loadOrCreateMasterKey(s.workdir)
			if err != nil {
				return err
			}
			s.masterKey = mk
		}
	}
	s.encrypt = enable
	return nil
}

func (s *Service) userRoot(userID int64) string {
	return filepath.Join(s.workdir, "users", fmt.Sprint(userID), "projects")
}

func (s *Service) projectRoot(userID int64, projectID string) (string, error) {
	cleanPID, err := workspaces.ValidateProjectID(projectID)
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
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (s *Service) metaPath(root string) string {
	return filepath.Join(root, ".meta", "project.json")
}

func (s *Service) encMetaPath(root string) string {
	return filepath.Join(root, ".meta", "enc.json")
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
	meta := projectMeta{ID: id, Name: name, CreatedAt: now, UpdatedAt: now}
	if b, err := json.MarshalIndent(meta, "", "  "); err == nil {
		_ = os.WriteFile(s.metaPath(root), b, 0o644)
	}
	// Initialize envelope encryption metadata when enabled
	if s.encrypt {
		if _, err := s.ensureProjectDEK(userID, id); err != nil {
			return Project{}, err
		}
	}
	// Seed helper files (best-effort)
	_ = os.WriteFile(filepath.Join(root, "README.md"), []byte("# Project\n\nThis directory is managed by the platform.\n"), 0o644)
	return Project{ID: id, Name: name, CreatedAt: now, UpdatedAt: now, Bytes: 0, FileCount: 0}, nil
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
	return Project{ID: m.ID, Name: m.Name, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}, true
}

func (s *Service) writeUpdatedAt(userID int64, projectID string, t time.Time) {
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
	if s.encrypt {
		// Ensure project DEK exists
		dek, err := s.ensureProjectDEK(userID, projectID)
		if err != nil {
			return err
		}
		if err := encryptStreamToFile(f, dek, r); err != nil {
			return err
		}
	} else {
		if _, err := io.Copy(f, r); err != nil {
			return err
		}
	}
	s.writeUpdatedAt(userID, projectID, time.Now().UTC())
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
	s.writeUpdatedAt(userID, projectID, time.Now().UTC())
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
	s.writeUpdatedAt(userID, projectID, time.Now().UTC())
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
	s.writeUpdatedAt(userID, projectID, time.Now().UTC())
	return nil
}

// ReadFile opens a file for reading and returns a reader that yields plaintext
// bytes when encryption is enabled; otherwise returns the raw file contents.
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
	if s.encrypt {
		dek, err := s.getProjectDEK(userID, projectID)
		if err != nil {
			f.Close()
			return nil, err
		}
		rc, err := decryptStreamFromFile(f, dek)
		if err != nil {
			f.Close()
			return nil, err
		}
		// rc owns f and will close it
		return rc, nil
	}
	return f, nil
}

// ----- Encryption helpers -----

// ensureProjectDEK creates a new DEK and writes enc.json if missing; returns the DEK.
// Uses KeyProvider if configured, otherwise falls back to legacy masterKey.
func (s *Service) ensureProjectDEK(userID int64, projectID string) ([]byte, error) {
	root, err := s.projectRoot(userID, projectID)
	if err != nil {
		return nil, err
	}
	encPath := s.encMetaPath(root)

	// Check if DEK already exists
	if _, err := os.Stat(encPath); err == nil {
		return s.getProjectDEK(userID, projectID)
	}

	// Generate new DEK
	dek := make([]byte, 32)
	if _, err := crand.Read(dek); err != nil {
		return nil, err
	}

	// Use KeyProvider if configured
	if s.keyProvider != nil {
		ctx := context.Background()
		wrapped, err := s.keyProvider.WrapDEK(ctx, projectID, dek)
		if err != nil {
			return nil, fmt.Errorf("wrap DEK: %w", err)
		}
		if err := writeWrappedDEKv2(encPath, s.keyProvider.ProviderType(), wrapped); err != nil {
			return nil, err
		}
		return dek, nil
	}

	// Legacy: use masterKey
	if len(s.masterKey) == 0 {
		return nil, fmt.Errorf("encryption master key not initialized")
	}
	if err := writeWrappedDEK(encPath, s.masterKey, dek); err != nil {
		return nil, err
	}
	return dek, nil
}

// getProjectDEK retrieves and unwraps the DEK for a project.
// Uses KeyProvider if configured, otherwise falls back to legacy masterKey.
func (s *Service) getProjectDEK(userID int64, projectID string) ([]byte, error) {
	root, err := s.projectRoot(userID, projectID)
	if err != nil {
		return nil, err
	}
	encPath := s.encMetaPath(root)

	// Read envelope
	b, err := os.ReadFile(encPath)
	if err != nil {
		return nil, err
	}
	var env encEnvelope
	if err := json.Unmarshal(b, &env); err != nil {
		return nil, err
	}

	// Check if this is a v2 envelope (KeyProvider-based)
	if env.WrapVersion == 2 && s.keyProvider != nil {
		wrapped, err := base64.StdEncoding.DecodeString(env.WrappedB64)
		if err != nil {
			return nil, fmt.Errorf("decode wrapped DEK: %w", err)
		}
		ctx := context.Background()
		dek, err := s.keyProvider.UnwrapDEK(ctx, projectID, wrapped)
		if err != nil {
			return nil, fmt.Errorf("unwrap DEK: %w", err)
		}
		return dek, nil
	}

	// Legacy: use masterKey for v1 envelopes
	if len(s.masterKey) == 0 {
		return nil, fmt.Errorf("encryption master key not initialized")
	}
	return readWrappedDEK(encPath, s.masterKey)
}

// local keystore helpers
func keystoreDir(workdir string) string { return filepath.Join(workdir, ".keystore") }

func loadOrCreateMasterKey(workdir string) ([]byte, error) {
	dir := keystoreDir(workdir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "master.key")
	if b, err := os.ReadFile(path); err == nil {
		if len(b) != 32 {
			return nil, fmt.Errorf("invalid master.key length: %d", len(b))
		}
		return b, nil
	}
	b := make([]byte, 32)
	if _, err := crand.Read(b); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return nil, err
	}
	return b, nil
}

// encEnvelope represents the encrypted DEK metadata stored in enc.json.
// WrapVersion=1: Legacy local masterKey-based AES-GCM wrapping (nonce+ciphertext in envelope)
// WrapVersion=2: KeyProvider-based wrapping (opaque wrapped bytes, provider type stored)
type encEnvelope struct {
	Alg         string `json:"alg"`
	WrapVersion int    `json:"wrap_version"`
	NonceB64    string `json:"nonce,omitempty"` // v1 only
	WrappedB64  string `json:"wrapped_dek"`
	// ProviderType identifies the KeyProvider used (v2 only): "file", "vault", "awskms"
	ProviderType string `json:"provider_type,omitempty"`
	// Optional previous wrapped DEK to support in-progress rotation.
	PrevWrappedB64 string `json:"prev_wrapped_dek,omitempty"`
	Active         string `json:"active,omitempty"` // "new" (default) or "prev"
}

// writeWrappedDEKv2 writes a KeyProvider-wrapped DEK to enc.json.
func writeWrappedDEKv2(path, providerType string, wrapped []byte) error {
	env := encEnvelope{
		Alg:          "envelope",
		WrapVersion:  2,
		ProviderType: providerType,
		WrappedB64:   base64.StdEncoding.EncodeToString(wrapped),
	}
	b, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return err
	}
	return nil
}

// writeWrappedDEK writes a legacy v1 masterKey-wrapped DEK to enc.json.
func writeWrappedDEK(path string, master, dek []byte) error {
	block, err := aes.NewCipher(master)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := crand.Read(nonce); err != nil {
		return err
	}
	ct := gcm.Seal(nil, nonce, dek, nil)
	env := encEnvelope{Alg: "AES-256-GCM", WrapVersion: 1, NonceB64: base64.StdEncoding.EncodeToString(nonce), WrappedB64: base64.StdEncoding.EncodeToString(ct)}
	b, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return err
	}
	return nil
}

func readWrappedDEK(path string, master []byte) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var env encEnvelope
	if err := json.Unmarshal(b, &env); err != nil {
		return nil, err
	}
	if env.Alg != "AES-256-GCM" {
		return nil, fmt.Errorf("unsupported alg: %s", env.Alg)
	}
	nonce, err := base64.StdEncoding.DecodeString(env.NonceB64)
	if err != nil {
		return nil, err
	}
	wrapped := env.WrappedB64
	if env.Active == "prev" && env.PrevWrappedB64 != "" {
		wrapped = env.PrevWrappedB64
	}
	ct, err := base64.StdEncoding.DecodeString(wrapped)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(master)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, err
	}
	if len(pt) != 32 {
		return nil, fmt.Errorf("invalid dek length: %d", len(pt))
	}
	return pt, nil
}

// File content encryption format:
// magic[4] = 'M','G','C','M', ver[1]=1, nonce[12], ct[...] (AES-GCM)
var fileMagic = [4]byte{'M', 'G', 'C', 'M'}

func encryptStreamToFile(dst *os.File, dek []byte, r io.Reader) error {
	block, err := aes.NewCipher(dek)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := crand.Read(nonce); err != nil {
		return err
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	ct := gcm.Seal(nil, nonce, data, nil)
	header := append(fileMagic[:], 1)
	header = append(header, nonce...)
	if _, err := dst.Write(header); err != nil {
		return err
	}
	if _, err := dst.Write(ct); err != nil {
		return err
	}
	return nil
}

type readCloser struct {
	io.Reader
	c io.Closer
}

func (rc readCloser) Close() error { return rc.c.Close() }

func decryptStreamFromFile(src *os.File, dek []byte) (io.ReadCloser, error) {
	data, err := io.ReadAll(src)
	if err != nil {
		return nil, err
	}
	if len(data) < 4+1+12 {
		return nil, fmt.Errorf("ciphertext too short")
	}
	if !bytes.Equal(data[:4], fileMagic[:]) {
		return nil, fmt.Errorf("bad magic")
	}
	if data[4] != 1 {
		return nil, fmt.Errorf("unsupported version: %d", int(data[4]))
	}
	nonce := data[5:17]
	ct := data[17:]
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, err
	}
	return readCloser{Reader: bytes.NewReader(pt), c: src}, nil
}

// RotateProjectDEK re-encrypts all files in the project with a new DEK.
// It writes enc.json with both old and new wrapped keys while rotation is in progress,
// and finalizes to only the new wrapped key when done.
// Supports both KeyProvider-based and legacy masterKey-based encryption.
func (s *Service) RotateProjectDEK(ctx context.Context, userID int64, projectID string) error {
	if !s.encrypt {
		return fmt.Errorf("encryption disabled")
	}
	old, err := s.getProjectDEK(userID, projectID)
	if err != nil {
		return err
	}
	// generate new key
	neu := make([]byte, 32)
	if _, err := crand.Read(neu); err != nil {
		return err
	}
	root, err := s.projectRoot(userID, projectID)
	if err != nil {
		return err
	}
	encPath := s.encMetaPath(root)

	// Write enc.json with both keys (active=new)
	if s.keyProvider != nil {
		// KeyProvider-based rotation
		wrappedNew, err := s.keyProvider.WrapDEK(ctx, projectID, neu)
		if err != nil {
			return fmt.Errorf("wrap new DEK: %w", err)
		}
		wrappedOld, err := s.keyProvider.WrapDEK(ctx, projectID, old)
		if err != nil {
			return fmt.Errorf("wrap old DEK: %w", err)
		}
		if err := writeDualWrappedv2(encPath, s.keyProvider.ProviderType(), wrappedOld, wrappedNew); err != nil {
			return err
		}
	} else {
		// Legacy masterKey-based rotation
		if err := writeDualWrapped(encPath, s.masterKey, old, neu); err != nil {
			return err
		}
	}

	// re-encrypt files
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if filepath.Base(p) == ".meta" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		// re-encrypt single file
		return reencryptFile(p, old, neu)
	})
	if err != nil {
		return err
	}

	// finalize: write enc.json with only new wrapped
	if s.keyProvider != nil {
		wrappedNew, err := s.keyProvider.WrapDEK(ctx, projectID, neu)
		if err != nil {
			return fmt.Errorf("wrap new DEK: %w", err)
		}
		if err := writeWrappedDEKv2(encPath, s.keyProvider.ProviderType(), wrappedNew); err != nil {
			return err
		}
	} else {
		if err := writeWrappedDEK(encPath, s.masterKey, neu); err != nil {
			return err
		}
	}
	s.writeUpdatedAt(userID, projectID, time.Now().UTC())
	return nil
}

// writeDualWrappedv2 writes a KeyProvider-wrapped dual-key envelope for rotation.
func writeDualWrappedv2(path, providerType string, wrappedOld, wrappedNew []byte) error {
	env := encEnvelope{
		Alg:            "envelope",
		WrapVersion:    2,
		ProviderType:   providerType,
		WrappedB64:     base64.StdEncoding.EncodeToString(wrappedNew),
		PrevWrappedB64: base64.StdEncoding.EncodeToString(wrappedOld),
		Active:         "new",
	}
	b, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// writeDualWrapped writes a legacy v1 dual-key envelope for rotation.
func writeDualWrapped(path string, master, old, neu []byte) error {
	block, err := aes.NewCipher(master)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := crand.Read(nonce); err != nil {
		return err
	}
	ctNew := gcm.Seal(nil, nonce, neu, nil)
	ctOld := gcm.Seal(nil, nonce, old, nil)
	env := encEnvelope{Alg: "AES-256-GCM", WrapVersion: 1, NonceB64: base64.StdEncoding.EncodeToString(nonce), WrappedB64: base64.StdEncoding.EncodeToString(ctNew), PrevWrappedB64: base64.StdEncoding.EncodeToString(ctOld), Active: "new"}
	b, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func reencryptFile(path string, old, neu []byte) error {
	// Open and read existing file
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	data, err := io.ReadAll(f)
	f.Close()
	if err != nil {
		return err
	}
	var pt []byte
	// If looks like our encrypted format, decrypt with old key; else treat as plaintext
	if len(data) >= 5 && bytes.Equal(data[:4], fileMagic[:]) && data[4] == 1 {
		// emulate decryptStreamFromFile but with buffer
		nonce := data[5:17]
		ct := data[17:]
		block, err := aes.NewCipher(old)
		if err != nil {
			return err
		}
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			return err
		}
		pt, err = gcm.Open(nil, nonce, ct, nil)
		if err != nil {
			return err
		}
	} else {
		pt = data
	}
	// Write to temp file encrypted with new key
	tmp := path + ".rotmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := encryptStreamToFile(out, neu, bytes.NewReader(pt)); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	// Replace atomically
	return os.Rename(tmp, path)
}
