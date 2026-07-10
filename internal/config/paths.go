package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// EnvConfigPath overrides the resolved configuration file path when set.
	EnvConfigPath = "MANIFOLD_CONFIG"
	// PreferPort is the preferred HTTP listen port for desktop and local installs.
	PreferPort = 32180
)

// DefaultConfigDir returns ~/.manifold (or an error when HOME is unavailable).
func DefaultConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "", fmt.Errorf("home directory is unavailable")
	}
	return filepath.Join(home, ".manifold"), nil
}

// DefaultConfigPath returns the stable desktop config path (~/.manifold/config.yaml).
func DefaultConfigPath() string {
	dir, err := DefaultConfigDir()
	if err != nil {
		return "config.yaml"
	}
	return filepath.Join(dir, "config.yaml")
}

// DefaultLogPath returns the default log file path under ~/.manifold/logs.
func DefaultLogPath() string {
	dir, err := DefaultConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "logs", "manifold.log")
}

// DefaultSecretsKeyPath returns the persisted secrets-key file path
// (~/.manifold/secret.key) used to auto-provision MANIFOLD_SECRETS_KEY on first
// run. Returns "" when the home directory is unavailable.
func DefaultSecretsKeyPath() string {
	dir, err := DefaultConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "secret.key")
}

// EnsureManifoldHome creates ~/.manifold and commonly used subdirectories.
func EnsureManifoldHome() (string, error) {
	dir, err := DefaultConfigDir()
	if err != nil {
		return "", err
	}
	for _, sub := range []string{"", "projects", "logs"} {
		path := dir
		if sub != "" {
			path = filepath.Join(dir, sub)
		}
		if err := os.MkdirAll(path, 0o755); err != nil {
			return "", fmt.Errorf("create %s: %w", path, err)
		}
	}
	return dir, nil
}

// ResolveConfigPath finds the first existing configuration candidate.
// Order:
//  1. MANIFOLD_CONFIG (if set)
//  2. CWD config.yaml / config.yml (dev convenience)
//  3. ~/.manifold/config.yaml / config.yml (desktop stable path)
//
// When no file exists found=false and path is DefaultConfigPath() (write destination).
func ResolveConfigPath() (path string, found bool, err error) {
	override := strings.TrimSpace(os.Getenv(EnvConfigPath))
	if override != "" {
		info, statErr := os.Stat(override)
		if statErr == nil {
			if info.IsDir() {
				return "", false, fmt.Errorf("configuration path must be a file: %s", override)
			}
			return override, true, nil
		}
		if !os.IsNotExist(statErr) {
			return "", false, fmt.Errorf("stat %s: %w", override, statErr)
		}
		// Treat explicit override as the write destination even if missing.
		return override, false, nil
	}

	cwdCandidates := []string{"config.yaml", "config.yml"}
	if homeDir, homeErr := DefaultConfigDir(); homeErr == nil {
		cwdCandidates = append(cwdCandidates,
			filepath.Join(homeDir, "config.yaml"),
			filepath.Join(homeDir, "config.yml"),
		)
	}

	path, found, err = findFirstFile(cwdCandidates...)
	if err != nil {
		return "", false, err
	}
	if found {
		return path, true, nil
	}
	return DefaultConfigPath(), false, nil
}

// ConfigYAMLCandidates lists candidate path strings used by legacy helpers / tests.
func ConfigYAMLCandidates() []string {
	candidates := []string{"config.yaml", "config.yml"}
	if dir, err := DefaultConfigDir(); err == nil {
		candidates = append(candidates,
			filepath.Join(dir, "config.yaml"),
			filepath.Join(dir, "config.yml"),
		)
	}
	return candidates
}
