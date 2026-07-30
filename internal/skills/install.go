package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// bundledManifestName tracks which skills were installed from the embedded
// bundle and the content hash they were installed at, so updates only rewrite
// changed skills and user-authored skills are never touched.
const bundledManifestName = ".manifold-bundled.json"

// InstallBundledSkills copies bundled skills from `bundled` (an FS whose root
// contains one directory per skill) into destRoot. A skill's directory is
// overwritten only when its bundled content has changed since the last install
// or the destination is missing. Skills that were previously installed from the
// bundle but are no longer present are removed. Skills in destRoot that the
// bundle never managed (user-authored) are left untouched. It returns the names
// of bundled skills present after installation.
func InstallBundledSkills(bundled fs.FS, destRoot string) ([]string, error) {
	entries, err := fs.ReadDir(bundled, ".")
	if err != nil {
		return nil, fmt.Errorf("read bundled skills: %w", err)
	}
	if err := os.MkdirAll(destRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create skills dir %s: %w", destRoot, err)
	}

	prev := loadBundledManifest(destRoot)
	next := map[string]string{}
	var installed []string

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		hash, err := hashBundledSkill(bundled, name)
		if err != nil {
			return nil, fmt.Errorf("hash bundled skill %s: %w", name, err)
		}
		destDir := filepath.Join(destRoot, name)
		_, statErr := os.Stat(destDir)
		if prev[name] != hash || statErr != nil {
			if err := os.RemoveAll(destDir); err != nil {
				return nil, fmt.Errorf("remove %s: %w", destDir, err)
			}
			if err := copyBundledSkill(bundled, name, destDir); err != nil {
				return nil, fmt.Errorf("install bundled skill %s: %w", name, err)
			}
		}
		next[name] = hash
		installed = append(installed, name)
	}

	// Remove skills that were bundle-managed but dropped from a later release.
	for name := range prev {
		if _, ok := next[name]; ok {
			continue
		}
		_ = os.RemoveAll(filepath.Join(destRoot, name))
	}

	if err := saveBundledManifest(destRoot, next); err != nil {
		return installed, fmt.Errorf("write bundled manifest: %w", err)
	}
	sort.Strings(installed)
	return installed, nil
}

// hashBundledSkill returns a stable content hash over all files in the bundled
// skill's subtree (paths + bytes), so any change to any file triggers a
// reinstall.
func hashBundledSkill(bundled fs.FS, name string) (string, error) {
	type entry struct {
		path string
		data []byte
	}
	var files []entry
	err := fs.WalkDir(bundled, name, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(bundled, p)
		if err != nil {
			return err
		}
		files = append(files, entry{path: p, data: data})
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })
	h := sha256.New()
	for _, f := range files {
		h.Write([]byte(f.path))
		h.Write([]byte{0})
		h.Write(f.data)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// copyBundledSkill writes the bundled skill subtree to destDir.
func copyBundledSkill(bundled fs.FS, name, destDir string) error {
	return fs.WalkDir(bundled, name, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(p, name), "/")
		target := destDir
		if rel != "" {
			target = filepath.Join(destDir, filepath.FromSlash(rel))
		}
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fs.ReadFile(bundled, p)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		mode := fs.FileMode(0o644)
		if path.Ext(p) == ".sh" {
			mode = 0o755
		}
		return os.WriteFile(target, data, mode)
	})
}

func loadBundledManifest(destRoot string) map[string]string {
	data, err := os.ReadFile(filepath.Join(destRoot, bundledManifestName))
	if err != nil {
		return map[string]string{}
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil || m == nil {
		return map[string]string{}
	}
	return m
}

func saveBundledManifest(destRoot string, m map[string]string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(destRoot, bundledManifestName), data, 0o644)
}
