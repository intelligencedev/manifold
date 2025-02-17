// sefii/pathingest.go
package sefii

import (
	"context"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// IngestPath traverses the given file system path and ingests text files
// using the provided SEFII engine. The function determines the MIME type
// of each file and only ingests those whose MIME type starts with "text/".
func IngestPath(
	ctx context.Context,
	localPath string,
	engine *Engine,
	embeddingsHost, apiKey string,
	chunkSize, chunkOverlap int,
) error {
	// Walk the file tree rooted at localPath.
	err := filepath.Walk(localPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories.
		if info.IsDir() {
			return nil
		}

		// Determine the file's relative path (used as metadata).
		relPath, err := filepath.Rel(localPath, path)
		if err != nil {
			return err
		}

		// Read the file content.
		data, err := ioutil.ReadFile(path)
		if err != nil {
			log.Printf("Error reading file %s: %v", path, err)
			return nil // Skip this file if an error occurs.
		}

		// Determine MIME type using the first 512 bytes.
		sampleSize := 512
		if len(data) < sampleSize {
			sampleSize = len(data)
		}
		mimeType := http.DetectContentType(data[:sampleSize])
		if !strings.HasPrefix(mimeType, "text/") {
			log.Printf("Skipping non-text file: %s (mime: %s)", relPath, mimeType)
			return nil
		}

		log.Printf("Ingesting text file: %s", relPath)
		// Ingest file content via the SEFII engine.
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
