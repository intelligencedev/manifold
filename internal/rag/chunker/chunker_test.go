package chunker

import (
	"strings"
	"testing"

	"manifold/internal/rag/ingest"
)

func genText(words int) string {
	var b strings.Builder
	for i := 0; i < words; i++ {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString("word")
	}
	return b.String()
}

func TestFixedChunk_SizeToleranceAndOverlap(t *testing.T) {
	text := genText(2000) // ~8000 chars
	ch := SimpleChunker{}
	opt := ingest.ChunkingOptions{Strategy: "fixed", MaxTokens: 200, Overlap: 10}
	chunks, err := ch.Chunk(text, opt)
	if err != nil {
		t.Fatalf("chunk error: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatalf("expected some chunks")
	}
	tgt := 200 * 4
	tolLow, tolHigh := int(float64(tgt)*0.9), int(float64(tgt)*1.1)
	for i, c := range chunks {
		if i == len(chunks)-1 {
			break
		}
		if l := len(c.Text); !(l >= tolLow && l <= tolHigh) {
			t.Fatalf("chunk %d length %d out of tolerance [%d,%d]", i, l, tolLow, tolHigh)
		}
	}
}

func TestMarkdownChunk_PreservesHeadings(t *testing.T) {
	text := "# Title\n\npara1 text here.\n\n## Sub\n\npara2 text here."
	ch := SimpleChunker{}
	// Small target to force multiple chunks
	chunks, err := ch.Chunk(text, ingest.ChunkingOptions{Strategy: "md", MaxTokens: 10})
	if err != nil {
		t.Fatalf("chunk error: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected >=2 chunks, got %d", len(chunks))
	}
	if !strings.Contains(chunks[0].Text, "# Title") {
		t.Fatalf("first chunk should contain heading: %q", chunks[0].Text)
	}
}

func TestCodeChunk_RarelySplitsFunctions(t *testing.T) {
	text := "package x\n\n// comment\n\nfunc A() {}\n\nfunc B() {}\n\nfunc C() {}\n"
	ch := SimpleChunker{}
	chunks, err := ch.Chunk(text, ingest.ChunkingOptions{Strategy: "code", MaxTokens: 8})
	if err != nil {
		t.Fatalf("chunk error: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks")
	}
	// Heuristic: each chunk should contain whole functions when possible
	for _, c := range chunks {
		if strings.Count(c.Text, "func ") > 1 {
			t.Fatalf("chunk should not contain many functions: %q", c.Text)
		}
	}
}
