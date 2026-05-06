package embeddedpg

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	// defaultExtBaseURL is the default location for pre-built extension packages.
	defaultExtBaseURL = "https://github.com/intelligencedev/manifold/releases/download/pgext-v1"

	// maxDownloadBytes caps the size of a single extension download (500 MB).
	maxDownloadBytes int64 = 500 << 20

	// maxExtractBytes caps the total extracted size across all files (1 GB).
	maxExtractBytes int64 = 1 << 30
)

// defaultExtensions lists extensions installed by default in embedded mode.
var defaultExtensions = []string{"pgvector", "postgis", "pgrouting"}

// extensionCatalog maps extension names to their version per PG major version.
// Versions here must match the pre-built packages hosted at the extension base URL.
var extensionCatalog = map[string]map[int]string{
	"pgvector":  {18: "0.8.2", 17: "0.8.2", 16: "0.8.2", 15: "0.8.2"},
	"postgis":   {18: "3.6.2", 17: "3.6.2", 16: "3.6.2"},
	"pgrouting": {18: "4.0.1", 17: "4.0.1", 16: "4.0.1"},
}

// installExtensions installs the requested extensions into the embedded PG
// binaries directory. Returns a map indicating which extensions were
// successfully installed. Installation is idempotent via stamp files.
func installExtensions(binariesPath, cachePath string, pgMajor int, names []string, baseURL string) map[string]bool {
	if len(names) == 0 {
		return nil
	}
	if baseURL == "" {
		baseURL = defaultExtBaseURL
	}

	installed := make(map[string]bool, len(names))

	for _, name := range names {
		version, ok := extensionVersion(name, pgMajor)
		if !ok {
			log.Warn().Str("extension", name).Int("pgMajor", pgMajor).
				Msg("unknown extension or unsupported PG version; skipping")
			continue
		}

		stamp := filepath.Join(binariesPath, fmt.Sprintf(".ext-%s-%s", name, version))
		if _, err := os.Stat(stamp); err == nil {
			log.Debug().Str("extension", name).Str("version", version).
				Msg("extension already installed")
			installed[name] = true
			continue
		}

		// Primary: download pre-built package.
		if err := acquireExtension(binariesPath, cachePath, name, version, pgMajor, baseURL); err != nil {
			log.Debug().Err(err).Str("extension", name).
				Msg("remote install failed; trying local system discovery")
			// Fallback: discover from local system (Homebrew, apt, etc.).
			if sysErr := installFromSystem(binariesPath, name, pgMajor); sysErr != nil {
				log.Warn().Str("extension", name).
					Str("downloadErr", err.Error()).
					Str("systemErr", sysErr.Error()).
					Msg("extension not available from any source")
				continue
			}
		}

		_ = os.WriteFile(stamp, []byte(version+"\n"), 0644)
		installed[name] = true
		log.Info().Str("extension", name).Str("version", version).
			Msg("extension installed into embedded postgres")
	}

	return installed
}

// extensionVersion returns the catalog version for an extension and PG major.
func extensionVersion(name string, pgMajor int) (string, bool) {
	versions, ok := extensionCatalog[name]
	if !ok {
		return "", false
	}
	v, ok := versions[pgMajor]
	return v, ok
}

// ---------------------------------------------------------------------------
// Remote download path
// ---------------------------------------------------------------------------

// acquireExtension downloads and installs an extension package from the
// configured base URL. Package naming convention:
//
//	{name}-{version}-pg{major}-{os}-{arch}.tar.gz
func acquireExtension(binariesPath, cachePath, name, version string, pgMajor int, baseURL string) error {
	osName, archName := platformIdentifiers()
	filename := fmt.Sprintf("%s-%s-pg%d-%s-%s.tar.gz", name, version, pgMajor, osName, archName)
	extCacheDir := filepath.Join(cachePath, "extensions")
	cachedPath := filepath.Join(extCacheDir, filename)

	if _, err := os.Stat(cachedPath); err != nil {
		downloadURL := strings.TrimRight(baseURL, "/") + "/" + filename
		log.Info().Str("url", downloadURL).Str("extension", name).
			Msg("downloading extension package")
		if err := downloadFile(cachedPath, downloadURL); err != nil {
			return fmt.Errorf("download %s: %w", name, err)
		}
	}

	return extractExtensionPackage(cachedPath, binariesPath)
}

// downloadFile fetches url into destPath atomically using a temp file.
func downloadFile(destPath, url string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url) //nolint:gosec // URL is built from config, not user input
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	tmpFile := destPath + ".tmp"
	out, err := os.Create(tmpFile)
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(out, io.LimitReader(resp.Body, maxDownloadBytes))
	closeErr := out.Close()
	if copyErr != nil {
		os.Remove(tmpFile)
		return copyErr
	}
	if closeErr != nil {
		os.Remove(tmpFile)
		return closeErr
	}

	return os.Rename(tmpFile, destPath)
}

// extractExtensionPackage extracts a tar.gz extension package into the PG
// binaries directory. Expected package layout:
//
//	lib/            → binariesPath/lib/postgresql/   (extension modules)
//	deps/           → binariesPath/lib/              (shared library deps)
//	share/extension → binariesPath/share/postgresql/extension/
func extractExtensionPackage(archivePath, binariesPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip open: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	var totalBytes int64

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read: %w", err)
		}

		// Only process regular files.
		if hdr.Typeflag != tar.TypeReg {
			continue
		}

		totalBytes += hdr.Size
		if totalBytes > maxExtractBytes {
			return fmt.Errorf("extraction exceeds %d byte limit", maxExtractBytes)
		}

		dest, ok := mapExtensionPath(hdr.Name, binariesPath)
		if !ok {
			continue
		}

		// Guard against path traversal.
		cleanDest := filepath.Clean(dest)
		cleanBase := filepath.Clean(binariesPath)
		if !strings.HasPrefix(cleanDest, cleanBase+string(filepath.Separator)) {
			log.Warn().Str("path", hdr.Name).Msg("skipping path outside binaries directory")
			continue
		}

		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return err
		}

		outFile, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode)|0644)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(outFile, io.LimitReader(tr, hdr.Size+1))
		closeErr := outFile.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

// mapExtensionPath maps a path inside the extension tarball to the
// corresponding path under binariesPath. Returns ("", false) for unknown
// paths.
func mapExtensionPath(name, binariesPath string) (string, bool) {
	clean := filepath.Clean(name)
	clean = strings.TrimPrefix(clean, "./")
	clean = strings.TrimPrefix(clean, "/")

	switch {
	case strings.HasPrefix(clean, "deps/"):
		rel := strings.TrimPrefix(clean, "deps/")
		return filepath.Join(binariesPath, "lib", rel), true
	case strings.HasPrefix(clean, "lib/"):
		rel := strings.TrimPrefix(clean, "lib/")
		return filepath.Join(binariesPath, "lib", "postgresql", rel), true
	case strings.HasPrefix(clean, "share/extension/"):
		rel := strings.TrimPrefix(clean, "share/extension/")
		return filepath.Join(binariesPath, "share", "postgresql", "extension", rel), true
	default:
		return "", false
	}
}

// ---------------------------------------------------------------------------
// Local system discovery (fallback)
// ---------------------------------------------------------------------------

// installFromSystem attempts to copy extension files from a system-level
// PostgreSQL installation.
func installFromSystem(binariesPath, name string, pgMajor int) error {
	switch runtime.GOOS {
	case "darwin":
		return installFromHomebrew(binariesPath, name, pgMajor)
	case "linux":
		return installFromLinuxSystem(binariesPath, name, pgMajor)
	default:
		return fmt.Errorf("system discovery not supported on %s", runtime.GOOS)
	}
}

// installFromHomebrew discovers extension files from Homebrew on macOS.
func installFromHomebrew(binariesPath, name string, pgMajor int) error {
	brewPkg := name
	if name == "pgvector" {
		brewPkg = "pgvector"
	}

	// Apple Silicon first, then Intel.
	prefixes := []string{
		fmt.Sprintf("/opt/homebrew/opt/%s", brewPkg),
		fmt.Sprintf("/usr/local/opt/%s", brewPkg),
	}

	var brewPrefix string
	for _, p := range prefixes {
		if _, err := os.Stat(p); err == nil {
			brewPrefix = p
			break
		}
	}
	if brewPrefix == "" {
		return fmt.Errorf("homebrew package %s not found", brewPkg)
	}

	pgSuffix := fmt.Sprintf("postgresql@%d", pgMajor)

	libSrc := filepath.Join(brewPrefix, "lib", pgSuffix)
	libDst := filepath.Join(binariesPath, "lib", "postgresql")
	if err := copyDirContents(libSrc, libDst); err != nil {
		return fmt.Errorf("copy lib files: %w", err)
	}

	shareSrc := filepath.Join(brewPrefix, "share", pgSuffix, "extension")
	shareDst := filepath.Join(binariesPath, "share", "postgresql", "extension")
	if err := copyDirContents(shareSrc, shareDst); err != nil {
		return fmt.Errorf("copy share files: %w", err)
	}

	return nil
}

// installFromLinuxSystem discovers extension files from system packages on
// Debian/Ubuntu/RHEL Linux.
func installFromLinuxSystem(binariesPath, name string, pgMajor int) error {
	libDir := fmt.Sprintf("/usr/lib/postgresql/%d/lib", pgMajor)
	shareDir := fmt.Sprintf("/usr/share/postgresql/%d/extension", pgMajor)

	libDst := filepath.Join(binariesPath, "lib", "postgresql")
	shareDst := filepath.Join(binariesPath, "share", "postgresql", "extension")

	// Copy matching shared library files.
	libFiles := extensionLibNames(name)
	var found bool
	for _, libFile := range libFiles {
		src := filepath.Join(libDir, libFile)
		dst := filepath.Join(libDst, libFile)
		if err := copyFile(src, dst); err == nil {
			found = true
		}
	}

	if !found {
		return fmt.Errorf("no library files found for %s in %s", name, libDir)
	}

	// Copy matching share/extension files.
	prefixes := extensionSharePrefixes(name)
	entries, err := os.ReadDir(shareDir)
	if err != nil {
		return fmt.Errorf("read system share dir: %w", err)
	}

	for _, entry := range entries {
		for _, prefix := range prefixes {
			if strings.HasPrefix(entry.Name(), prefix) {
				src := filepath.Join(shareDir, entry.Name())
				dst := filepath.Join(shareDst, entry.Name())
				_ = copyFile(src, dst)
				break
			}
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// Extension metadata helpers
// ---------------------------------------------------------------------------

// extensionLibNames returns the expected shared library file names.
func extensionLibNames(name string) []string {
	ext := ".so"
	if runtime.GOOS == "darwin" {
		ext = ".dylib"
	}
	switch name {
	case "pgvector":
		return []string{"vector" + ext}
	case "postgis":
		return []string{
			"postgis-3" + ext,
			"postgis_raster-3" + ext,
			"postgis_topology-3" + ext,
			"address_standardizer-3" + ext,
		}
	case "pgrouting":
		return []string{"libpgrouting-4.0" + ext}
	default:
		return []string{name + ext}
	}
}

// extensionSharePrefixes returns filename prefixes used to match .control
// and .sql files in share/postgresql/extension/.
func extensionSharePrefixes(name string) []string {
	switch name {
	case "pgvector":
		return []string{"vector"}
	case "postgis":
		return []string{"postgis", "address_standardizer", "postgis_raster", "postgis_topology"}
	case "pgrouting":
		return []string{"pgrouting"}
	default:
		return []string{name}
	}
}

// platformIdentifiers returns the OS and architecture names used in
// extension package filenames, matching the zonkyio naming convention.
func platformIdentifiers() (osName, archName string) {
	switch runtime.GOOS {
	case "darwin":
		osName = "darwin"
	case "linux":
		osName = "linux"
	case "windows":
		osName = "windows"
	default:
		osName = runtime.GOOS
	}

	switch runtime.GOARCH {
	case "arm64":
		archName = "arm64v8"
	case "amd64":
		archName = "amd64"
	default:
		archName = runtime.GOARCH
	}
	return
}

// ---------------------------------------------------------------------------
// File utilities
// ---------------------------------------------------------------------------

// copyDirContents copies all regular files from src into dst.
func copyDirContents(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if err := copyFile(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

// copyFile copies a single file from src to dst, preserving the executable bit.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
