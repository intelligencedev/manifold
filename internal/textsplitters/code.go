package textsplitters

import (
    "regexp"
    "strings"
)

// CodeConfig configures code-aware splitting.
type CodeConfig struct {
    // Language hint (e.g., "go", "python", "js"). If empty, use generic regexes.
    Language string
    // Fallback boundary grouping when blocks exceed target.
    Within BoundaryConfig
}

// Very lightweight regexes to detect code blocks/functions/classes; not exhaustive.
var (
    reGoFunc    = regexp.MustCompile(`(?m)^func\s+\(?.*?\)?\s*[A-Za-z_][A-Za-z0-9_]*\s*\(.*\)`) // start of func
    reGoType    = regexp.MustCompile(`(?m)^type\s+[A-Za-z_][A-Za-z0-9_]*\s+struct\s*{`)
    rePyDef     = regexp.MustCompile(`(?m)^def\s+[A-Za-z_][A-Za-z0-9_]*\s*\(.*\)\s*:`)
    rePyClass   = regexp.MustCompile(`(?m)^class\s+[A-Za-z_][A-Za-z0-9_]*\s*(\(.*\))?\s*:`)
    reJSFunc    = regexp.MustCompile(`(?m)^(function\s+[A-Za-z_][A-Za-z0-9_]*\s*\(|[A-Za-z_][A-Za-z0-9_]*\s*=\s*\(.*\)\s*=>)`)
)

type codeSplitter struct{ cfg CodeConfig }

func newCodeSplitter(cfg CodeConfig) (Splitter, error) { return &codeSplitter{cfg: cfg}, nil }

func (s *codeSplitter) Split(text string) []string {
    text = strings.ReplaceAll(text, "\r\n", "\n")
    if strings.TrimSpace(text) == "" {
        return nil
    }
    var lines = strings.Split(text, "\n")
    // choose patterns
    pats := []*regexp.Regexp{}
    switch strings.ToLower(s.cfg.Language) {
    case "go":
        pats = []*regexp.Regexp{reGoType, reGoFunc}
    case "python", "py":
        pats = []*regexp.Regexp{rePyClass, rePyDef}
    case "javascript", "js", "ts", "typescript":
        pats = []*regexp.Regexp{reJSFunc}
    default:
        pats = []*regexp.Regexp{reGoFunc, rePyDef, reJSFunc}
    }
    // Find block starts
    isStart := func(line string) bool {
        for _, r := range pats {
            if r.MatchString(line) {
                return true
            }
        }
        return false
    }
    var chunks []string
    var cur []string
    for i, ln := range lines {
        if isStart(ln) && len(cur) > 0 {
            chunk := strings.TrimSpace(strings.Join(cur, "\n"))
            if chunk != "" {
                chunks = append(chunks, chunk)
            }
            cur = cur[:0]
        }
        cur = append(cur, ln)
        if i == len(lines)-1 {
            chunk := strings.TrimSpace(strings.Join(cur, "\n"))
            if chunk != "" {
                chunks = append(chunks, chunk)
            }
        }
    }
    // If blocks are too large, apply Within grouping
    if s.cfg.Within.Size > 0 {
        var adjusted []string
        bcfg := s.cfg.Within
        bs := &boundarySplitter{mode: "hybrid", cfg: bcfg}
        for _, c := range chunks {
            if measure(c, bcfg.Unit, bcfg.Tokenizer) > bcfg.Size {
                adjusted = append(adjusted, bs.Split(c)...)
            } else {
                adjusted = append(adjusted, c)
            }
        }
        return adjusted
    }
    return chunks
}

