package main

import (
	"path/filepath"
	"testing"

	configpkg "manifold/internal/config"
	servicespkg "manifold/internal/services"
)

func TestParseConnectionString_Valid(t *testing.T) {
	user, pass, db, ok := servicespkg.ParseConnectionString("postgres://u:p@host:5432/mydb")
	if !ok {
		t.Fatal("expected ok true for valid connection string")
	}
	if user != "u" || pass != "p" || db != "mydb" {
		t.Errorf("unexpected parse results: %s, %s, %s", user, pass, db)
	}
}

func TestParseConnectionString_WithQueryParams(t *testing.T) {
	user, pass, db, ok := servicespkg.ParseConnectionString("postgres://user:pass@host:5432/dbname?sslmode=disable")
	if !ok {
		t.Fatal("expected ok true for valid connection string with params")
	}
	if user != "user" || pass != "pass" || db != "dbname" {
		t.Errorf("unexpected parse with params: %s, %s, %s", user, pass, db)
	}
}

func TestParseConnectionString_MissingCredentials(t *testing.T) {
	// Test with format: postgres://host:5432/testdb (no @ symbol)
	user, pass, db, ok := servicespkg.ParseConnectionString("postgres://host:5432/testdb")
	if ok {
		// The current implementation returns ok=false for this format
		// since it can't find the '@' symbol to separate credentials
		t.Fatal("expected ok false for connection string missing @ symbol")
	}
	// Should still return default values
	if user != "postgres" || pass != "postgres" || db != "manifold" {
		t.Errorf("unexpected default values: user=%s, pass=%s, db=%s", user, pass, db)
	}

	// Test with properly formatted empty credentials: postgres://@host:5432/testdb
	user, pass, db, ok = servicespkg.ParseConnectionString("postgres://@host:5432/testdb")
	if !ok {
		t.Fatal("expected ok true for connection string with empty but properly formatted credentials")
	}
	// Should use default values for empty credentials
	if user != "postgres" || pass != "postgres" {
		t.Errorf("unexpected default credentials: user=%s, pass=%s", user, pass)
	}
	// But should still parse the database name correctly
	if db != "testdb" {
		t.Errorf("unexpected database name: got %s, expected: %s", db, "testdb")
	}

	// Test with username but no password: postgres://someuser@host:5432/testdb
	user, pass, db, ok = servicespkg.ParseConnectionString("postgres://someuser@host:5432/testdb")
	if !ok {
		t.Fatal("expected ok true for connection string with username but no password")
	}
	// Should use the provided username but default password
	if user != "someuser" || pass != "postgres" {
		t.Errorf("unexpected credentials: user=%s, pass=%s; expected user=someuser, pass=postgres", user, pass)
	}
	if db != "testdb" {
		t.Errorf("unexpected database name: got %s, expected: %s", db, "testdb")
	}
}

func TestParseConnectionString_Invalid(t *testing.T) {
	user, pass, db, ok := servicespkg.ParseConnectionString("invalidformat")
	if ok {
		t.Fatal("expected ok false for invalid connection string")
	}
	// Should return defaults
	if user == "" || pass == "" || db == "" {
		t.Error("expected default non-empty values for user, pass, db")
	}
}

func TestGetModelPath(t *testing.T) {
	cfg := &configpkg.Config{DataPath: "/data"}
	cases := []struct {
		service string
		expects string
	}{
		{"completions", filepath.Join("/data", "models", "gguf", "gemma-3-4b-it-Q8_0.gguf")},
		{"embeddings", filepath.Join("/data", "models", "embeddings", "nomic-embed-text-v1.5.Q8_0.gguf")},
		{"reranker", filepath.Join("/data", "models", "rerankers", "slide-bge-reranker-v2-m3.Q4_K_M.gguf")},
		{"unknown", ""},
	}
	for _, c := range cases {
		got := servicespkg.GetModelPath(cfg, c.service)
		if got != c.expects {
			t.Errorf("getModelPath(%s) = %s; want %s", c.service, got, c.expects)
		}
	}
}
