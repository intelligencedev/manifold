package secrets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsurePersistentKeyGeneratesWhenMissing(t *testing.T) {
	t.Setenv(EnvKeyName, "")
	keyPath := filepath.Join(t.TempDir(), "sub", "secret.key")

	created, err := EnsurePersistentKey(keyPath)
	if err != nil {
		t.Fatalf("EnsurePersistentKey: %v", err)
	}
	if !created {
		t.Fatalf("expected created=true when key file is missing")
	}

	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("expected key file to be written: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("expected key file perms 0600, got %o", perm)
	}

	envValue := strings.TrimSpace(os.Getenv(EnvKeyName))
	if envValue == "" {
		t.Fatalf("expected %s to be exported after provisioning", EnvKeyName)
	}

	fileBytes, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("read key file: %v", err)
	}
	if strings.TrimSpace(string(fileBytes)) != envValue {
		t.Fatalf("env value and file content differ")
	}

	// The provisioned key must be immediately usable by the codec.
	if _, err := NewCodecFromEnv(); err != nil {
		t.Fatalf("provisioned key not usable by NewCodecFromEnv: %v", err)
	}
}

func TestEnsurePersistentKeyLoadsExisting(t *testing.T) {
	t.Setenv(EnvKeyName, "")
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "secret.key")

	// Seed an existing key file.
	seed, err := EnsurePersistentKey(keyPath)
	if err != nil || !seed {
		t.Fatalf("seed EnsurePersistentKey: created=%v err=%v", seed, err)
	}
	want := strings.TrimSpace(os.Getenv(EnvKeyName))

	// Clear the env and re-run: it should load the existing file, not regenerate.
	t.Setenv(EnvKeyName, "")
	created, err := EnsurePersistentKey(keyPath)
	if err != nil {
		t.Fatalf("EnsurePersistentKey (reload): %v", err)
	}
	if created {
		t.Fatalf("expected created=false when key file already exists")
	}
	if got := strings.TrimSpace(os.Getenv(EnvKeyName)); got != want {
		t.Fatalf("reloaded key differs: got %q want %q", got, want)
	}
}

func TestEnsurePersistentKeyRespectsExistingEnv(t *testing.T) {
	existing := "already-configured-key"
	t.Setenv(EnvKeyName, existing)
	keyPath := filepath.Join(t.TempDir(), "secret.key")

	created, err := EnsurePersistentKey(keyPath)
	if err != nil {
		t.Fatalf("EnsurePersistentKey: %v", err)
	}
	if created {
		t.Fatalf("expected created=false when env var is already set")
	}
	if got := os.Getenv(EnvKeyName); got != existing {
		t.Fatalf("existing env var was modified: got %q want %q", got, existing)
	}
	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Fatalf("expected no key file to be written when env var is set, stat err=%v", err)
	}
}
