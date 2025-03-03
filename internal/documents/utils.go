package documents

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Language is a type that represents a programming language.
type Language string

const (
	PYTHON   Language = "PYTHON"
	GO       Language = "GO"
	HTML     Language = "HTML"
	JS       Language = "JS"
	TS       Language = "TS"
	MARKDOWN Language = "MARKDOWN"
	JSON     Language = "JSON"
	DEFAULT  Language = "DEFAULT"
)

// FileData represents a file's path and its content.
type FileData struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// isTextFile checks if a file's content appears to be text.
func IsTextFile(data []byte) bool {
	// A simple heuristic: if the file contains a null byte, consider it binary.
	return !strings.Contains(string(data), "\x00")
}

// deduceLanguage inspects the file extension and returns a Language.
func DeduceLanguage(filePath string) Language {
	switch {
	case strings.HasSuffix(filePath, ".go"):
		return GO
	case strings.HasSuffix(filePath, ".py"):
		return PYTHON
	case strings.HasSuffix(filePath, ".md"):
		return MARKDOWN
	case strings.HasSuffix(filePath, ".html"):
		return HTML
	case strings.HasSuffix(filePath, ".js"):
		return JS
	case strings.HasSuffix(filePath, ".ts"):
		return TS
	case strings.HasSuffix(filePath, ".json"):
		return JSON
	default:
		return DEFAULT
	}
}

// GetGitFiles retrieves all text files tracked by Git in a given repository.
func GetGitFiles(repoPath string) ([]FileData, error) {
	// Ensure that repoPath exists and is a directory.
	info, err := os.Stat(repoPath)
	if err != nil || !info.IsDir() {
		return nil, err
	}

	// Run `git ls-files` to list tracked files in the repo.
	cmd := exec.Command("git", "-C", repoPath, "ls-files")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	// Parse the output into file paths.
	relativePaths := strings.Split(strings.TrimSpace(string(output)), "\n")
	files := make([]FileData, 0, len(relativePaths))

	// Iterate over the files, read content, and filter out non-text files.
	for _, relPath := range relativePaths {
		fullPath := filepath.Join(repoPath, relPath)
		contentBytes, err := os.ReadFile(fullPath)
		if err != nil {
			continue // Skip unreadable files
		}

		// Check if it's a text file.
		if !IsTextFile(contentBytes) {
			continue // Skip binary files
		}

		files = append(files, FileData{
			Path:    relPath,
			Content: string(contentBytes),
		})
	}

	return files, nil
}

// GetFilesInDir retrieves all text files in a given directory.
func GetFilesInDir(dirPath string) ([]FileData, error) {
	// Ensure that dirPath exists and is a directory.
	info, err := os.Stat(dirPath)
	if err != nil || !info.IsDir() {
		return nil, err
	}

	// Walk the directory to get all files.
	files := make([]FileData, 0)
	err = filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and hidden files
		if info.IsDir() || strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		contentBytes, err := os.ReadFile(path)
		if err != nil {
			return nil // Skip unreadable files
		}

		// Check if it's a text file.
		if !IsTextFile(contentBytes) {
			return nil // Skip binary files
		}

		files = append(files, FileData{
			Path:    path,
			Content: string(contentBytes),
		})

		return nil
	})

	if err != nil {
		return nil, err
	}

	return files, nil
}
