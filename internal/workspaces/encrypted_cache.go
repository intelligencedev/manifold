package workspaces

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	crand "crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"manifold/internal/projects"
)

type EncryptedCacheManager struct {
	root        string
	keyProvider projects.KeyProvider
	dekCache    sync.Map
	dekTTL      time.Duration
}

type cachedDEK struct {
	dek       []byte
	expiresAt time.Time
}

func NewEncryptedCacheManager(root string, kp projects.KeyProvider) (*EncryptedCacheManager, error) {
	if root == "" {
		return nil, errors.New("cache root is required")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create cache root: %w", err)
	}
	return &EncryptedCacheManager{root: filepath.Clean(root), keyProvider: kp, dekTTL: 10 * time.Minute}, nil
}

func (m *EncryptedCacheManager) SetKeyProvider(kp projects.KeyProvider) {
	m.keyProvider = kp
	m.dekCache.Range(func(key, _ any) bool {
		m.dekCache.Delete(key)
		return true
	})
}

func (m *EncryptedCacheManager) projectRoot(tenantID, projectID string) string {
	return filepath.Join(m.root, "users", tenantID, "projects", projectID)
}

func (m *EncryptedCacheManager) filesRoot(tenantID, projectID string) string {
	return filepath.Join(m.projectRoot(tenantID, projectID), "files")
}

func (m *EncryptedCacheManager) metaRoot(tenantID, projectID string) string {
	return filepath.Join(m.projectRoot(tenantID, projectID), ".meta")
}

func (m *EncryptedCacheManager) manifestPath(tenantID, projectID string) string {
	return filepath.Join(m.metaRoot(tenantID, projectID), "cache-manifest.json")
}

func (m *EncryptedCacheManager) wrappedDEKPath(tenantID, projectID string) string {
	return filepath.Join(m.metaRoot(tenantID, projectID), "wrapped-dek.bin")
}

func (m *EncryptedCacheManager) ensureProjectRoots(tenantID, projectID string) error {
	if err := os.MkdirAll(m.filesRoot(tenantID, projectID), 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(m.metaRoot(tenantID, projectID), 0o700); err != nil {
		return err
	}
	return nil
}

func sanitizeRelPath(relPath string) (string, error) {
	clean := filepath.Clean(relPath)
	clean = filepath.ToSlash(clean)
	if clean == "." || clean == "" {
		return "", fmt.Errorf("invalid path")
	}
	if strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "./") || strings.HasPrefix(clean, "/") {
		return "", fmt.Errorf("path escapes root")
	}
	if strings.Contains(clean, "\\") {
		return "", fmt.Errorf("path contains backslash")
	}
	return clean, nil
}

func (m *EncryptedCacheManager) getDEK(ctx context.Context, tenantID, projectID string) ([]byte, error) {
	cacheKey := tenantID + ":" + projectID
	if v, ok := m.dekCache.Load(cacheKey); ok {
		cd := v.(cachedDEK)
		if time.Now().Before(cd.expiresAt) {
			return cd.dek, nil
		}
		m.dekCache.Delete(cacheKey)
	}
	if m.keyProvider == nil {
		return nil, errors.New("key provider not configured for encrypted cache")
	}

	if err := m.ensureProjectRoots(tenantID, projectID); err != nil {
		return nil, fmt.Errorf("ensure cache roots: %w", err)
	}

	wrappedPath := m.wrappedDEKPath(tenantID, projectID)
	var dek []byte
	if data, err := os.ReadFile(wrappedPath); err == nil {
		unwrapped, err := m.keyProvider.UnwrapDEK(ctx, projectID, data)
		if err != nil {
			return nil, fmt.Errorf("unwrap dek: %w", err)
		}
		dek = unwrapped
	} else if errors.Is(err, os.ErrNotExist) {
		fresh := make([]byte, 32)
		if _, err := crand.Read(fresh); err != nil {
			return nil, fmt.Errorf("generate dek: %w", err)
		}
		wrapped, err := m.keyProvider.WrapDEK(ctx, projectID, fresh)
		if err != nil {
			return nil, fmt.Errorf("wrap dek: %w", err)
		}
		if err := os.WriteFile(wrappedPath, wrapped, 0o600); err != nil {
			return nil, fmt.Errorf("persist wrapped dek: %w", err)
		}
		dek = fresh
	} else {
		return nil, fmt.Errorf("load wrapped dek: %w", err)
	}

	ttl := m.dekTTL
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	m.dekCache.Store(cacheKey, cachedDEK{dek: dek, expiresAt: time.Now().Add(ttl)})
	return dek, nil
}

func encrypt(dek, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(crand.Reader, nonce); err != nil {
		return nil, err
	}
	ct := gcm.Seal(nonce, nonce, plaintext, nil)
	return ct, nil
}

func decrypt(dek, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	n := gcm.NonceSize()
	if len(ciphertext) < n {
		return nil, errors.New("ciphertext too short")
	}
	nonce := ciphertext[:n]
	ct := ciphertext[n:]
	return gcm.Open(nil, nonce, ct, nil)
}

func (m *EncryptedCacheManager) WriteFile(ctx context.Context, tenantID, projectID, relPath string, plaintext []byte) error {
	clean, err := sanitizeRelPath(relPath)
	if err != nil {
		return err
	}
	dek, err := m.getDEK(ctx, tenantID, projectID)
	if err != nil {
		return err
	}
	cipher, err := encrypt(dek, plaintext)
	if err != nil {
		return fmt.Errorf("encrypt %s: %w", clean, err)
	}
	if err := m.ensureProjectRoots(tenantID, projectID); err != nil {
		return err
	}
	target := filepath.Join(m.filesRoot(tenantID, projectID), filepath.FromSlash(clean))
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	return os.WriteFile(target, cipher, 0o600)
}

func (m *EncryptedCacheManager) ReadFile(ctx context.Context, tenantID, projectID, relPath string) ([]byte, error) {
	clean, err := sanitizeRelPath(relPath)
	if err != nil {
		return nil, err
	}
	dek, err := m.getDEK(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	target := filepath.Join(m.filesRoot(tenantID, projectID), filepath.FromSlash(clean))
	data, err := os.ReadFile(target)
	if err != nil {
		return nil, err
	}
	pt, err := decrypt(dek, data)
	if err != nil {
		return nil, fmt.Errorf("decrypt %s: %w", clean, err)
	}
	return pt, nil
}

func (m *EncryptedCacheManager) RemoveFile(_ context.Context, tenantID, projectID, relPath string) error {
	clean, err := sanitizeRelPath(relPath)
	if err != nil {
		return err
	}
	target := filepath.Join(m.filesRoot(tenantID, projectID), filepath.FromSlash(clean))
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (m *EncryptedCacheManager) WalkFiles(_ context.Context, tenantID, projectID string, fn func(relPath string, absPath string) error) error {
	root := m.filesRoot(tenantID, projectID)
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		return fn(rel, path)
	})
}

func (m *EncryptedCacheManager) SaveManifest(_ context.Context, tenantID, projectID string, manifest *SyncManifest) error {
	if manifest == nil {
		return errors.New("manifest is required")
	}
	if err := m.ensureProjectRoots(tenantID, projectID); err != nil {
		return err
	}
	manifestPath := m.manifestPath(tenantID, projectID)
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	return os.WriteFile(manifestPath, data, 0o600)
}

func (m *EncryptedCacheManager) LoadManifest(_ context.Context, tenantID, projectID string) (*SyncManifest, error) {
	manifestPath := m.manifestPath(tenantID, projectID)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	var manifest SyncManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("unmarshal manifest: %w", err)
	}
	if manifest.Files == nil {
		manifest.Files = make(map[string]FileManifest)
	}
	return &manifest, nil
}

func (m *EncryptedCacheManager) StoreWorkspace(ctx context.Context, tenantID, projectID, workspacePath string, manifest *SyncManifest) error {
	if err := m.ensureProjectRoots(tenantID, projectID); err != nil {
		return err
	}

	targetManifest := manifest
	if targetManifest == nil {
		targetManifest = &SyncManifest{Version: 1, CheckoutTime: time.Now().UTC(), Files: make(map[string]FileManifest)}
	}
	if targetManifest.Files == nil {
		targetManifest.Files = make(map[string]FileManifest)
	}

	err := filepath.WalkDir(workspacePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".meta" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		rel, err := filepath.Rel(workspacePath, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		clean, err := sanitizeRelPath(rel)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := m.WriteFile(ctx, tenantID, projectID, clean, data); err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		hash, err := hashFile(path)
		if err != nil {
			return err
		}
		targetManifest.Files[clean] = FileManifest{Size: info.Size(), SHA256: hash, LastModified: time.Now().UTC()}
		return nil
	})
	if err != nil {
		return err
	}
	targetManifest.CheckoutTime = time.Now().UTC()
	return m.SaveManifest(ctx, tenantID, projectID, targetManifest)
}
