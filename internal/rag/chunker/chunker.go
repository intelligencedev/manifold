package chunker

import (
    "regexp"
    "strings"

    "manifold/internal/rag/ingest"
)

// Chunk represents a produced chunk of text.
type Chunk struct {
    Index int
    Text  string
}

// Chunker interface provides text chunking strategies.
type Chunker interface {
    Chunk(text string, opt ingest.ChunkingOptions) ([]Chunk, error)
}

// SimpleChunker implements multiple lightweight strategies based on options.
type SimpleChunker struct{}

// Chunk splits text into chunks using strategy hints in options.
func (SimpleChunker) Chunk(text string, opt ingest.ChunkingOptions) ([]Chunk, error) {
    strategy := strings.ToLower(opt.Strategy)
    if strategy == "" {
        strategy = "fixed"
    }
    switch strategy {
    case "fixed", "tokens", "sentences":
        return fixedChunk(text, opt), nil
    case "markdown", "md":
        return markdownChunk(text, opt), nil
    case "code":
        return codeChunk(text, opt), nil
    default:
        return fixedChunk(text, opt), nil
    }
}

func targetLen(opt ingest.ChunkingOptions) int {
    n := opt.MaxTokens
    if n <= 0 {
        n = 512
    }
    // treat as approximate characters per chunk if tokens unknown
    return n * 4 // rough 4 chars per token heuristic
}

// fixedChunk makes contiguous chunks of target size with optional overlap.
func fixedChunk(text string, opt ingest.ChunkingOptions) []Chunk {
    tgt := targetLen(opt)
    if tgt < 32 {
        tgt = 32
    }
    ov := opt.Overlap
    if ov < 0 {
        ov = 0
    }
    ovChars := ov * 4
    var out []Chunk
    start := 0
    idx := 0
    for start < len(text) {
        end := start + tgt
        if end > len(text) {
            end = len(text)
        } else {
            // try to cut at whitespace boundary to reduce mid-word splits
            if i := strings.LastIndex(text[start:end], " "); i > tgt/2 {
                end = start + i
            }
        }
        chunk := strings.TrimSpace(text[start:end])
        if chunk != "" {
            out = append(out, Chunk{Index: idx, Text: chunk})
            idx++
        }
        if end == len(text) {
            break
        }
        // next start considers overlap
        next := end - ovChars
        if next <= start {
            next = end
        }
        start = next
    }
    return out
}

// markdownChunk prefers splitting on headings and paragraph breaks and preserves headings.
func markdownChunk(text string, opt ingest.ChunkingOptions) []Chunk {
    tgt := targetLen(opt)
    lines := strings.Split(text, "\n")
    var out []Chunk
    var buf strings.Builder
    idx := 0
    writeFlush := func() {
        if s := strings.TrimSpace(buf.String()); s != "" {
            out = append(out, Chunk{Index: idx, Text: s})
            idx++
            buf.Reset()
        }
    }
    for i, ln := range lines {
        isHeading := strings.HasPrefix(ln, "#")
        isParaBreak := strings.TrimSpace(ln) == "" && i+1 < len(lines) && strings.TrimSpace(lines[i+1]) != ""
        // Always consider heading as a hard boundary when buffer has content
        if isHeading && buf.Len() > 0 { writeFlush() }
        // Append line
        if buf.Len() > 0 {
            buf.WriteString("\n")
        }
        buf.WriteString(ln)
        // Consider flushing at paragraph boundary if exceeding target
        if (isHeading || isParaBreak) && buf.Len() >= tgt {
            writeFlush()
        }
    }
    writeFlush()
    return out
}

var codeSplitRe = regexp.MustCompile(`(?m)^\s*(func |class |def |#[#\s]|//)`) // heuristics for code boundaries

// codeChunk attempts to respect function/class boundaries and comments.
func codeChunk(text string, opt ingest.ChunkingOptions) []Chunk {
    tgt := targetLen(opt)
    lines := strings.Split(text, "\n")
    var out []Chunk
    var buf strings.Builder
    idx := 0
    for i, ln := range lines {
        if codeSplitRe.MatchString(ln) && (buf.Len() > 0 && (buf.Len()+len(ln)+1 > tgt || strings.Contains(buf.String(), "func "))) {
            out = append(out, Chunk{Index: idx, Text: strings.TrimRight(buf.String(), "\n")})
            idx++
            buf.Reset()
        }
        buf.WriteString(ln)
        if i < len(lines)-1 {
            buf.WriteString("\n")
        }
    }
    if s := strings.TrimSpace(buf.String()); s != "" {
        out = append(out, Chunk{Index: idx, Text: s})
    }
    return out
}

