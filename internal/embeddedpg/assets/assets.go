package assets

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"runtime"
	"strings"
)

// ErrNoRuntimeAsset indicates this binary was built without an embedded
// PostgreSQL runtime payload.
var ErrNoRuntimeAsset = errors.New("embedded postgres runtime asset not present")

//go:embed all:runtimes
var runtimeFS embed.FS

// Current returns the embedded PostgreSQL runtime for the current platform.
func Current() (RuntimeAsset, error) {
	archivePath := strings.TrimSpace(generatedRuntimeArchivePath)
	manifestPath := strings.TrimSpace(generatedRuntimeManifestPath)
	if archivePath == "" || manifestPath == "" {
		return RuntimeAsset{}, ErrNoRuntimeAsset
	}

	manifestBytes, err := fs.ReadFile(runtimeFS, manifestPath)
	if err != nil {
		return RuntimeAsset{}, fmt.Errorf("read embedded runtime manifest: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return RuntimeAsset{}, fmt.Errorf("parse embedded runtime manifest: %w", err)
	}
	if err := validateCurrentManifest(manifest); err != nil {
		return RuntimeAsset{}, err
	}

	archiveBytes, err := fs.ReadFile(runtimeFS, archivePath)
	if err != nil {
		return RuntimeAsset{}, fmt.Errorf("read embedded runtime archive: %w", err)
	}

	return RuntimeAsset{
		RuntimeID:   manifest.RuntimeID,
		OS:          manifest.OS,
		Arch:        manifest.Arch,
		PGMajor:     manifest.Postgres.Major,
		ArchiveName: archivePath,
		Archive:     archiveBytes,
		Manifest:    manifest,
	}, nil
}

func validateCurrentManifest(manifest Manifest) error {
	if manifest.SchemaVersion != 1 {
		return fmt.Errorf("unsupported embedded postgres runtime manifest schema %d", manifest.SchemaVersion)
	}
	if strings.TrimSpace(manifest.RuntimeID) == "" {
		return errors.New("embedded postgres runtime manifest missing runtimeID")
	}
	if manifest.OS != runtime.GOOS || manifest.Arch != runtime.GOARCH {
		return fmt.Errorf("embedded postgres runtime %s targets %s/%s, current platform is %s/%s",
			manifest.RuntimeID, manifest.OS, manifest.Arch, runtime.GOOS, runtime.GOARCH)
	}
	if manifest.Postgres.Major <= 0 || strings.TrimSpace(manifest.Postgres.Version) == "" {
		return fmt.Errorf("embedded postgres runtime %s has invalid postgres version metadata", manifest.RuntimeID)
	}
	if len(manifest.Files) == 0 {
		return fmt.Errorf("embedded postgres runtime %s manifest contains no files", manifest.RuntimeID)
	}
	return nil
}
