package main

import (
	"path/filepath"
	"testing"
)

func TestParseConnectionString_Valid(t *testing.T) {
	user, pass, db, ok := parseConnectionString("postgres://u:p@host:5432/mydb")
	if !ok {
		t.Fatal("expected ok true for valid connection string")
	}
	if user != "u" || pass != "p" || db != "mydb" {
		t.Errorf("unexpected parse results: %s, %s, %s", user, pass, db)
	}
}

func TestParseConnectionString_WithQueryParams(t *testing.T) {
	user, pass, db, ok := parseConnectionString("postgres://user:pass@host:5432/dbname?sslmode=disable")
	if !ok {
		t.Fatal("expected ok true for valid connection string with params")
	}
	if user != "user" || pass != "pass" || db != "dbname" {
		t.Errorf("unexpected parse with params: %s, %s, %s", user, pass, db)
	}
}

func TestParseConnectionString_Invalid(t *testing.T) {
	user, pass, db, ok := parseConnectionString("invalidformat")
	if ok {
		t.Fatal("expected ok false for invalid connection string")
	}
	// Should return defaults
	if user == "" || pass == "" || db == "" {
		t.Error("expected default non-empty values for user, pass, db")
	}
}

func TestGetModelPath(t *testing.T) {
	cfg := &Config{DataPath: "/data"}
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
		got := getModelPath(cfg, c.service)
		if got != c.expects {
			t.Errorf("getModelPath(%s) = %s; want %s", c.service, got, c.expects)
		}
	}
}
