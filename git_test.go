package main

import (
	"testing"
)

func TestSetDefaultChunkValues(t *testing.T) {
	req := &struct {
		RepoPath     string `json:"repo_path"`
		ChunkSize    int    `json:"chunk_size"`
		ChunkOverlap int    `json:"chunk_overlap"`
	}{
		RepoPath: "some/path",
	}
	setDefaultChunkValues(req)
	if req.ChunkSize != 1000 {
		t.Errorf("expected default ChunkSize 1000, got %d", req.ChunkSize)
	}
	if req.ChunkOverlap != 100 {
		t.Errorf("expected default ChunkOverlap 100, got %d", req.ChunkOverlap)
	}

	req2 := &struct {
		RepoPath     string `json:"repo_path"`
		ChunkSize    int    `json:"chunk_size"`
		ChunkOverlap int    `json:"chunk_overlap"`
	}{
		RepoPath:     "some/path",
		ChunkSize:    500,
		ChunkOverlap: 50,
	}
	setDefaultChunkValues(req2)
	if req2.ChunkSize != 500 {
		t.Errorf("expected ChunkSize unchanged 500, got %d", req2.ChunkSize)
	}
	if req2.ChunkOverlap != 50 {
		t.Errorf("expected ChunkOverlap unchanged 50, got %d", req2.ChunkOverlap)
	}
}
