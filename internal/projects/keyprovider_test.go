package projects

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileKeyProvider_WrapUnwrap(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	provider, err := NewFileKeyProvider(tmpDir, "")
	if err != nil {
		t.Fatalf("NewFileKeyProvider failed: %v", err)
	}
	defer provider.Close()

	// Test basic wrap/unwrap
	dek := make([]byte, 32)
	if _, err := rand.Read(dek); err != nil {
		t.Fatalf("generate DEK: %v", err)
	}

	ctx := context.Background()
	wrapped, err := provider.WrapDEK(ctx, "test-project", dek)
	if err != nil {
		t.Fatalf("WrapDEK failed: %v", err)
	}

	if len(wrapped) == 0 {
		t.Error("wrapped DEK should not be empty")
	}

	unwrapped, err := provider.UnwrapDEK(ctx, "test-project", wrapped)
	if err != nil {
		t.Fatalf("UnwrapDEK failed: %v", err)
	}

	if len(unwrapped) != len(dek) {
		t.Errorf("unwrapped DEK length mismatch: got %d, want %d", len(unwrapped), len(dek))
	}

	for i := range dek {
		if unwrapped[i] != dek[i] {
			t.Errorf("unwrapped DEK mismatch at byte %d", i)
			break
		}
	}
}

func TestFileKeyProvider_ProviderType(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	provider, err := NewFileKeyProvider(tmpDir, "")
	if err != nil {
		t.Fatalf("NewFileKeyProvider failed: %v", err)
	}
	defer provider.Close()

	if got := provider.ProviderType(); got != "file" {
		t.Errorf("ProviderType() = %q, want %q", got, "file")
	}
}

func TestFileKeyProvider_HealthCheck(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	provider, err := NewFileKeyProvider(tmpDir, "")
	if err != nil {
		t.Fatalf("NewFileKeyProvider failed: %v", err)
	}
	defer provider.Close()

	ctx := context.Background()
	if err := provider.HealthCheck(ctx); err != nil {
		t.Errorf("HealthCheck failed: %v", err)
	}
}

func TestFileKeyProvider_CustomKeystorePath(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	customPath := filepath.Join(tmpDir, "custom", "keystore")

	provider, err := NewFileKeyProvider(tmpDir, customPath)
	if err != nil {
		t.Fatalf("NewFileKeyProvider failed: %v", err)
	}
	defer provider.Close()

	// Verify keystore was created at custom path
	keyPath := filepath.Join(customPath, "master.key")
	if _, err := os.Stat(keyPath); err != nil {
		t.Errorf("master.key not found at custom path: %v", err)
	}
}

func TestFileKeyProvider_PersistentMasterKey(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create first provider
	provider1, err := NewFileKeyProvider(tmpDir, "")
	if err != nil {
		t.Fatalf("NewFileKeyProvider failed: %v", err)
	}

	ctx := context.Background()
	dek := make([]byte, 32)
	if _, err := rand.Read(dek); err != nil {
		t.Fatalf("generate DEK: %v", err)
	}

	wrapped, err := provider1.WrapDEK(ctx, "test-project", dek)
	if err != nil {
		t.Fatalf("WrapDEK failed: %v", err)
	}
	provider1.Close()

	// Create second provider using same directory
	provider2, err := NewFileKeyProvider(tmpDir, "")
	if err != nil {
		t.Fatalf("NewFileKeyProvider (second) failed: %v", err)
	}
	defer provider2.Close()

	// Should be able to unwrap with same master key
	unwrapped, err := provider2.UnwrapDEK(ctx, "test-project", wrapped)
	if err != nil {
		t.Fatalf("UnwrapDEK (second provider) failed: %v", err)
	}

	for i := range dek {
		if unwrapped[i] != dek[i] {
			t.Errorf("unwrapped DEK mismatch at byte %d", i)
			break
		}
	}
}

func TestFileKeyProvider_DifferentProjectIDs(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	provider, err := NewFileKeyProvider(tmpDir, "")
	if err != nil {
		t.Fatalf("NewFileKeyProvider failed: %v", err)
	}
	defer provider.Close()

	ctx := context.Background()
	dek := make([]byte, 32)
	if _, err := rand.Read(dek); err != nil {
		t.Fatalf("generate DEK: %v", err)
	}

	// Wrap with project ID "A"
	wrapped, err := provider.WrapDEK(ctx, "project-A", dek)
	if err != nil {
		t.Fatalf("WrapDEK failed: %v", err)
	}

	// Unwrap with project ID "B" should still work (file provider ignores projectID)
	unwrapped, err := provider.UnwrapDEK(ctx, "project-B", wrapped)
	if err != nil {
		t.Fatalf("UnwrapDEK failed: %v", err)
	}

	for i := range dek {
		if unwrapped[i] != dek[i] {
			t.Errorf("unwrapped DEK mismatch at byte %d", i)
			break
		}
	}
}

func TestNewKeyProvider_Factory(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	tests := []struct {
		name     string
		cfg      KeyProviderConfig
		wantType string
		wantErr  bool
	}{
		{
			name:     "default (empty type)",
			cfg:      KeyProviderConfig{Type: ""},
			wantType: "file",
		},
		{
			name:     "explicit file type",
			cfg:      KeyProviderConfig{Type: "file"},
			wantType: "file",
		},
		{
			name:    "vault without config",
			cfg:     KeyProviderConfig{Type: "vault"},
			wantErr: true,
		},
		{
			name:    "awskms without config",
			cfg:     KeyProviderConfig{Type: "awskms"},
			wantErr: true,
		},
		{
			name:    "unknown type",
			cfg:     KeyProviderConfig{Type: "unknown"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewKeyProvider(tmpDir, tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewKeyProvider() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			defer provider.Close()

			if got := provider.ProviderType(); got != tt.wantType {
				t.Errorf("ProviderType() = %q, want %q", got, tt.wantType)
			}
		})
	}
}

func TestEncEnvelopeV2_Format(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	encPath := filepath.Join(tmpDir, "enc.json")

	wrapped := []byte("test-wrapped-dek-data")
	if err := writeWrappedDEKv2(encPath, "vault", wrapped); err != nil {
		t.Fatalf("writeWrappedDEKv2 failed: %v", err)
	}

	// Read and verify format
	data, err := os.ReadFile(encPath)
	if err != nil {
		t.Fatalf("read enc.json: %v", err)
	}

	var env encEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}

	if env.Alg != "envelope" {
		t.Errorf("Alg = %q, want %q", env.Alg, "envelope")
	}
	if env.WrapVersion != 2 {
		t.Errorf("WrapVersion = %d, want %d", env.WrapVersion, 2)
	}
	if env.ProviderType != "vault" {
		t.Errorf("ProviderType = %q, want %q", env.ProviderType, "vault")
	}
	if env.NonceB64 != "" {
		t.Errorf("NonceB64 should be empty for v2, got %q", env.NonceB64)
	}
}

func TestServiceWithKeyProvider(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create service with KeyProvider
	svc := NewService(tmpDir)
	provider, err := NewFileKeyProvider(tmpDir, "")
	if err != nil {
		t.Fatalf("NewFileKeyProvider failed: %v", err)
	}
	defer provider.Close()

	svc.SetKeyProvider(provider)
	if err := svc.EnableEncryption(true); err != nil {
		t.Fatalf("EnableEncryption failed: %v", err)
	}

	// Verify provider is set
	if got := svc.GetKeyProvider(); got == nil {
		t.Error("GetKeyProvider() returned nil")
	}

	// Test project creation with encryption
	ctx := context.Background()
	proj, err := svc.CreateProject(ctx, 1, "Test Project")
	if err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}

	// Verify enc.json was created
	encPath := filepath.Join(tmpDir, "users", "1", "projects", proj.ID, ".meta", "enc.json")
	if _, err := os.Stat(encPath); err != nil {
		t.Errorf("enc.json not found: %v", err)
	}

	// Verify it's a v2 envelope with KeyProvider
	data, err := os.ReadFile(encPath)
	if err != nil {
		t.Fatalf("read enc.json: %v", err)
	}

	var env encEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}

	if env.WrapVersion != 2 {
		t.Errorf("WrapVersion = %d, want %d (KeyProvider)", env.WrapVersion, 2)
	}
	if env.ProviderType != "file" {
		t.Errorf("ProviderType = %q, want %q", env.ProviderType, "file")
	}
}

func TestServiceLegacyMasterKey(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create service without KeyProvider (legacy mode)
	svc := NewService(tmpDir)
	if err := svc.EnableEncryption(true); err != nil {
		t.Fatalf("EnableEncryption failed: %v", err)
	}

	// Verify no KeyProvider is set
	if got := svc.GetKeyProvider(); got != nil {
		t.Error("GetKeyProvider() should be nil in legacy mode")
	}

	// Test project creation with legacy encryption
	ctx := context.Background()
	proj, err := svc.CreateProject(ctx, 1, "Legacy Project")
	if err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}

	// Verify enc.json was created
	encPath := filepath.Join(tmpDir, "users", "1", "projects", proj.ID, ".meta", "enc.json")
	data, err := os.ReadFile(encPath)
	if err != nil {
		t.Fatalf("read enc.json: %v", err)
	}

	var env encEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}

	// Legacy mode should use v1 envelope
	if env.WrapVersion != 1 {
		t.Errorf("WrapVersion = %d, want %d (legacy)", env.WrapVersion, 1)
	}
	if env.Alg != "AES-256-GCM" {
		t.Errorf("Alg = %q, want %q", env.Alg, "AES-256-GCM")
	}
}

// TestVaultKeyProvider_ConfigValidation tests Vault config validation
func TestVaultKeyProvider_ConfigValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     VaultKeyProviderConfig
		wantErr string
	}{
		{
			name:    "missing address",
			cfg:     VaultKeyProviderConfig{KeyName: "test-key", Token: "test-token"},
			wantErr: "vault address required",
		},
		{
			name:    "missing key name",
			cfg:     VaultKeyProviderConfig{Address: "http://vault:8200", Token: "test-token"},
			wantErr: "vault key name required",
		},
		{
			name:    "missing token (no env)",
			cfg:     VaultKeyProviderConfig{Address: "http://vault:8200", KeyName: "test-key"},
			wantErr: "vault token required",
		},
	}

	// Clear VAULT_TOKEN to test validation
	origToken := os.Getenv("VAULT_TOKEN")
	os.Unsetenv("VAULT_TOKEN")
	defer func() {
		if origToken != "" {
			os.Setenv("VAULT_TOKEN", origToken)
		}
	}()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewVaultKeyProvider(tt.cfg)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// TestAWSKMSKeyProvider_ConfigValidation tests AWS KMS config validation
func TestAWSKMSKeyProvider_ConfigValidation(t *testing.T) {
	t.Parallel()

	// Clear AWS env vars
	origRegion := os.Getenv("AWS_REGION")
	origDefaultRegion := os.Getenv("AWS_DEFAULT_REGION")
	os.Unsetenv("AWS_REGION")
	os.Unsetenv("AWS_DEFAULT_REGION")
	defer func() {
		if origRegion != "" {
			os.Setenv("AWS_REGION", origRegion)
		}
		if origDefaultRegion != "" {
			os.Setenv("AWS_DEFAULT_REGION", origDefaultRegion)
		}
	}()

	tests := []struct {
		name    string
		cfg     AWSKMSKeyProviderConfig
		wantErr string
	}{
		{
			name:    "missing key ID",
			cfg:     AWSKMSKeyProviderConfig{Region: "us-east-1"},
			wantErr: "KMS key ID required",
		},
		{
			name:    "missing region",
			cfg:     AWSKMSKeyProviderConfig{KeyID: "arn:aws:kms:us-east-1:123456789:key/test"},
			wantErr: "AWS region required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewAWSKMSKeyProvider(tt.cfg)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err.Error() != tt.wantErr {
				t.Errorf("error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}
