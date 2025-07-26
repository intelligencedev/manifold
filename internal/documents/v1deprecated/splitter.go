package v1deprecated

import (
	"strings"

	doc "manifold/internal/documents"
)

const (
	GO       = Language(doc.Go)
	MARKDOWN = Language(doc.Markdown)
	DEFAULT  = Language(doc.Plain)
)

// RecursiveCharacterTextSplitter is a thin wrapper around the v2 Splitter.
type RecursiveCharacterTextSplitter struct {
	ChunkSize   int
	OverlapSize int
	Lang        Language
}

func FromLanguage(l Language) (*RecursiveCharacterTextSplitter, error) {
	return &RecursiveCharacterTextSplitter{ChunkSize: 1000, OverlapSize: 100, Lang: l}, nil
}

// SplitText splits using the v2 splitter.
func (r *RecursiveCharacterTextSplitter) SplitText(text string) []string {
	s := doc.Splitter{MaxTokens: r.ChunkSize, OverlapTokens: r.OverlapSize, Lang: doc.Language(r.Lang)}
	var chunks []string
	s.Stream(strings.NewReader(text), func(c doc.Chunk) error {
		chunks = append(chunks, c.Text)
		return nil
	})
	return chunks
}

func (r *RecursiveCharacterTextSplitter) AdaptiveSplit(text string) []string {
	return r.SplitText(text)
}
