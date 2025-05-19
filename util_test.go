package main

import (
	codeeval "manifold/internal/codeeval"
	"strings"
	"testing"
)

func TestGenerateUniqueFilename(t *testing.T) {
	orig := "file.txt"
	fname := generateUniqueFilename(orig)
	// Should start with name_ and end with .txt
	if !strings.HasPrefix(fname, "file_") {
		t.Errorf("filename %s does not have expected prefix", fname)
	}
	if !strings.HasSuffix(fname, ".txt") {
		t.Errorf("filename %s does not have expected suffix", fname)
	}
	// Timestamp should be between underscore and .txt
	parts := strings.Split(fname, "_")
	if len(parts) < 2 {
		t.Fatalf("filename %s not in expected format", fname)
	}
	timestampPart := strings.TrimSuffix(parts[len(parts)-1], ".txt")
	if len(timestampPart) != len("20060102-150405") {
		t.Errorf("timestamp %s has unexpected length", timestampPart)
	}
}

func TestConvertDockerResponse_Success(t *testing.T) {
	dresp := &codeeval.DockerExecResponse{ReturnCode: 0, Stdout: "output", Stderr: ""}
	resp := codeeval.ConvertDockerResponse(dresp)
	if resp.Result != "output" {
		t.Errorf("expected result output, got %s", resp.Result)
	}
	if resp.Error != "" {
		t.Errorf("expected no error, got %s", resp.Error)
	}
}

func TestConvertDockerResponse_Error(t *testing.T) {
	dresp := &codeeval.DockerExecResponse{ReturnCode: 1, Stdout: "", Stderr: "error occurred"}
	resp := codeeval.ConvertDockerResponse(dresp)
	if resp.Error != "error occurred" {
		t.Errorf("expected error 'error occurred', got %s", resp.Error)
	}
	if resp.Result != "" {
		t.Errorf("expected empty result, got %s", resp.Result)
	}
}
