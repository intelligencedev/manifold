package textsplitters

import (
    "regexp"
    "strings"
)

// LayoutConfig applies simple heuristics for page/table-aware splitting.
type LayoutConfig struct {
    // PageDelimiter is a string or regex that denotes page breaks (e.g., "\f" or "\n\f\n"). Empty -> heuristics.
    PageDelimiter string
    // Within groups each page by boundary config
    Within BoundaryConfig
}

type layoutSplitter struct{ cfg LayoutConfig }

func newLayoutSplitter(cfg LayoutConfig) (Splitter, error) { return &layoutSplitter{cfg: cfg}, nil }

func (l *layoutSplitter) Split(text string) []string {
    text = strings.ReplaceAll(text, "\r\n", "\n")
    if strings.TrimSpace(text) == "" {
        return nil
    }
    // Pages: try form-feed first, else multiple blank lines as rough delimiter
    var pages []string
    if l.cfg.PageDelimiter != "" {
        // treat as regex
        if re, err := regexp.Compile(l.cfg.PageDelimiter); err == nil {
            pages = re.Split(text, -1)
        }
    }
    if len(pages) == 0 {
        if strings.Contains(text, "\f") {
            pages = strings.Split(text, "\f")
        } else {
            pages = regexp.MustCompile(`\n\s*\n{2,}`).Split(text, -1)
        }
    }
    bs := &boundarySplitter{mode: "hybrid", cfg: l.cfg.Within}
    var out []string
    for _, p := range pages {
        p = strings.TrimSpace(p)
        if p == "" {
            continue
        }
        out = append(out, bs.Split(p)...)
    }
    return out
}

