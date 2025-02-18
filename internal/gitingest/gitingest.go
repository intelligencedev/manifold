// internal/gitingest/gitingest.go
package gitingest

import (
	"bufio"
	"context"
	"log"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"manifold/internal/sefii"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

// CloneAndIngestRepo clones (or opens) a Git repository, honors its .gitignore,
// and traverses the repository to ingest test documents (txt and code files)
// using the provided SEFII engine. The engine uses PGVector for storage.
func CloneAndIngestRepo(
	ctx context.Context,
	repoURL, localPath string,
	engine *sefii.Engine,
	embeddingsHost, apiKey string,
	chunkSize, chunkOverlap int,
) error {
	var repo *git.Repository
	var err error

	// Clone the repository if the local path does not exist.
	if _, err = os.Stat(localPath); os.IsNotExist(err) {
		log.Printf("Cloning repository %s into %s", repoURL, localPath)
		repo, err = git.PlainClone(localPath, false, &git.CloneOptions{
			URL:      repoURL,
			Progress: os.Stdout,
		})
		if err != nil {
			return err
		}
	} else {
		log.Printf("Opening existing repository at %s", localPath)
		repo, err = git.PlainOpen(localPath)
		if err != nil {
			return err
		}
	}

	// Log repository HEAD (if needed for metadata).
	headRef, err := repo.Head()
	if err == nil {
		log.Printf("Repository HEAD: %s", headRef.Hash())
	}

	// Load and parse .gitignore file from the repository root.
	var matcher gitignore.Matcher
	gitignorePath := filepath.Join(localPath, ".gitignore")
	f, err := os.Open(gitignorePath)
	if err == nil {
		defer f.Close()

		var patterns []gitignore.Pattern
		scanner := bufio.NewScanner(f)

		for scanner.Scan() {
			line := scanner.Text()
			// Parse each line to get a Pattern.
			p := gitignore.ParsePattern(line, nil)
			patterns = append(patterns, p)
		}
		if scanErr := scanner.Err(); scanErr != nil {
			// Log or handle any scanning errors
			log.Printf("Error reading .gitignore lines: %v", scanErr)
		}

		// Build a matcher from all the patterns.
		matcher = gitignore.NewMatcher(patterns)
	} else {
		log.Printf("No .gitignore file found or error opening it: %v", err)
	}

	// Walk the repository's file tree.
	err = filepath.Walk(localPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Skip directories.
		if info.IsDir() {
			return nil
		}

		// Determine the file's relative path.
		relPath, err := filepath.Rel(localPath, path)
		if err != nil {
			return err
		}

		// Check if it should be ignored according to .gitignore.
		if matcher != nil {
			splitPath := strings.Split(relPath, string(os.PathSeparator))
			if matcher.Match(splitPath, info.IsDir()) {
				return nil
			}
		}

		// Use the mime.TypeByExtension function to check if a file is text
		ext := filepath.Ext(path)
		if strings.HasPrefix(mime.TypeByExtension(ext), "text/") {
			return nil
		}

		// Read the file content.
		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("Error reading file %s: %v", path, err)
			return nil // Skip file if error occurs.
		}

		log.Printf("Ingesting file: %s", relPath)
		// Ingest file content via the SEFII engine (using relPath as metadata).
		if err := engine.IngestDocument(
			ctx,
			string(data),
			"DEFAULT",
			relPath,
			embeddingsHost,
			apiKey,
			chunkSize,
			chunkOverlap,
		); err != nil {
			log.Printf("Error ingesting file %s: %v", relPath, err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	return nil
}
