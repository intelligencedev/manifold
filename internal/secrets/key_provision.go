package secrets

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EnsurePersistentKey guarantees a usable MANIFOLD_SECRETS_KEY is available in
// the process environment so database-backed secret storage can initialize on a
// fresh install without a manual setup step.
//
// Precedence:
//  1. If MANIFOLD_SECRETS_KEY is already set (non-empty), it is left untouched
//     and no file is written or read.
//  2. Otherwise the key is loaded from keyPath, generating and persisting a new
//     random 32-byte key (0600) when the file is absent or empty.
//
// The resolved key is exported into the process environment via os.Setenv so
// later callers of NewCodecFromEnv succeed. created reports whether a new key
// file was generated (true only on first-run provisioning).
func EnsurePersistentKey(keyPath string) (created bool, err error) {
	if strings.TrimSpace(os.Getenv(EnvKeyName)) != "" {
		return false, nil
	}
	if strings.TrimSpace(keyPath) == "" {
		return false, fmt.Errorf("secrets key path is empty")
	}

	// Reuse an existing on-disk key when present so encrypted rows stay readable.
	if data, readErr := os.ReadFile(keyPath); readErr == nil {
		if existing := strings.TrimSpace(string(data)); existing != "" {
			if err := os.Setenv(EnvKeyName, existing); err != nil {
				return false, fmt.Errorf("set %s: %w", EnvKeyName, err)
			}
			return false, nil
		}
		// File exists but is empty; fall through and regenerate.
	} else if !os.IsNotExist(readErr) {
		return false, fmt.Errorf("read %s: %w", keyPath, readErr)
	}

	raw := make([]byte, keySize)
	if _, err := rand.Read(raw); err != nil {
		return false, fmt.Errorf("generate secrets key: %w", err)
	}
	encoded := base64.StdEncoding.EncodeToString(raw)

	if dir := filepath.Dir(keyPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return false, fmt.Errorf("create %s: %w", dir, err)
		}
	}
	if err := os.WriteFile(keyPath, []byte(encoded+"\n"), 0o600); err != nil {
		return false, fmt.Errorf("write %s: %w", keyPath, err)
	}
	if err := os.Setenv(EnvKeyName, encoded); err != nil {
		return false, fmt.Errorf("set %s: %w", EnvKeyName, err)
	}
	return true, nil
}
