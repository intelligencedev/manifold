package documents

import (
	"bytes"
	"testing"
)

func TestSplitterOverlap(t *testing.T) {
	s := Splitter{MaxTokens: 5, OverlapTokens: 2, Tok: RuneTokenizer{}}
	input := "a b c d e f g"
	var chunks []Chunk
	err := s.Stream(bytes.NewBufferString(input), func(c Chunk) error {
		chunks = append(chunks, c)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) == 0 {
		t.Fatal("no chunks")
	}
	for i := 1; i < len(chunks); i++ {
		prev := chunks[i-1]
		cur := chunks[i]
		if cur.StartToken-prev.EndToken != -s.OverlapTokens {
			t.Fatalf("overlap not preserved between chunk %d and %d", i-1, i)
		}
		if cur.EndToken-cur.StartToken > s.MaxTokens {
			t.Fatalf("chunk %d exceeds max tokens", i)
		}
	}
}
