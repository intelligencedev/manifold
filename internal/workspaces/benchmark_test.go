//go:build nowhisper

package workspaces_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"manifold/internal/config"
	"manifold/internal/objectstore"
	"manifold/internal/workspaces"
)

// BenchmarkCheckout_Cold measures checkout performance from a cold state
// (no local workspace exists).
func BenchmarkCheckout_Cold(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "bench-cold-*")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store := objectstore.NewMemoryStore()
	ctx := context.Background()

	// Seed with test files of varying sizes
	files := []struct {
		path string
		size int
	}{
		{"files/small.txt", 100},
		{"files/medium.txt", 10_000},
		{"files/large.txt", 100_000},
		{"files/src/main.go", 5_000},
		{"files/src/utils.go", 3_000},
		{"files/docs/readme.md", 2_000},
	}

	for _, f := range files {
		data := make([]byte, f.size)
		for i := range data {
			data[i] = byte('a' + i%26)
		}
		key := fmt.Sprintf("workspaces/users/1/projects/bench-proj/%s", f.path)
		_, err := store.Put(ctx, key, bytes.NewReader(data), objectstore.PutOptions{})
		if err != nil {
			b.Fatal(err)
		}
	}

	cfg := &config.Config{
		Workdir: tmpDir,
		Projects: config.ProjectsConfig{
			Backend: "s3",
			Workspace: config.WorkspaceConfig{
				Mode: "ephemeral",
				Root: filepath.Join(tmpDir, "sandboxes"),
			},
			S3: config.S3Config{
				Prefix: "workspaces",
			},
		},
	}

	mgr := workspaces.NewEphemeralManager(store, cfg)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		sessionID := fmt.Sprintf("bench-session-%d", i)
		ws, err := mgr.Checkout(ctx, 1, "bench-proj", sessionID)
		if err != nil {
			b.Fatal(err)
		}
		// Cleanup immediately
		_ = mgr.Cleanup(ctx, ws)
	}
}

// BenchmarkCheckout_Warm measures checkout performance when workspace already exists
// (session reuse scenario).
func BenchmarkCheckout_Warm(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "bench-warm-*")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store := objectstore.NewMemoryStore()
	ctx := context.Background()

	// Seed with test files
	files := []struct {
		path string
		size int
	}{
		{"files/small.txt", 100},
		{"files/medium.txt", 10_000},
		{"files/large.txt", 100_000},
	}

	for _, f := range files {
		data := make([]byte, f.size)
		key := fmt.Sprintf("workspaces/users/1/projects/bench-proj/%s", f.path)
		_, err := store.Put(ctx, key, bytes.NewReader(data), objectstore.PutOptions{})
		if err != nil {
			b.Fatal(err)
		}
	}

	cfg := &config.Config{
		Workdir: tmpDir,
		Projects: config.ProjectsConfig{
			Backend: "s3",
			Workspace: config.WorkspaceConfig{
				Mode: "ephemeral",
				Root: filepath.Join(tmpDir, "sandboxes"),
			},
			S3: config.S3Config{
				Prefix: "workspaces",
			},
		},
	}

	mgr := workspaces.NewEphemeralManager(store, cfg)

	// Initial checkout to warm the workspace
	ws, err := mgr.Checkout(ctx, 1, "bench-proj", "warm-session")
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Reuse the same session - should be fast
		ws, err = mgr.Checkout(ctx, 1, "bench-proj", "warm-session")
		if err != nil {
			b.Fatal(err)
		}
		_ = ws
	}
}

// BenchmarkCommit measures commit performance with various file sizes.
func BenchmarkCommit(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "bench-commit-*")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store := objectstore.NewMemoryStore()
	ctx := context.Background()

	// Seed initial file
	_, err = store.Put(ctx, "workspaces/users/1/projects/bench-proj/files/initial.txt",
		bytes.NewReader([]byte("initial")), objectstore.PutOptions{})
	if err != nil {
		b.Fatal(err)
	}

	cfg := &config.Config{
		Workdir: tmpDir,
		Projects: config.ProjectsConfig{
			Backend: "s3",
			Workspace: config.WorkspaceConfig{
				Mode: "ephemeral",
				Root: filepath.Join(tmpDir, "sandboxes"),
			},
			S3: config.S3Config{
				Prefix: "workspaces",
			},
		},
	}

	mgr := workspaces.NewEphemeralManager(store, cfg)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		b.StopTimer()

		sessionID := fmt.Sprintf("commit-session-%d", i)
		ws, err := mgr.Checkout(ctx, 1, "bench-proj", sessionID)
		if err != nil {
			b.Fatal(err)
		}

		// Create modified files
		modFile := filepath.Join(ws.BaseDir, "modified.txt")
		data := make([]byte, 10_000) // 10KB
		for j := range data {
			data[j] = byte('a' + j%26)
		}
		err = os.WriteFile(modFile, data, 0644)
		if err != nil {
			b.Fatal(err)
		}

		b.StartTimer()

		err = mgr.Commit(ctx, ws)
		if err != nil {
			b.Fatal(err)
		}

		b.StopTimer()
		_ = mgr.Cleanup(ctx, ws)
		b.StartTimer()
	}
}

// BenchmarkConcurrentCheckouts measures throughput under concurrent load.
func BenchmarkConcurrentCheckouts(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "bench-concurrent-*")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store := objectstore.NewMemoryStore()
	ctx := context.Background()

	// Seed test files
	_, err = store.Put(ctx, "workspaces/users/1/projects/bench-proj/files/test.txt",
		bytes.NewReader([]byte("test content")), objectstore.PutOptions{})
	if err != nil {
		b.Fatal(err)
	}

	cfg := &config.Config{
		Workdir: tmpDir,
		Projects: config.ProjectsConfig{
			Backend: "s3",
			Workspace: config.WorkspaceConfig{
				Mode: "ephemeral",
				Root: filepath.Join(tmpDir, "sandboxes"),
			},
			S3: config.S3Config{
				Prefix: "workspaces",
			},
		},
	}

	mgr := workspaces.NewEphemeralManager(store, cfg)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			sessionID := fmt.Sprintf("parallel-%d-%d", b.N, i)
			ws, err := mgr.Checkout(ctx, 1, "bench-proj", sessionID)
			if err != nil {
				b.Error(err)
				return
			}
			_ = mgr.Cleanup(ctx, ws)
			i++
		}
	})
}

// BenchmarkLegacyVsEphemeral compares legacy and ephemeral modes.
func BenchmarkLegacyVsEphemeral(b *testing.B) {
	b.Run("Legacy", func(b *testing.B) {
		tmpDir, err := os.MkdirTemp("", "bench-legacy-*")
		if err != nil {
			b.Fatal(err)
		}
		defer os.RemoveAll(tmpDir)

		// Create project directory - legacy mode expects projects under workdir/users/USER_ID/PROJECT_ID
		projectDir := filepath.Join(tmpDir, "users", "1", "bench-proj")
		err = os.MkdirAll(projectDir, 0755)
		if err != nil {
			b.Fatal(err)
		}
		err = os.WriteFile(filepath.Join(projectDir, "test.txt"), []byte("test"), 0644)
		if err != nil {
			b.Fatal(err)
		}

		cfg := &config.Config{
			Workdir: tmpDir,
			Projects: config.ProjectsConfig{
				Workspace: config.WorkspaceConfig{
					Mode: "legacy",
				},
			},
		}

		mgr := workspaces.NewManager(cfg)
		ctx := context.Background()

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			ws, err := mgr.Checkout(ctx, 1, "bench-proj", "session")
			if err != nil {
				b.Fatal(err)
			}
			_ = ws
		}
	})

	b.Run("Ephemeral", func(b *testing.B) {
		tmpDir, err := os.MkdirTemp("", "bench-ephemeral-*")
		if err != nil {
			b.Fatal(err)
		}
		defer os.RemoveAll(tmpDir)

		store := objectstore.NewMemoryStore()
		ctx := context.Background()

		_, err = store.Put(ctx, "workspaces/users/1/projects/bench-proj/files/test.txt",
			bytes.NewReader([]byte("test")), objectstore.PutOptions{})
		if err != nil {
			b.Fatal(err)
		}

		cfg := &config.Config{
			Workdir: tmpDir,
			Projects: config.ProjectsConfig{
				Backend: "s3",
				Workspace: config.WorkspaceConfig{
					Mode: "ephemeral",
					Root: filepath.Join(tmpDir, "sandboxes"),
				},
				S3: config.S3Config{
					Prefix: "workspaces",
				},
			},
		}

		mgr := workspaces.NewEphemeralManager(store, cfg)

		// Warm the workspace
		ws, _ := mgr.Checkout(ctx, 1, "bench-proj", "session")

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			ws, err = mgr.Checkout(ctx, 1, "bench-proj", "session")
			if err != nil {
				b.Fatal(err)
			}
			_ = ws
		}
	})
}

// BenchmarkS3Requests tracks the number of S3 operations per checkout.
// Note: This uses memory store, actual S3 will have network overhead.
func BenchmarkS3Requests_PerCheckout(b *testing.B) {
	sizes := []int{1, 10, 50, 100}

	for _, numFiles := range sizes {
		b.Run(fmt.Sprintf("Files_%d", numFiles), func(b *testing.B) {
			tmpDir, err := os.MkdirTemp("", "bench-s3-*")
			if err != nil {
				b.Fatal(err)
			}
			defer os.RemoveAll(tmpDir)

			store := objectstore.NewMemoryStore()
			ctx := context.Background()

			// Seed with N files
			for i := 0; i < numFiles; i++ {
				key := fmt.Sprintf("workspaces/users/1/projects/bench-proj/files/file%d.txt", i)
				_, err := store.Put(ctx, key, bytes.NewReader([]byte("content")), objectstore.PutOptions{})
				if err != nil {
					b.Fatal(err)
				}
			}

			cfg := &config.Config{
				Workdir: tmpDir,
				Projects: config.ProjectsConfig{
					Backend: "s3",
					Workspace: config.WorkspaceConfig{
						Mode: "ephemeral",
						Root: filepath.Join(tmpDir, "sandboxes"),
					},
					S3: config.S3Config{
						Prefix: "workspaces",
					},
				},
			}

			mgr := workspaces.NewEphemeralManager(store, cfg)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				sessionID := fmt.Sprintf("s3-session-%d", i)
				ws, err := mgr.Checkout(ctx, 1, "bench-proj", sessionID)
				if err != nil {
					b.Fatal(err)
				}
				_ = mgr.Cleanup(ctx, ws)
			}
		})
	}
}
