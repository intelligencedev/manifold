package mcpclient

import (
	"context"
	"testing"
	"time"

	"manifold/internal/config"
)

func TestMCPServerPool_RequiresPerUserMCP(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *config.Config
		expected bool
	}{
		{
			name:     "nil config",
			cfg:      nil,
			expected: false,
		},
		{
			name: "simple mode - auth disabled",
			cfg: &config.Config{
				Auth: config.AuthConfig{Enabled: false},
				MCP: config.MCPConfig{
					Servers: []config.MCPServerConfig{{Name: "pd", PathDependent: true}},
				},
			},
			expected: false,
		},
		{
			name: "auth enabled without path-dependent servers",
			cfg: &config.Config{
				Auth: config.AuthConfig{Enabled: true},
			},
			expected: false,
		},
		{
			name: "auth enabled with path-dependent servers",
			cfg: &config.Config{
				Auth: config.AuthConfig{Enabled: true},
				MCP: config.MCPConfig{
					Servers: []config.MCPServerConfig{{Name: "pd", PathDependent: true}},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var pool *MCPServerPool
			if tt.cfg == nil {
				pool = &MCPServerPool{}
			} else {
				pool = NewMCPServerPool(tt.cfg, nil, nil)
			}
			if got := pool.RequiresPerUserMCP(); got != tt.expected {
				t.Errorf("RequiresPerUserMCP() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestMCPServerPool_ResolveServerConfig(t *testing.T) {
	pool := &MCPServerPool{}

	srv := config.MCPServerConfig{
		Name:    "test",
		Command: "docker",
		Args: []string{
			"run",
			"-v",
			"{{PROJECT_DIR}}:/app/files",
			"--workdir",
			"{{PROJECT_DIR}}",
		},
		Env: map[string]string{
			"PROJECT_PATH": "{{PROJECT_DIR}}",
			"HOME":         "/home/user",
		},
	}

	resolved := pool.resolveServerConfig(srv, "/tmp/workspace/user1/project-abc")

	// Check args expansion
	expectedArgs := []string{
		"run",
		"-v",
		"/tmp/workspace/user1/project-abc:/app/files",
		"--workdir",
		"/tmp/workspace/user1/project-abc",
	}
	if len(resolved.Args) != len(expectedArgs) {
		t.Fatalf("Args length mismatch: got %d, want %d", len(resolved.Args), len(expectedArgs))
	}
	for i, arg := range resolved.Args {
		if arg != expectedArgs[i] {
			t.Errorf("Args[%d] = %q, want %q", i, arg, expectedArgs[i])
		}
	}

	// Check env expansion
	if resolved.Env["PROJECT_PATH"] != "/tmp/workspace/user1/project-abc" {
		t.Errorf("Env[PROJECT_PATH] = %q, want %q", resolved.Env["PROJECT_PATH"], "/tmp/workspace/user1/project-abc")
	}
	if resolved.Env["HOME"] != "/home/user" {
		t.Errorf("Env[HOME] = %q, want %q (should not be modified)", resolved.Env["HOME"], "/home/user")
	}

	// Check original is not modified
	if srv.Args[2] != "{{PROJECT_DIR}}:/app/files" {
		t.Error("Original config was modified")
	}
}

func TestMCPServerPool_SimpleMode_SkipsPerUserSetup(t *testing.T) {
	cfg := &config.Config{
		Auth: config.AuthConfig{Enabled: false},
		MCP: config.MCPConfig{
			Servers: []config.MCPServerConfig{
				{Name: "test", PathDependent: true},
			},
		},
	}

	pool := NewMCPServerPool(cfg, nil, nil)

	// In simple mode, EnsureUserSession should be a no-op
	err := pool.EnsureUserSession(context.Background(), nil, 123, "project-1", "/tmp/ws")
	if err != nil {
		t.Errorf("EnsureUserSession() error = %v, want nil", err)
	}

	// Should not have any per-user sessions
	if pool.ActiveUserCount() != 0 {
		t.Errorf("ActiveUserCount() = %d, want 0", pool.ActiveUserCount())
	}
}

func TestMCPServerPool_PathDependentServerCategorization(t *testing.T) {
	cfg := &config.Config{
		MCP: config.MCPConfig{
			Servers: []config.MCPServerConfig{
				{Name: "shared1", PathDependent: false},
				{Name: "shared2", PathDependent: false},
				{Name: "peruser1", PathDependent: true},
				{Name: "peruser2", PathDependent: true},
			},
		},
	}

	pool := NewMCPServerPool(cfg, nil, nil)

	if len(pool.pathDependentServers) != 2 {
		t.Errorf("pathDependentServers count = %d, want 2", len(pool.pathDependentServers))
	}

	names := make(map[string]bool)
	for _, srv := range pool.pathDependentServers {
		names[srv.Name] = true
	}
	if !names["peruser1"] || !names["peruser2"] {
		t.Error("Expected peruser1 and peruser2 in pathDependentServers")
	}
}

func TestMCPServerPool_UserSessionTracking(t *testing.T) {
	pool := &MCPServerPool{
		perUser: make(map[int64]*userMCPState),
	}

	// Initially no sessions
	if pool.UserHasSession(1) {
		t.Error("Expected no session for user 1")
	}

	// Add a session
	pool.perUser[1] = &userMCPState{
		projectID:     "proj-a",
		workspacePath: "/tmp/ws/1",
		lastAccess:    time.Now(),
		manager:       NewManager(),
	}

	// Check session exists
	if !pool.UserHasSession(1) {
		t.Error("Expected session for user 1")
	}

	// Check project ID
	projectID, ok := pool.GetUserProjectID(1)
	if !ok || projectID != "proj-a" {
		t.Errorf("GetUserProjectID(1) = %q, %v; want %q, true", projectID, ok, "proj-a")
	}

	// Check active count
	if pool.ActiveUserCount() != 1 {
		t.Errorf("ActiveUserCount() = %d, want 1", pool.ActiveUserCount())
	}
}

func TestMCPServerPool_Close(t *testing.T) {
	pool := &MCPServerPool{
		shared:  NewManager(),
		perUser: make(map[int64]*userMCPState),
	}

	// Add some user sessions
	pool.perUser[1] = &userMCPState{manager: NewManager()}
	pool.perUser[2] = &userMCPState{manager: NewManager()}

	pool.Close()

	// Should have no per-user sessions after close
	if len(pool.perUser) != 0 {
		t.Errorf("Expected 0 per-user sessions after Close, got %d", len(pool.perUser))
	}
}
