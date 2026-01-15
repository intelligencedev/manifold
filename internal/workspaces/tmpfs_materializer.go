//go:build enterprise
// +build enterprise

package workspaces

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type TmpfsMaterializer struct {
	root  string
	cache *EncryptedCacheManager
}

func NewTmpfsMaterializer(root string, cache *EncryptedCacheManager) (*TmpfsMaterializer, error) {
	if root == "" {
		return nil, errors.New("tmpfs root is required")
	}
	if cache == nil {
		return nil, errors.New("encrypted cache is required")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create tmpfs root: %w", err)
	}
	return &TmpfsMaterializer{root: filepath.Clean(root), cache: cache}, nil
}

func (m *TmpfsMaterializer) workspacePath(tenantID, projectID, sessionID string) (string, error) {
	wsPath := filepath.Join(m.root, "users", tenantID, "projects", projectID, "sessions", sessionID)
	absRoot, err := filepath.Abs(m.root)
	if err != nil {
		return "", err
	}
	absWS, err := filepath.Abs(wsPath)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absRoot, absWS)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("workspace path escapes tmpfs root")
	}
	return absWS, nil
}

func (m *TmpfsMaterializer) Materialize(ctx context.Context, tenantID, projectID, sessionID string) (string, *SyncManifest, error) {
	wsPath, err := m.workspacePath(tenantID, projectID, sessionID)
	if err != nil {
		return "", nil, err
	}
	_ = os.RemoveAll(wsPath)
	if err := os.MkdirAll(wsPath, 0o755); err != nil {
		return "", nil, fmt.Errorf("create tmpfs workspace: %w", err)
	}

	manifest, err := m.cache.LoadManifest(ctx, tenantID, projectID)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", nil, err
	}
	if manifest == nil {
		manifest = &SyncManifest{Version: 1, CheckoutTime: time.Now().UTC(), Files: make(map[string]FileManifest)}
	}

	if walkErr := m.cache.WalkFiles(ctx, tenantID, projectID, func(relPath, absPath string) error {
		data, err := m.cache.ReadFile(ctx, tenantID, projectID, relPath)
		if err != nil {
			return err
		}
		dest := filepath.Join(wsPath, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dest, data, 0o644)
	}); walkErr != nil && !errors.Is(walkErr, os.ErrNotExist) {
		return "", nil, walkErr
	}

	if err := writeManifestToWorkspace(wsPath, manifest); err != nil {
		return "", nil, err
	}

	return wsPath, manifest, nil
}

func (m *TmpfsMaterializer) SyncBack(ctx context.Context, tenantID, projectID, sessionID string, manifest *SyncManifest, changedPaths []string) (*SyncManifest, error) {
	if manifest == nil {
		manifest = &SyncManifest{Version: 1, CheckoutTime: time.Now().UTC(), Files: make(map[string]FileManifest)}
	}
	if manifest.Files == nil {
		manifest.Files = make(map[string]FileManifest)
	}

	wsPath, err := m.workspacePath(tenantID, projectID, sessionID)
	if err != nil {
		return manifest, err
	}

	for _, rel := range changedPaths {
		clean, err := sanitizeRelPath(rel)
		if err != nil {
			return manifest, err
		}
		abs := filepath.Join(wsPath, filepath.FromSlash(clean))
		info, err := os.Stat(abs)
		if err != nil {
			if os.IsNotExist(err) {
				_ = m.cache.RemoveFile(ctx, tenantID, projectID, clean)
				delete(manifest.Files, clean)
				continue
			}
			return manifest, err
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			return manifest, err
		}
		if err := m.cache.WriteFile(ctx, tenantID, projectID, clean, data); err != nil {
			return manifest, err
		}
		hash, err := hashFile(abs)
		if err != nil {
			return manifest, err
		}
		manifest.Files[clean] = FileManifest{Size: info.Size(), SHA256: hash, LastModified: time.Now().UTC()}
	}

	manifest.CheckoutTime = time.Now().UTC()
	if err := m.cache.SaveManifest(ctx, tenantID, projectID, manifest); err != nil {
		return manifest, err
	}
	return manifest, nil
}

func (m *TmpfsMaterializer) Unmaterialize(_ context.Context, tenantID, projectID, sessionID string) error {
	wsPath, err := m.workspacePath(tenantID, projectID, sessionID)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(wsPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func writeManifestToWorkspace(basePath string, manifest *SyncManifest) error {
	if manifest == nil {
		return nil
	}
	manifestPath := filepath.Join(basePath, ".meta", "sync-manifest.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	return os.WriteFile(manifestPath, data, 0o644)
}
