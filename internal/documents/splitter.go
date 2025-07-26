package documents

import (
	"bufio"
	"io"
	"strings"
)

// Chunk represents a piece of text with token offsets.
type Chunk struct {
	Index      int
	Text       string
	StartToken int
	EndToken   int
}

// Splitter breaks a stream of text into chunks.
type Splitter struct {
	MaxTokens     int
	OverlapTokens int
	Lang          Language
	Tok           Tokenizer
}

// Stream reads from r and emits chunks via emit.
func (s Splitter) Stream(r io.Reader, emit func(Chunk) error) error {
	if s.Tok == nil {
		s.Tok = RuneTokenizer{}
	}
	scanner := bufio.NewScanner(r)
	buf := strings.Builder{}
	curTokens := 0
	start := 0
	idx := 0
	for scanner.Scan() {
		line := scanner.Text()
		tokens := s.Tok.Count(line) + 1 // newline token
		boundary := false
		switch s.Lang {
		case Markdown:
			boundary = strings.HasPrefix(line, "#")
		case Go:
			boundary = strings.HasPrefix(strings.TrimSpace(line), "func ")
		}

		if curTokens+tokens > s.MaxTokens || (boundary && curTokens > 0) {
			if err := emit(Chunk{Index: idx, Text: buf.String(), StartToken: start, EndToken: start + curTokens}); err != nil {
				return err
			}
			idx++
			start += curTokens - s.OverlapTokens
			text := lastTokens(buf.String(), s.Tok, s.OverlapTokens)
			buf.Reset()
			buf.WriteString(text)
			curTokens = s.Tok.Count(text)
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
		curTokens += tokens
	}
	if buf.Len() > 0 {
		emit(Chunk{Index: idx, Text: buf.String(), StartToken: start, EndToken: start + curTokens})
	}
	return scanner.Err()
}

func lastTokens(text string, tok Tokenizer, n int) string {
	if n <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= n {
		return text
	}
	return string(runes[len(runes)-n:])
}
