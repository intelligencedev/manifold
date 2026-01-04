// migrateprojects-s3 migrates existing filesystem-based projects to S3 storage.
//
// It scans the local filesystem for project files and uploads them to S3/MinIO,
// preserving the same directory structure. Projects are then marked as using
// the S3 backend in the database.
//
// Usage:
//
//	go run cmd/migrateprojects-s3/main.go [flags]
//
// Flags:
//
//	-workdir string
//	    Base workdir containing users/<uid>/projects/<pid> (required or via WORKDIR env)
//	-dsn string
//	    PostgreSQL connection string (required or via DATABASE_URL env)
//	-endpoint string
//	    S3 endpoint URL (required or via PROJECTS_S3_ENDPOINT env)
//	-bucket string
//	    S3 bucket name (required or via PROJECTS_S3_BUCKET env)
//	-prefix string
//	    S3 key prefix (default: "workspaces" or via PROJECTS_S3_PREFIX env)
//	-access-key string
//	    S3 access key (required or via PROJECTS_S3_ACCESS_KEY env)
//	-secret-key string
//	    S3 secret key (required or via PROJECTS_S3_SECRET_KEY env)
//	-region string
//	    S3 region (default: "us-east-1" or via PROJECTS_S3_REGION env)
//	-path-style
//	    Use path-style addressing (required for MinIO)
//	-dry-run
//	    Print what would be migrated without making changes
//	-verbose
//	    Print detailed progress
//	-project string
//	    Migrate only a specific project ID (optional)
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	appconfig "manifold/internal/config"
	"manifold/internal/objectstore"
	"manifold/internal/persistence"
	"manifold/internal/persistence/databases"
)

func main() {
	workdir := flag.String("workdir", os.Getenv("WORKDIR"), "Base workdir (WORKDIR env)")
	dsn := flag.String("dsn", os.Getenv("DATABASE_URL"), "Postgres DSN (DATABASE_URL env)")
	endpoint := flag.String("endpoint", os.Getenv("PROJECTS_S3_ENDPOINT"), "S3 endpoint URL")
	bucket := flag.String("bucket", os.Getenv("PROJECTS_S3_BUCKET"), "S3 bucket name")
	prefix := flag.String("prefix", envOrDefault("PROJECTS_S3_PREFIX", "workspaces"), "S3 key prefix")
	accessKey := flag.String("access-key", os.Getenv("PROJECTS_S3_ACCESS_KEY"), "S3 access key")
	secretKey := flag.String("secret-key", os.Getenv("PROJECTS_S3_SECRET_KEY"), "S3 secret key")
	region := flag.String("region", envOrDefault("PROJECTS_S3_REGION", "us-east-1"), "S3 region")
	pathStyle := flag.Bool("path-style", envBool("PROJECTS_S3_USE_PATH_STYLE"), "Use path-style addressing")
	dryRun := flag.Bool("dry-run", false, "Print what would be migrated without making changes")
	verbose := flag.Bool("verbose", false, "Print detailed progress")
	projectFilter := flag.String("project", "", "Migrate only a specific project ID")
	flag.Parse()

	if *workdir == "" {
		fmt.Fprintln(os.Stderr, "error: -workdir or WORKDIR env required")
		os.Exit(1)
	}
	if *dsn == "" {
		fmt.Fprintln(os.Stderr, "error: -dsn or DATABASE_URL env required")
		os.Exit(1)
	}
	if *endpoint == "" {
		fmt.Fprintln(os.Stderr, "error: -endpoint or PROJECTS_S3_ENDPOINT env required")
		os.Exit(1)
	}
	if *bucket == "" {
		fmt.Fprintln(os.Stderr, "error: -bucket or PROJECTS_S3_BUCKET env required")
		os.Exit(1)
	}
	if *accessKey == "" {
		fmt.Fprintln(os.Stderr, "error: -access-key or PROJECTS_S3_ACCESS_KEY env required")
		os.Exit(1)
	}
	if *secretKey == "" {
		fmt.Fprintln(os.Stderr, "error: -secret-key or PROJECTS_S3_SECRET_KEY env required")
		os.Exit(1)
	}

	ctx := context.Background()
	if err := run(ctx, config{
		workdir:       *workdir,
		dsn:           *dsn,
		endpoint:      *endpoint,
		bucket:        *bucket,
		prefix:        *prefix,
		accessKey:     *accessKey,
		secretKey:     *secretKey,
		region:        *region,
		pathStyle:     *pathStyle,
		dryRun:        *dryRun,
		verbose:       *verbose,
		projectFilter: *projectFilter,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

type config struct {
	workdir       string
	dsn           string
	endpoint      string
	bucket        string
	prefix        string
	accessKey     string
	secretKey     string
	region        string
	pathStyle     bool
	dryRun        bool
	verbose       bool
	projectFilter string
}

func run(ctx context.Context, cfg config) error {
	// Connect to database
	var store persistence.ProjectsStore
	var pool *pgxpool.Pool

	if !cfg.dryRun {
		var err error
		pool, err = pgxpool.New(ctx, cfg.dsn)
		if err != nil {
			return fmt.Errorf("connect to postgres: %w", err)
		}
		defer pool.Close()

		store = databases.NewPostgresProjectsStore(pool)
		if err := store.Init(ctx); err != nil {
			return fmt.Errorf("init projects store: %w", err)
		}
	}

	// Create S3 client
	var s3Client objectstore.ObjectStore
	if !cfg.dryRun {
		var err error
		s3Client, err = objectstore.NewS3Store(ctx, appconfig.S3Config{
			Endpoint:     cfg.endpoint,
			Region:       cfg.region,
			Bucket:       cfg.bucket,
			Prefix:       cfg.prefix,
			AccessKey:    cfg.accessKey,
			SecretKey:    cfg.secretKey,
			UsePathStyle: cfg.pathStyle,
		})
		if err != nil {
			return fmt.Errorf("create S3 client: %w", err)
		}
	}

	// Scan users directory
	usersDir := filepath.Join(cfg.workdir, "users")
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
		filesUploaded    int
		bytesUploaded    int64
		filesSkipped     int
		errors           int
	}

	for _, userEntry := range userEntries {
		if !userEntry.IsDir() {
			continue
		}

		userID, err := strconv.ParseInt(userEntry.Name(), 10, 64)
		if err != nil {
			if cfg.verbose {
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

			projectID := projectEntry.Name()

			// Skip if filtering to specific project
			if cfg.projectFilter != "" && projectID != cfg.projectFilter {
				continue
			}

			stats.projectsFound++

			projectRoot := filepath.Join(projectsDir, projectID)

			// Validate UUID format
			if !isValidUUID(projectID) {
				fmt.Fprintf(os.Stderr, "warning: skipping invalid project ID %s (not a valid UUID)\n", projectID)
				stats.errors++
				continue
			}

			if cfg.verbose || cfg.dryRun {
				fmt.Printf("\nProject: user=%d id=%s\n", userID, projectID)
			}

			// Collect files to upload
			var files []fileInfo
			err = filepath.WalkDir(projectRoot, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return nil // Skip errors
				}
				// Skip .meta directory (will be handled separately)
				if d.IsDir() && d.Name() == ".meta" {
					return filepath.SkipDir
				}
				// Skip symlinks
				if d.Type()&fs.ModeSymlink != 0 {
					return nil
				}
				// Skip directories (S3 doesn't need them)
				if d.IsDir() {
					return nil
				}

				relPath, _ := filepath.Rel(projectRoot, path)
				relPath = filepath.ToSlash(relPath)

				info, err := d.Info()
				if err != nil {
					return nil
				}

				files = append(files, fileInfo{
					localPath: path,
					relPath:   relPath,
					size:      info.Size(),
					modTime:   info.ModTime(),
				})

				return nil
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: error walking project %s: %v\n", projectID, err)
				stats.errors++
				continue
			}

			if cfg.verbose || cfg.dryRun {
				fmt.Printf("  Files to upload: %d\n", len(files))
			}

			if cfg.dryRun {
				for _, f := range files {
					s3Key := buildS3Key(cfg.prefix, userID, projectID, f.relPath)
					fmt.Printf("  [dry-run] %s -> s3://%s/%s (%d bytes)\n",
						f.relPath, cfg.bucket, s3Key, f.size)
					stats.bytesUploaded += f.size
				}
				stats.projectsMigrated++
				stats.filesUploaded += len(files)
				continue
			}

			// Upload files to S3
			projectFilesUploaded := 0
			projectErrors := 0

			for _, f := range files {
				s3Key := buildS3Key(cfg.prefix, userID, projectID, f.relPath)

				// Check if file already exists with same size (simple skip logic)
				attrs, err := s3Client.Head(ctx, s3Key)
				if err == nil && attrs.Size == f.size {
					if cfg.verbose {
						fmt.Printf("  [skip] %s (already exists)\n", f.relPath)
					}
					stats.filesSkipped++
					continue
				}

				// Read file
				data, err := os.ReadFile(f.localPath)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  warning: cannot read %s: %v\n", f.relPath, err)
					projectErrors++
					continue
				}

				// Calculate SHA256 for verification
				hash := sha256.Sum256(data)
				expectedHash := hex.EncodeToString(hash[:])

				// Upload to S3
				_, err = s3Client.Put(ctx, s3Key, bytes.NewReader(data), objectstore.PutOptions{})
				if err != nil {
					fmt.Fprintf(os.Stderr, "  warning: upload failed %s: %v\n", f.relPath, err)
					projectErrors++
					continue
				}

				// Verify upload
				uploadedAttrs, err := s3Client.Head(ctx, s3Key)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  warning: verify failed %s: %v\n", f.relPath, err)
					projectErrors++
					continue
				}

				if uploadedAttrs.Size != f.size {
					fmt.Fprintf(os.Stderr, "  warning: size mismatch %s: expected %d, got %d\n",
						f.relPath, f.size, uploadedAttrs.Size)
					projectErrors++
					continue
				}

				if cfg.verbose {
					fmt.Printf("  [ok] %s (%d bytes, sha256=%s)\n", f.relPath, f.size, expectedHash[:16])
				}

				projectFilesUploaded++
				stats.bytesUploaded += f.size

				// Update file index in database
				if store != nil {
					pf := persistence.ProjectFile{
						ProjectID: projectID,
						Path:      f.relPath,
						Name:      filepath.Base(f.relPath),
						IsDir:     false,
						Size:      f.size,
						ModTime:   f.modTime,
						ETag:      uploadedAttrs.ETag,
					}
					if err := store.IndexFile(ctx, pf); err != nil {
						if cfg.verbose {
							fmt.Fprintf(os.Stderr, "  warning: index failed %s: %v\n", f.relPath, err)
						}
					}
				}
			}

			stats.filesUploaded += projectFilesUploaded
			stats.errors += projectErrors

			// Update project storage_backend in database
			if store != nil && projectErrors == 0 {
				if err := updateProjectBackend(ctx, pool, projectID, "s3"); err != nil {
					fmt.Fprintf(os.Stderr, "  warning: failed to update project backend: %v\n", err)
					stats.errors++
				} else {
					stats.projectsMigrated++
					if cfg.verbose {
						fmt.Printf("  Project marked as S3 backend\n")
					}
				}
			} else if projectErrors > 0 {
				fmt.Fprintf(os.Stderr, "  Project %s had %d errors, not marking as migrated\n",
					projectID, projectErrors)
			}
		}
	}

	fmt.Println("\n--- S3 Migration Summary ---")
	fmt.Printf("Users scanned:      %d\n", stats.usersScanned)
	fmt.Printf("Projects found:     %d\n", stats.projectsFound)
	fmt.Printf("Projects migrated:  %d\n", stats.projectsMigrated)
	fmt.Printf("Files uploaded:     %d\n", stats.filesUploaded)
	fmt.Printf("Files skipped:      %d\n", stats.filesSkipped)
	fmt.Printf("Bytes uploaded:     %s\n", formatBytes(stats.bytesUploaded))
	fmt.Printf("Errors:             %d\n", stats.errors)
	if cfg.dryRun {
		fmt.Println("\n(dry-run mode - no changes made)")
	}

	return nil
}

type fileInfo struct {
	localPath string
	relPath   string
	size      int64
	modTime   time.Time
}

func buildS3Key(prefix string, userID int64, projectID, relPath string) string {
	// Key format: {prefix}/users/{uid}/projects/{pid}/files/{path}
	return fmt.Sprintf("%s/users/%d/projects/%s/files/%s", prefix, userID, projectID, relPath)
}

func updateProjectBackend(ctx context.Context, pool *pgxpool.Pool, projectID, backend string) error {
	_, err := pool.Exec(ctx, `
UPDATE projects SET storage_backend = $1, updated_at = NOW() WHERE id = $2`,
		backend, projectID)
	return err
}

func isValidUUID(s string) bool {
	// Simple UUID validation: 8-4-4-4-12 hex format
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}
	return true
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func envOrDefault(key, defaultVal string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return defaultVal
}

func envBool(key string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	return v == "true" || v == "1" || v == "yes"
}
