package textsplitters

import (
	"unicode/utf8"
)

// FixedConfig configures the fixed-length splitter.
type FixedConfig struct {
	// Unit chooses chars or tokens.
	Unit Unit
	// Size is the chunk length in the given Unit. Must be > 0.
	Size int
	// Overlap defines how much adjacent chunks overlap, in the same Unit.
	// Must be >= 0 and < Size to make progress; values >= Size are clamped to Size-1.
	Overlap int
	// Tokenizer is required when Unit is UnitTokens. If nil and UnitTokens is
	// selected, a WhitespaceTokenizer is used by default.
	Tokenizer Tokenizer
}

type fixedSplitter struct {
	unit      Unit
	size      int
	overlap   int
	tokenizer Tokenizer // optional when unit==tokens
}

func newFixedSplitter(cfg FixedConfig) (Splitter, error) {
	size := cfg.Size
	if size <= 0 {
		size = 1
	}
	ov := cfg.Overlap
	if ov < 0 {
		ov = 0
	}
	if ov >= size {
		ov = size - 1
		if ov < 0 {
			ov = 0
		}
	}
	tok := cfg.Tokenizer
	if cfg.Unit == UnitTokens && tok == nil {
		tok = WhitespaceTokenizer{}
	}
	return &fixedSplitter{unit: cfg.Unit, size: size, overlap: ov, tokenizer: tok}, nil
}

func (s *fixedSplitter) Split(text string) []string {
	if text == "" {
		return nil
	}
	switch s.unit {
	case UnitTokens:
		return s.splitTokens(text)
	default: // UnitChars or unspecified
		return s.splitRunes(text)
	}
}

func (s *fixedSplitter) splitRunes(text string) []string {
	// Convert to rune offsets to avoid splitting inside a rune.
	// For performance, we avoid allocating full rune slice; we compute byte
	// indices per window step.
	var chunks []string
	size := s.size
	step := size - s.overlap
	if step <= 0 {
		step = 1
	}

	// Precompute rune boundaries (byte indices) up to needed positions.
	// We'll walk the string and record boundaries in a slice.
	idxs := make([]int, 0, utf8.RuneCountInString(text)+1)
	idxs = append(idxs, 0)
	for i := 0; i < len(text); {
		_, w := utf8.DecodeRuneInString(text[i:])
		i += w
		idxs = append(idxs, i)
	}
	// idxs[j] is byte index where rune j starts; idxs[len-1] == len(text).

	// Iterate by rune positions.
	for start := 0; start < len(idxs)-1; start += step {
		end := start + size
		if end >= len(idxs)-1 {
			end = len(idxs) - 1
		}
		if end <= start {
			break
		}
		chunk := text[idxs[start]:idxs[end]]
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
		if end == len(idxs)-1 {
			break
		}
	}
	return chunks
}

func (s *fixedSplitter) splitTokens(text string) []string {
	tok := s.tokenizer
	if tok == nil {
		tok = WhitespaceTokenizer{}
	}
	tokens := tok.Tokenize(text)
	if len(tokens) == 0 {
		return nil
	}
	size := s.size
	step := size - s.overlap
	if step <= 0 {
		step = 1
	}
	var chunks []string
	for start := 0; start < len(tokens); start += step {
		end := start + size
		if end > len(tokens) {
			end = len(tokens)
		}
		if end <= start {
			break
		}
		chunk := tok.Detokenize(tokens[start:end])
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
		if end == len(tokens) {
			break
		}
	}
	return chunks
}
