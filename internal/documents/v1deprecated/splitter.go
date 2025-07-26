package v1deprecated

import (
	"strings"

	doc "manifold/internal/documents"
)

const (
	GO         = Language(doc.Go)
	PYTHON     = Language(doc.Python)
	JAVASCRIPT = Language(doc.JavaScript)
	TYPESCRIPT = Language(doc.TypeScript)
	JAVA       = Language(doc.Java)
	CSHARP     = Language(doc.CSharp)
	RUST       = Language(doc.Rust)
	CPP        = Language(doc.Cpp)
	C          = Language(doc.C)
	MARKDOWN   = Language(doc.Markdown)
	JSON       = Language(doc.JSON)
	YAML       = Language(doc.YAML)
	XML        = Language(doc.XML)
	HTML       = Language(doc.HTML)
	CSS        = Language(doc.CSS)
	SQL        = Language(doc.SQL)
	SHELL      = Language(doc.Shell)
	DEFAULT    = Language(doc.Plain)
)

// RecursiveCharacterTextSplitter is a thin wrapper around the v2 Splitter.
type RecursiveCharacterTextSplitter struct {
	ChunkSize   int
	OverlapSize int
	Lang        Language
	UseAdvanced bool // Enable advanced structure-aware splitting
}

func FromLanguage(l Language) (*RecursiveCharacterTextSplitter, error) {
	// Enable advanced splitting for programming languages by default
	useAdvanced := l != DEFAULT && l != MARKDOWN
	return &RecursiveCharacterTextSplitter{
		ChunkSize:   1000,
		OverlapSize: 100,
		Lang:        l,
		UseAdvanced: useAdvanced,
	}, nil
}

// SplitText splits using the v2 splitter.
func (r *RecursiveCharacterTextSplitter) SplitText(text string) []string {
	if r.UseAdvanced {
		// Use advanced structure-aware splitter
		splitter := doc.NewAdvancedSplitter(r.ChunkSize, r.OverlapSize, doc.Language(r.Lang))
		return splitter.StructureAwareSplit(text)
	}

	// Fall back to basic splitter
	s := doc.Splitter{MaxTokens: r.ChunkSize, OverlapTokens: r.OverlapSize, Lang: doc.Language(r.Lang)}
	var chunks []string
	s.Stream(strings.NewReader(text), func(c doc.Chunk) error {
		chunks = append(chunks, c.Text)
		return nil
	})
	return chunks
}

func (r *RecursiveCharacterTextSplitter) AdaptiveSplit(text string) []string {
	// For adaptive split, always use advanced splitting if available
	if r.Lang != DEFAULT {
		splitter := doc.NewAdvancedSplitter(r.ChunkSize, r.OverlapSize, doc.Language(r.Lang))
		return splitter.StructureAwareSplit(text)
	}
	return r.SplitText(text)
}
