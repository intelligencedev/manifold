package textsplitters

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// BoundaryConfig controls sentence/paragraph/hybrid splitters.
type BoundaryConfig struct {
	Unit      Unit      // chars or tokens for target size
	Size      int       // target size; if <=0 default to 500
	Overlap   int       // optional overlap in same unit (best-effort)
	Tokenizer Tokenizer // used when Unit==tokens
}

var sentRe = regexp.MustCompile(`(?s)([^\.!?]+[\.!?]+|[^\.!?]+$)`) // naive sentence finder

func sentencesOf(text string) []string {
	parts := sentRe.FindAllString(strings.TrimSpace(text), -1)
	// trim spaces
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	out := parts[:0]
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func paragraphsOf(text string) []string {
	// split on blank line
	raw := regexp.MustCompile(`\n\s*\n+`).Split(strings.TrimSpace(text), -1)
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func measure(text string, unit Unit, tok Tokenizer) int {
	if unit == UnitTokens {
		if tok == nil {
			tok = WhitespaceTokenizer{}
		}
		return len(tok.Tokenize(text))
	}
	return utf8.RuneCountInString(text)
}

func clipOverlapTail(chunk string, want int, unit Unit, tok Tokenizer) string {
	if want <= 0 || chunk == "" {
		return ""
	}
	if unit == UnitTokens {
		if tok == nil {
			tok = WhitespaceTokenizer{}
		}
		toks := tok.Tokenize(chunk)
		if want >= len(toks) {
			return chunk
		}
		return tok.Detokenize(toks[len(toks)-want:])
	}
	// chars
	// walk runes from end
	n := utf8.RuneCountInString(chunk)
	if want >= n {
		return chunk
	}
	// get byte index where last want runes start
	// compute forward to reduce complexity
	var idxs []int
	idxs = make([]int, 0, n+1)
	idxs = append(idxs, 0)
	for i := 0; i < len(chunk); {
		_, w := utf8.DecodeRuneInString(chunk[i:])
		i += w
		idxs = append(idxs, i)
	}
	start := idxs[n-want]
	return chunk[start:]
}

func groupByTarget(units []string, cfg BoundaryConfig) []string {
	size := cfg.Size
	if size <= 0 {
		size = 500
	}
	if cfg.Overlap < 0 {
		cfg.Overlap = 0
	}
	var tok Tokenizer
	if cfg.Unit == UnitTokens {
		tok = cfg.Tokenizer
		if tok == nil {
			tok = WhitespaceTokenizer{}
		}
	}

	var chunks []string
	var cur strings.Builder
	var tail string
	curSize := 0
	for i, u := range units {
		if u == "" {
			continue
		}
		candidate := u
		if cur.Len() > 0 {
			candidate = cur.String() + "\n" + u
		}
		m := measure(candidate, cfg.Unit, tok)
		if m <= size || cur.Len() == 0 {
			if cur.Len() > 0 {
				cur.WriteString("\n")
			}
			cur.WriteString(u)
			curSize = m
			if i == len(units)-1 {
				s := cur.String()
				if s != "" {
					chunks = append(chunks, s)
				}
			}
			continue
		}
		// close current chunk
		s := cur.String()
		if s != "" {
			chunks = append(chunks, s)
		}
		// compute overlap tail from s
		tail = clipOverlapTail(s, cfg.Overlap, cfg.Unit, tok)
		cur.Reset()
		if tail != "" {
			cur.WriteString(tail)
			cur.WriteString("\n")
		}
		cur.WriteString(u)
		curSize = measure(cur.String(), cfg.Unit, tok)
		_ = curSize
		if i == len(units)-1 {
			s := cur.String()
			if s != "" {
				chunks = append(chunks, s)
			}
		}
	}
	if len(units) == 0 {
		return nil
	}
	return chunks
}

type boundarySplitter struct {
	mode string // "sent"|"para"|"hybrid"
	cfg  BoundaryConfig
}

func newSentenceSplitter(cfg BoundaryConfig) (Splitter, error) {
	return &boundarySplitter{mode: "sent", cfg: cfg}, nil
}
func newParagraphSplitter(cfg BoundaryConfig) (Splitter, error) {
	return &boundarySplitter{mode: "para", cfg: cfg}, nil
}
func newHybridSplitter(cfg BoundaryConfig) (Splitter, error) {
	return &boundarySplitter{mode: "hybrid", cfg: cfg}, nil
}

func (s *boundarySplitter) Split(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	var units []string
	switch s.mode {
	case "para":
		units = paragraphsOf(text)
	case "hybrid":
		// First by paragraphs, then flatten to sentences for very large paragraphs
		paras := paragraphsOf(text)
		for _, p := range paras {
			if measure(p, s.cfg.Unit, s.cfg.Tokenizer) > s.cfg.Size*2 && s.cfg.Size > 0 {
				units = append(units, sentencesOf(p)...)
			} else {
				units = append(units, p)
			}
		}
	default:
		units = sentencesOf(text)
	}
	return groupByTarget(units, s.cfg)
}

// Rolling windows of N sentences
type RollingConfig struct {
	Window int // number of sentences per chunk
	Step   int // advance by Step sentences (default 1)
}

type rollingSentenceSplitter struct{ cfg RollingConfig }

func newRollingSentenceSplitter(cfg RollingConfig) (Splitter, error) {
	return &rollingSentenceSplitter{cfg: cfg}, nil
}

func (s *rollingSentenceSplitter) Split(text string) []string {
	ss := sentencesOf(text)
	if len(ss) == 0 {
		return nil
	}
	n := s.cfg.Window
	if n <= 0 {
		n = 3
	}
	step := s.cfg.Step
	if step <= 0 {
		step = 1
	}
	var out []string
	for i := 0; i < len(ss); i += step {
		j := i + n
		if j > len(ss) {
			j = len(ss)
		}
		if i >= j {
			break
		}
		out = append(out, strings.Join(ss[i:j], " "))
		if j == len(ss) {
			break
		}
	}
	return out
}
