package documents

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// FileData holds a file path and its textual content.
type FileData struct {
	Path    string
	Content string
}

// FileReader walks a directory tree and streams text files.
type FileReader struct {
	root   string
	listFn func(string) ([]string, error)
}

// NewDirReader creates a reader for a directory tree.
func NewDirReader(root string) *FileReader {
	return &FileReader{root: root, listFn: listDir}
}

// NewGitReader creates a reader for a Git repository.
// Actual git listing is left as a placeholder and should be replaced by humans.
func NewGitReader(repoDir string) *FileReader {
	return &FileReader{root: repoDir, listFn: listGit}
}

func listDir(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		files = append(files, path)
		return nil
	})
	return files, err
}

func listGit(root string) ([]string, error) {
	// Placeholder: real git integration should be injected by humans.
	return listDir(root)
}

// Stream sends FileData for each textual file found under root.
func (f *FileReader) Stream(ctx context.Context, out chan<- FileData) error {
	paths, err := f.listFn(f.root)
	if err != nil {
		return err
	}
	for _, p := range paths {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		rel := p
		if abs, err := filepath.Abs(p); err == nil {
			if rabs, err := filepath.Abs(f.root); err == nil {
				rel, _ = filepath.Rel(rabs, abs)
			}
		}
		file, err := os.Open(p)
		if err != nil {
			continue
		}
		r := bufio.NewReader(file)
		peek, _ := r.Peek(512 * 1024)
		if isBinary(peek) {
			file.Close()
			continue
		}
		data, err := io.ReadAll(r)
		file.Close()
		if err != nil {
			continue
		}
		out <- FileData{Path: rel, Content: string(data)}
	}
	return nil
}

func isBinary(buf []byte) bool {
	if strings.ContainsRune(string(buf), '\x00') {
		return true
	}
	ct := http.DetectContentType(buf)
	return !strings.HasPrefix(ct, "text/") && ct != "application/json"
}
