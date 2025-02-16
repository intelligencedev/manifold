package documents

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// FileData represents a file's path and its content.
type FileData struct {
	Path    string `json:"path"`
	Content string `json:"content"`
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
