//go:build enterprise
// +build enterprise

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectsConfig_Defaults(t *testing.T) {
	// Ensure required env vars for Load()
	oldOpenAI := os.Getenv("OPENAI_API_KEY")
	oldWorkdir := os.Getenv("WORKDIR")
	defer func() {
		_ = os.Setenv("OPENAI_API_KEY", oldOpenAI)
		_ = os.Setenv("WORKDIR", oldWorkdir)
	}()
	_ = os.Setenv("OPENAI_API_KEY", "dummy")
	_ = os.Setenv("WORKDIR", ".")

	// Clear any existing projects env vars
	projectsEnvVars := []string{
		"PROJECTS_BACKEND",
		"PROJECTS_ENCRYPT",
		"PROJECTS_WORKSPACE_MODE",
		"PROJECTS_WORKSPACE_ROOT",
		"PROJECTS_WORKSPACE_TTL_SECONDS",
		"PROJECTS_S3_ENDPOINT",
		"PROJECTS_S3_REGION",
		"PROJECTS_S3_BUCKET",
		"PROJECTS_S3_PREFIX",
		"PROJECTS_S3_ACCESS_KEY",
		"PROJECTS_S3_SECRET_KEY",
		"PROJECTS_S3_USE_PATH_STYLE",
		"PROJECTS_S3_TLS_INSECURE",
		"PROJECTS_S3_SSE_MODE",
		"PROJECTS_S3_SSE_KMS_KEY_ID",
	}
	oldEnvVars := make(map[string]string)
	for _, key := range projectsEnvVars {
		oldEnvVars[key] = os.Getenv(key)
		_ = os.Unsetenv(key)
	}
	defer func() {
		for key, val := range oldEnvVars {
			if val != "" {
				_ = os.Setenv(key, val)
			}
		}
	}()

	cfg, err := Load()
	require.NoError(t, err, "Load() should succeed")

	// Verify defaults
	assert.Equal(t, "filesystem", cfg.Projects.Backend, "default backend")
	assert.False(t, cfg.Projects.Encrypt, "encryption disabled by default")
	assert.Equal(t, "legacy", cfg.Projects.Workspace.Mode, "default workspace mode")
	assert.Equal(t, 86400, cfg.Projects.Workspace.TTLSeconds, "default TTL")
	assert.Contains(t, cfg.Projects.Workspace.Root, "sandboxes", "workspace root contains sandboxes")
	assert.Equal(t, "us-east-1", cfg.Projects.S3.Region, "default S3 region")
	assert.Equal(t, "workspaces", cfg.Projects.S3.Prefix, "default S3 prefix")
	assert.Equal(t, "none", cfg.Projects.S3.SSE.Mode, "default SSE mode")
}

func TestProjectsConfig_EnvOverrides(t *testing.T) {
	// Ensure required env vars for Load()
	oldOpenAI := os.Getenv("OPENAI_API_KEY")
	oldWorkdir := os.Getenv("WORKDIR")
	defer func() {
		_ = os.Setenv("OPENAI_API_KEY", oldOpenAI)
		_ = os.Setenv("WORKDIR", oldWorkdir)
	}()
	_ = os.Setenv("OPENAI_API_KEY", "dummy")
	_ = os.Setenv("WORKDIR", ".")

	// Set projects env vars
	projectsEnvVars := map[string]string{
		"PROJECTS_BACKEND":               "s3",
		"PROJECTS_ENCRYPT":               "true",
		"PROJECTS_WORKSPACE_MODE":        "ephemeral",
		"PROJECTS_WORKSPACE_TTL_SECONDS": "3600",
		"PROJECTS_S3_ENDPOINT":           "http://minio:9000",
		"PROJECTS_S3_REGION":             "eu-west-1",
		"PROJECTS_S3_BUCKET":             "test-bucket",
		"PROJECTS_S3_PREFIX":             "test-prefix",
		"PROJECTS_S3_ACCESS_KEY":         "minioadmin",
		"PROJECTS_S3_SECRET_KEY":         "miniosecret",
		"PROJECTS_S3_USE_PATH_STYLE":     "true",
		"PROJECTS_S3_TLS_INSECURE":       "true",
		"PROJECTS_S3_SSE_MODE":           "sse-kms",
		"PROJECTS_S3_SSE_KMS_KEY_ID":     "alias/my-key",
	}
	oldEnvVars := make(map[string]string)
	for key, val := range projectsEnvVars {
		oldEnvVars[key] = os.Getenv(key)
		_ = os.Setenv(key, val)
	}
	defer func() {
		for key := range projectsEnvVars {
			if old := oldEnvVars[key]; old != "" {
				_ = os.Setenv(key, old)
			} else {
				_ = os.Unsetenv(key)
			}
		}
	}()

	cfg, err := Load()
	require.NoError(t, err, "Load() should succeed")

	// Verify env overrides
	assert.Equal(t, "s3", cfg.Projects.Backend)
	assert.True(t, cfg.Projects.Encrypt)
	assert.Equal(t, "ephemeral", cfg.Projects.Workspace.Mode)
	assert.Equal(t, 3600, cfg.Projects.Workspace.TTLSeconds)
	assert.Equal(t, "http://minio:9000", cfg.Projects.S3.Endpoint)
	assert.Equal(t, "eu-west-1", cfg.Projects.S3.Region)
	assert.Equal(t, "test-bucket", cfg.Projects.S3.Bucket)
	assert.Equal(t, "test-prefix", cfg.Projects.S3.Prefix)
	assert.Equal(t, "minioadmin", cfg.Projects.S3.AccessKey)
	assert.Equal(t, "miniosecret", cfg.Projects.S3.SecretKey)
	assert.True(t, cfg.Projects.S3.UsePathStyle)
	assert.True(t, cfg.Projects.S3.TLSInsecureSkipVerify)
	assert.Equal(t, "sse-kms", cfg.Projects.S3.SSE.Mode)
	assert.Equal(t, "alias/my-key", cfg.Projects.S3.SSE.KMSKeyID)
}

func TestProjectsConfig_YAMLParsing(t *testing.T) {
	// Create temp config file
	yamlContent := `
projects:
  backend: s3
  encrypt: true
  workspace:
    mode: ephemeral
    root: /tmp/test-sandboxes
    ttlSeconds: 7200
  s3:
    endpoint: "http://localhost:9000"
    region: "ap-south-1"
    bucket: "yaml-bucket"
    prefix: "yaml-prefix"
    accessKey: "yaml-access"
    secretKey: "yaml-secret"
    usePathStyle: true
    tlsInsecureSkipVerify: true
    sse:
      mode: sse-s3
`
	tmpFile, err := os.CreateTemp("", "config_test_*.yaml")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	_, err = tmpFile.WriteString(yamlContent)
	require.NoError(t, err)
	tmpFile.Close()

	// Point SPECIALISTS_CONFIG to our test file
	oldSpecialistsConfig := os.Getenv("SPECIALISTS_CONFIG")
	defer func() { _ = os.Setenv("SPECIALISTS_CONFIG", oldSpecialistsConfig) }()
	_ = os.Setenv("SPECIALISTS_CONFIG", tmpFile.Name())

	// Ensure required env vars for Load()
	oldOpenAI := os.Getenv("OPENAI_API_KEY")
	oldWorkdir := os.Getenv("WORKDIR")
	defer func() {
		_ = os.Setenv("OPENAI_API_KEY", oldOpenAI)
		_ = os.Setenv("WORKDIR", oldWorkdir)
	}()
	_ = os.Setenv("OPENAI_API_KEY", "dummy")
	_ = os.Setenv("WORKDIR", ".")

	// Clear projects env vars so YAML takes precedence
	projectsEnvVars := []string{
		"PROJECTS_BACKEND",
		"PROJECTS_ENCRYPT",
		"PROJECTS_WORKSPACE_MODE",
		"PROJECTS_WORKSPACE_ROOT",
		"PROJECTS_WORKSPACE_TTL_SECONDS",
		"PROJECTS_S3_ENDPOINT",
		"PROJECTS_S3_REGION",
		"PROJECTS_S3_BUCKET",
		"PROJECTS_S3_PREFIX",
		"PROJECTS_S3_ACCESS_KEY",
		"PROJECTS_S3_SECRET_KEY",
		"PROJECTS_S3_USE_PATH_STYLE",
		"PROJECTS_S3_TLS_INSECURE",
		"PROJECTS_S3_SSE_MODE",
		"PROJECTS_S3_SSE_KMS_KEY_ID",
	}
	oldEnvVarsMap := make(map[string]string)
	for _, key := range projectsEnvVars {
		oldEnvVarsMap[key] = os.Getenv(key)
		_ = os.Unsetenv(key)
	}
	defer func() {
		for key, val := range oldEnvVarsMap {
			if val != "" {
				_ = os.Setenv(key, val)
			}
		}
	}()

	cfg, err := Load()
	require.NoError(t, err, "Load() should succeed with YAML")

	// Verify YAML values
	assert.Equal(t, "s3", cfg.Projects.Backend)
	assert.True(t, cfg.Projects.Encrypt)
	assert.Equal(t, "ephemeral", cfg.Projects.Workspace.Mode)
	assert.Equal(t, "/tmp/test-sandboxes", cfg.Projects.Workspace.Root)
	assert.Equal(t, 7200, cfg.Projects.Workspace.TTLSeconds)
	assert.Equal(t, "http://localhost:9000", cfg.Projects.S3.Endpoint)
	assert.Equal(t, "ap-south-1", cfg.Projects.S3.Region)
	assert.Equal(t, "yaml-bucket", cfg.Projects.S3.Bucket)
	assert.Equal(t, "yaml-prefix", cfg.Projects.S3.Prefix)
	assert.Equal(t, "yaml-access", cfg.Projects.S3.AccessKey)
	assert.Equal(t, "yaml-secret", cfg.Projects.S3.SecretKey)
	assert.True(t, cfg.Projects.S3.UsePathStyle)
	assert.True(t, cfg.Projects.S3.TLSInsecureSkipVerify)
	assert.Equal(t, "sse-s3", cfg.Projects.S3.SSE.Mode)
}

func TestProjectsConfig_WorkspaceRootDefault(t *testing.T) {
	// Create a temporary directory as workdir
	tmpDir, err := os.MkdirTemp("", "workdir_test_")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Ensure required env vars
	oldOpenAI := os.Getenv("OPENAI_API_KEY")
	oldWorkdir := os.Getenv("WORKDIR")
	defer func() {
		_ = os.Setenv("OPENAI_API_KEY", oldOpenAI)
		_ = os.Setenv("WORKDIR", oldWorkdir)
	}()
	_ = os.Setenv("OPENAI_API_KEY", "dummy")
	_ = os.Setenv("WORKDIR", tmpDir)

	// Clear workspace root env var
	oldRoot := os.Getenv("PROJECTS_WORKSPACE_ROOT")
	defer func() {
		if oldRoot != "" {
			_ = os.Setenv("PROJECTS_WORKSPACE_ROOT", oldRoot)
		}
	}()
	_ = os.Unsetenv("PROJECTS_WORKSPACE_ROOT")

	cfg, err := Load()
	require.NoError(t, err)

	// Workspace root should default to ${WORKDIR}/sandboxes
	expectedRoot := filepath.Join(tmpDir, "sandboxes")
	assert.Equal(t, expectedRoot, cfg.Projects.Workspace.Root)
}

func TestProjectsConfig_BackendValues(t *testing.T) {
	tests := []struct {
		name     string
		backend  string
		expected string
	}{
		{"filesystem", "filesystem", "filesystem"},
		{"s3", "s3", "s3"},
		{"empty defaults to filesystem", "", "filesystem"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldOpenAI := os.Getenv("OPENAI_API_KEY")
			oldWorkdir := os.Getenv("WORKDIR")
			oldBackend := os.Getenv("PROJECTS_BACKEND")
			defer func() {
				_ = os.Setenv("OPENAI_API_KEY", oldOpenAI)
				_ = os.Setenv("WORKDIR", oldWorkdir)
				if oldBackend != "" {
					_ = os.Setenv("PROJECTS_BACKEND", oldBackend)
				} else {
					_ = os.Unsetenv("PROJECTS_BACKEND")
				}
			}()

			_ = os.Setenv("OPENAI_API_KEY", "dummy")
			_ = os.Setenv("WORKDIR", ".")
			if tt.backend != "" {
				_ = os.Setenv("PROJECTS_BACKEND", tt.backend)
			} else {
				_ = os.Unsetenv("PROJECTS_BACKEND")
			}

			cfg, err := Load()
			require.NoError(t, err)
			assert.Equal(t, tt.expected, cfg.Projects.Backend)
		})
	}
}

func TestProjectsConfig_WorkspaceModeValues(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		expected string
	}{
		{"legacy", "legacy", "legacy"},
		{"ephemeral", "ephemeral", "ephemeral"},
		{"empty defaults to legacy", "", "legacy"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldOpenAI := os.Getenv("OPENAI_API_KEY")
			oldWorkdir := os.Getenv("WORKDIR")
			oldMode := os.Getenv("PROJECTS_WORKSPACE_MODE")
			defer func() {
				_ = os.Setenv("OPENAI_API_KEY", oldOpenAI)
				_ = os.Setenv("WORKDIR", oldWorkdir)
				if oldMode != "" {
					_ = os.Setenv("PROJECTS_WORKSPACE_MODE", oldMode)
				} else {
					_ = os.Unsetenv("PROJECTS_WORKSPACE_MODE")
				}
			}()

			_ = os.Setenv("OPENAI_API_KEY", "dummy")
			_ = os.Setenv("WORKDIR", ".")
			if tt.mode != "" {
				_ = os.Setenv("PROJECTS_WORKSPACE_MODE", tt.mode)
			} else {
				_ = os.Unsetenv("PROJECTS_WORKSPACE_MODE")
			}

			cfg, err := Load()
			require.NoError(t, err)
			assert.Equal(t, tt.expected, cfg.Projects.Workspace.Mode)
		})
	}
}
