/*
migrateprojects scans existing filesystem-based projects and imports them
into the database (projects table + project_files index).

Usage:

	go run cmd/migrateprojects/main.go [flags]

Flags:

	-workdir string
	    Base workdir containing users/<uid>/projects/<pid> (required or via WORKDIR env)
	-dsn string
	    PostgreSQL connection string (required or via DATABASE_URL env)
	-dry-run
	    Print what would be migrated without making changes
	-verbose
	    Print detailed progress

Example:

	go run cmd/migrateprojects/main.go -workdir /data -dsn "postgres://..." -dry-run
*/
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"manifold/internal/persistence"
	"manifold/internal/persistence/databases"
)

func main() {
	workdir := flag.String("workdir", os.Getenv("WORKDIR"), "Base workdir (WORKDIR env)")
	dsn := flag.String("dsn", os.Getenv("DATABASE_URL"), "Postgres DSN (DATABASE_URL env)")
	dryRun := flag.Bool("dry-run", false, "Print what would be migrated without making changes")
	verbose := flag.Bool("verbose", false, "Print detailed progress")
	flag.Parse()

	if *workdir == "" {
		fmt.Fprintln(os.Stderr, "error: -workdir or WORKDIR env required")
		os.Exit(1)
	}
	if *dsn == "" {
		fmt.Fprintln(os.Stderr, "error: -dsn or DATABASE_URL env required")
		os.Exit(1)
	}

	ctx := context.Background()
	if err := run(ctx, *workdir, *dsn, *dryRun, *verbose); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, workdir, dsn string, dryRun, verbose bool) error {
	// Connect to database
	var store persistence.ProjectsStore
	if !dryRun {
		pool, err := pgxpool.New(ctx, dsn)
		if err != nil {
			return fmt.Errorf("connect to postgres: %w", err)
		}
		defer pool.Close()

		store = databases.NewPostgresProjectsStore(pool)
		if err := store.Init(ctx); err != nil {
			return fmt.Errorf("init projects store: %w", err)
		}
	}

	// Scan users directory
	usersDir := filepath.Join(workdir, "users")
	userEntries, err := os.ReadDir(usersDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No users directory found, nothing to migrate")
			return nil
		}
		return fmt.Errorf("read users dir: %w", err)
	}

	var stats struct {
		usersScanned     int
		projectsFound    int
		projectsMigrated int
		filesIndexed     int
		errors           int
	}

	for _, userEntry := range userEntries {
		if !userEntry.IsDir() {
			continue
		}

		userID, err := strconv.ParseInt(userEntry.Name(), 10, 64)
		if err != nil {
			if verbose {
				fmt.Printf("Skipping non-numeric user dir: %s\n", userEntry.Name())
			}
			continue
		}
		stats.usersScanned++

		projectsDir := filepath.Join(usersDir, userEntry.Name(), "projects")
		projectEntries, err := os.ReadDir(projectsDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			fmt.Fprintf(os.Stderr, "warning: cannot read projects for user %d: %v\n", userID, err)
			stats.errors++
			continue
		}

		for _, projectEntry := range projectEntries {
			if !projectEntry.IsDir() {
				continue
			}
			stats.projectsFound++

			projectID := projectEntry.Name()
			projectRoot := filepath.Join(projectsDir, projectID)

			// Read project metadata
			meta, err := readProjectMeta(projectRoot)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: cannot read meta for project %s: %v\n", projectID, err)
				// Use fallback metadata
				info, _ := projectEntry.Info()
				meta = projectMeta{
					ID:        projectID,
					Name:      projectID,
					CreatedAt: time.Now().UTC(),
					UpdatedAt: time.Now().UTC(),
				}
				if info != nil {
					meta.UpdatedAt = info.ModTime().UTC()
				}
			}

			// Compute file stats
			var totalBytes int64
			var fileCount int
			var fileEntries []fileEntry

			err = filepath.WalkDir(projectRoot, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return nil // Skip errors
				}
				// Skip .meta directory
				if d.IsDir() && d.Name() == ".meta" {
					return filepath.SkipDir
				}
				// Skip symlinks
				if d.Type()&fs.ModeSymlink != 0 {
					return nil
				}

				relPath, err := filepath.Rel(projectRoot, path)
				if err != nil {
					return nil
				}
				relPath = filepath.ToSlash(relPath)

				if relPath == "." {
					return nil
				}

				info, err := d.Info()
				if err != nil {
					return nil
				}

				fe := fileEntry{
					path:    relPath,
					name:    d.Name(),
					isDir:   d.IsDir(),
					size:    info.Size(),
					modTime: info.ModTime().UTC(),
				}
				fileEntries = append(fileEntries, fe)

				if !d.IsDir() {
					totalBytes += info.Size()
					fileCount++
				}

				return nil
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: error walking project %s: %v\n", projectID, err)
				stats.errors++
				continue
			}

			if verbose || dryRun {
				fmt.Printf("Project: user=%d id=%s name=%q files=%d bytes=%d\n",
					userID, projectID, meta.Name, fileCount, totalBytes)
			}

			if dryRun {
				stats.projectsMigrated++
				stats.filesIndexed += len(fileEntries)
				continue
			}

			// Create project in database
			// First check if it already exists
			existing, err := store.Get(ctx, userID, projectID)
			if err == nil {
				if verbose {
					fmt.Printf("  Project already exists (revision=%d), updating stats\n", existing.Revision)
				}
				// Update stats only
				if err := store.UpdateStats(ctx, projectID, totalBytes, fileCount); err != nil {
					fmt.Fprintf(os.Stderr, "warning: failed to update stats for %s: %v\n", projectID, err)
					stats.errors++
				}
			} else if errors.Is(err, persistence.ErrNotFound) {
				// Insert new project with preserved ID
				if err := insertProject(ctx, store, userID, projectID, meta.Name, meta.CreatedAt, meta.UpdatedAt, totalBytes, fileCount); err != nil {
					fmt.Fprintf(os.Stderr, "warning: failed to insert project %s: %v\n", projectID, err)
					stats.errors++
					continue
				}
				stats.projectsMigrated++
			} else if errors.Is(err, persistence.ErrForbidden) {
				fmt.Fprintf(os.Stderr, "warning: project %s belongs to different user\n", projectID)
				stats.errors++
				continue
			} else {
				fmt.Fprintf(os.Stderr, "warning: failed to check project %s: %v\n", projectID, err)
				stats.errors++
				continue
			}

			// Index files
			for _, fe := range fileEntries {
				pf := persistence.ProjectFile{
					ProjectID: projectID,
					Path:      fe.path,
					Name:      fe.name,
					IsDir:     fe.isDir,
					Size:      fe.size,
					ModTime:   fe.modTime,
					ETag:      "", // Will be populated on first sync
				}
				if err := store.IndexFile(ctx, pf); err != nil {
					if verbose {
						fmt.Fprintf(os.Stderr, "warning: failed to index file %s: %v\n", fe.path, err)
					}
					stats.errors++
					continue
				}
				stats.filesIndexed++
			}
		}
	}

	fmt.Println("\n--- Migration Summary ---")
	fmt.Printf("Users scanned:     %d\n", stats.usersScanned)
	fmt.Printf("Projects found:    %d\n", stats.projectsFound)
	fmt.Printf("Projects migrated: %d\n", stats.projectsMigrated)
	fmt.Printf("Files indexed:     %d\n", stats.filesIndexed)
	fmt.Printf("Errors:            %d\n", stats.errors)
	if dryRun {
		fmt.Println("\n(dry-run mode - no changes made)")
	}

	return nil
}

type projectMeta struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type fileEntry struct {
	path    string
	name    string
	isDir   bool
	size    int64
	modTime time.Time
}

func readProjectMeta(root string) (projectMeta, error) {
	metaPath := filepath.Join(root, ".meta", "project.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return projectMeta{}, err
	}
	var meta projectMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return projectMeta{}, err
	}
	return meta, nil
}

// insertProject inserts a project with a specific ID (for migration).
// This bypasses the normal Create() which generates a new UUID.
func insertProject(ctx context.Context, store persistence.ProjectsStore, userID int64, projectID, name string, createdAt, updatedAt time.Time, bytes int64, fileCount int) error {
	// Check if the store supports direct ID insertion (postgres implementation)
	type directInserter interface {
		InsertWithID(ctx context.Context, userID int64, projectID, name string, createdAt, updatedAt time.Time, bytes int64, fileCount int) error
	}

	if di, ok := store.(directInserter); ok {
		return di.InsertWithID(ctx, userID, projectID, name, createdAt, updatedAt, bytes, fileCount)
	}

	return fmt.Errorf("store does not support InsertWithID - migration requires postgres store")
}
