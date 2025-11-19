package textsplitters

import (
	"regexp"
	"strings"
)

// MarkdownConfig configures markdown-aware splitting.
type MarkdownConfig struct {
	// Headers to treat as top-level boundaries (e.g., ["#","##"]). If empty, any heading starts a section.
	Headers []string
	// Within each section, group by BoundaryConfig (e.g., sentences) to target size.
	Within BoundaryConfig
}

var mdHeadingRe = regexp.MustCompile(`(?m)^(#{1,6})\s+(.+?)\s*$`)

type markdownSplitter struct{ cfg MarkdownConfig }

func newMarkdownSplitter(cfg MarkdownConfig) (Splitter, error) {
	return &markdownSplitter{cfg: cfg}, nil
}

func (m *markdownSplitter) Split(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if strings.TrimSpace(text) == "" {
		return nil
	}
	// Find headings; segment text by heading lines
	type seg struct {
		heading string
		body    string
	}
	var segs []seg
	idxs := mdHeadingRe.FindAllStringSubmatchIndex(text, -1)
	if len(idxs) == 0 {
		// No headings; fallback to boundary grouping
		return (&boundarySplitter{mode: "hybrid", cfg: m.cfg.Within}).Split(text)
	}
	// Append trailing end index
	for i := 0; i < len(idxs); i++ {
		start := idxs[i][0]
		end := 0
		if i+1 < len(idxs) {
			end = idxs[i+1][0]
		} else {
			end = len(text)
		}
		line := text[start:idxs[i][1]]
		body := strings.TrimSpace(text[idxs[i][1]:end])
		segs = append(segs, seg{heading: line, body: body})
	}

	// Filter by desired header levels if provided
	want := map[string]struct{}{}
	for _, h := range m.cfg.Headers {
		want[h] = struct{}{}
	}
	var chunks []string
	for _, s := range segs {
		if len(want) > 0 {
			m := mdHeadingRe.FindStringSubmatch(s.heading)
			if len(m) >= 2 {
				if _, ok := want[m[1]]; !ok {
					// Not desired level; merge into previous body or treat as plain
				}
			}
		}
		// First chunk is the heading itself for metadata/context
		header := strings.TrimSpace(s.heading)
		if header != "" {
			chunks = append(chunks, header)
		}
		// Then group the body using boundary grouping
		bs := (&boundarySplitter{mode: "hybrid", cfg: m.cfg.Within}).Split(s.body)
		chunks = append(chunks, bs...)
	}
	return chunks
}
