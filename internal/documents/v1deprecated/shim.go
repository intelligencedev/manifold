package v1deprecated

import (
	"context"
	doc "manifold/internal/documents"
)

// Re-export types for backwards compatibility.
type FileData = doc.FileData
type Language = doc.Language

func DeduceLanguage(p string) Language { return doc.DeduceLanguage(p) }

func GetGitFiles(repo string) ([]FileData, error) {
	r := doc.NewGitReader(repo)
	ch := make(chan FileData, 100)
	go func() {
		defer close(ch)
		_ = r.Stream(context.Background(), ch)
	}()
	var out []FileData
	for f := range ch {
		out = append(out, f)
	}
	return out, nil
}

func GetFilesInDir(dir string) ([]FileData, error) {
	r := doc.NewDirReader(dir)
	ch := make(chan FileData, 100)
	go func() {
		defer close(ch)
		_ = r.Stream(context.Background(), ch)
	}()
	var out []FileData
	for f := range ch {
		out = append(out, f)
	}
	return out, nil
}

// SplitTextByCount splits text into chunks of the given maximum size without overlap.
func SplitTextByCount(text string, chunkSize int) []string {
	// Use default language splitter with zero overlap
	splitter := &RecursiveCharacterTextSplitter{
		ChunkSize:   chunkSize,
		OverlapSize: 0,
		Lang:        DEFAULT,
	}
	return splitter.SplitText(text)
}
