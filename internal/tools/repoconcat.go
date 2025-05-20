// internal/tools/repoconcat.go
package tools

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// FileProcessor interface for processing files
type FileProcessor interface {
	Process(path string) error
	GetContent() string
}

// TextFileProcessor processes text files
type TextFileProcessor struct {
	content string
}

func (tfp *TextFileProcessor) Process(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	tfp.content = string(content)
	return nil
}

func (tfp *TextFileProcessor) GetContent() string {
	return tfp.content
}

// Formatter interface for formatting output
type Formatter interface {
	Format(fileStructure map[string][]string, fileContents map[string]string) string
}

// PlainTextFormatter formats output as plain text
type PlainTextFormatter struct{}

func (ptf *PlainTextFormatter) Format(fileStructure map[string][]string, fileContents map[string]string) string {
	var builder strings.Builder
	for path, files := range fileStructure {
		builder.WriteString(path + "/\n")
		for _, file := range files {
			filePath := filepath.Join(path, file)
			if _, exists := fileContents[filePath]; exists {
				builder.WriteString("├── " + file + "\n")
			}
		}
	}
	return builder.String()
}

// RepoConcat is the main struct for the repoconcat functionality.
type RepoConcat struct {
	Timeout   time.Duration
	Formatter Formatter
}

// NewRepoConcat creates a new RepoConcat instance with default values.
func NewRepoConcat() *RepoConcat {
	return &RepoConcat{
		Timeout:   30 * time.Second,
		Formatter: &PlainTextFormatter{},
	}
}

// Concatenate processes the given paths and returns the concatenated output.
func (rc *RepoConcat) Concatenate(paths []string, types []string, recursive bool, ignorePattern string) (string, error) {
	// Create a map to store file contents
	fileContents := make(map[string]string)
	// Create a map to store file structure
	fileStructure := make(map[string][]string)

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), rc.Timeout)
	defer cancel()

	// Create a WaitGroup to wait for all goroutines to finish
	var wg sync.WaitGroup

	// Loop over each path
	for _, path := range paths {
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			err := rc.processPath(ctx, path, types, recursive, ignorePattern, fileContents, fileStructure)
			if err != nil {
				log.Printf("Error processing path %s: %v\n", path, err) // Log, but don't return
			}
		}(path)
	}

	// Wait for all goroutines to finish
	wg.Wait()

	// Check for context error after goroutines finish
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	// Generate the file/folder structure string
	structureString := rc.Formatter.Format(fileStructure, fileContents)

	// Concatenate file contents with delimiters
	var concatenated strings.Builder
	concatenated.WriteString(structureString)
	for filePath, content := range fileContents {
		concatenated.WriteString(fmt.Sprintf("--- BEGIN %s ---\n", filePath))
		concatenated.WriteString(content)
		concatenated.WriteString(fmt.Sprintf("--- END %s ---\n\n", filePath))
	}

	return concatenated.String(), nil
}

func (rc *RepoConcat) processPath(ctx context.Context, path string, typeList []string, recursive bool, ignorePattern string, fileContents map[string]string, fileStructure map[string][]string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		files, err := os.ReadDir(path)
		if err != nil {
			return err
		}

		for _, file := range files {
			filePath := filepath.Join(path, file.Name())

			if file.IsDir() {
				if recursive {
					err := rc.processPath(ctx, filePath, typeList, recursive, ignorePattern, fileContents, fileStructure)
					if err != nil {
						return err
					}
				}
			} else if hasMatchingExtension(filePath, typeList) && !shouldIgnore(file.Name(), ignorePattern) {
				processor := &TextFileProcessor{}
				err := processor.Process(filePath)
				if err != nil {
					return err
				}

				// Store file contents in the map
				fileContents[filePath] = processor.GetContent()
				fileStructure[path] = append(fileStructure[path], file.Name())
			}
		}

		return nil
	}
}

func hasMatchingExtension(filePath string, extensions []string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	for _, e := range extensions {
		if strings.ToLower(e) == ext {
			return true
		}
	}
	return false
}

func shouldIgnore(fileName, ignorePattern string) bool {
	if ignorePattern == "" {
		return false
	}
	return strings.Contains(fileName, ignorePattern)
}
