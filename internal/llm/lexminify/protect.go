package lexminify

import (
	"regexp"
	"unicode"
	"unicode/utf8"
)

type span struct {
	start int
	end   int // exclusive
}

// Precompiled scanners for protected spans. Order of finding does not matter; we
// merge/overlaps later. Matching is byte-oriented on UTF-8 text.
var (
	reFence      = regexp.MustCompile("(?s)```.*?```")
	reInlineCode = regexp.MustCompile("`[^`\n]+`")
	reURL        = regexp.MustCompile(`https?://[^\s<>"'` + "`" + `]+|www\.[^\s<>"'` + "`" + `]+`)
	reEmail      = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	reUUID       = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)
	reNumberUnit = regexp.MustCompile(`\b\d+(?:[.,]\d+)*(?:\s?(?:%|px|em|rem|ms|s|kb|mb|gb|tb|k|m|b|x|ms|Hz|GB|MB|KB|TB|GiB|MiB|KiB))?\b`)
	// Unix/relative or Windows-like paths, conservative.
	rePath = regexp.MustCompile(`(?:\./|\.\./|/)[A-Za-z0-9._/\-]+|[A-Za-z]:\\[A-Za-z0-9._\\\-]+`)
	// Common section markers — write protective copy so transforms don't mangle them.
	reMarker = regexp.MustCompile(`\[(?:RUNTIME CONTEXT|CURRENT REQUEST|CONVERSATION HISTORY)\]`)
)

func findProtectedSpans(s string) []span {
	if s == "" {
		return nil
	}
	var spans []span
	addAll := func(re *regexp.Regexp) {
		locs := re.FindAllStringIndex(s, -1)
		for _, loc := range locs {
			if loc[1] > loc[0] {
				spans = append(spans, span{start: loc[0], end: loc[1]})
			}
		}
	}
	addAll(reFence)
	addAll(reInlineCode)
	addAll(reURL)
	addAll(reEmail)
	addAll(reUUID)
	addAll(rePath)
	addAll(reMarker)
	// Numbers / units outside of already covered regions.
	for _, loc := range reNumberUnit.FindAllStringIndex(s, -1) {
		spans = append(spans, span{start: loc[0], end: loc[1]})
	}
	// Balanced JSON-ish / YAML block heuristic: streak of lines that look structured.
	spans = append(spans, findStructuredBlobs(s)...)
	return mergeSpans(spans)
}

func findStructuredBlobs(s string) []span {
	// Protect balanced {...} and [...] blocks at least 16 chars, and fenced-looking
	// YAML key: value multi-line runs (≥3 consecutive yaml-looking lines).
	var out []span
	out = append(out, findBalanced(s, '{', '}')...)
	out = append(out, findBalanced(s, '[', ']')...)

	// YAML-ish consecutive indented key lines.
	lines := splitLineOffsets(s)
	runStart := -1
	runCount := 0
	for _, ln := range lines {
		line := s[ln.start:ln.end]
		if looksYAMLLine(line) {
			if runStart < 0 {
				runStart = ln.start
			}
			runCount++
		} else {
			if runCount >= 3 && runStart >= 0 {
				out = append(out, span{start: runStart, end: ln.start})
			}
			runStart = -1
			runCount = 0
		}
	}
	if runCount >= 3 && runStart >= 0 {
		out = append(out, span{start: runStart, end: len(s)})
	}
	return out
}

type lineOff struct {
	start, end int
}

func splitLineOffsets(s string) []lineOff {
	var out []lineOff
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, lineOff{start: start, end: i})
			start = i + 1
		}
	}
	if start <= len(s) {
		out = append(out, lineOff{start: start, end: len(s)})
	}
	return out
}

func looksYAMLLine(line string) bool {
	trim := stringsTrimLeftSpace(line)
	if trim == "" || trim[0] == '#' {
		return false
	}
	// key: value (no leading #, has ':')
	colon := indexByte(trim, ':')
	if colon <= 0 {
		return false
	}
	key := trim[:colon]
	for _, r := range key {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func stringsTrimLeftSpace(s string) string {
	i := 0
	for i < len(s) {
		r, size := utf8.DecodeRuneInString(s[i:])
		if !unicode.IsSpace(r) {
			break
		}
		i += size
	}
	return s[i:]
}

func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func findBalanced(s string, open, close rune) []span {
	var out []span
	depth := 0
	start := -1
	for i, r := range s {
		switch r {
		case open:
			if depth == 0 {
				start = i
			}
			depth++
		case close:
			if depth == 0 {
				continue
			}
			depth--
			if depth == 0 && start >= 0 {
				end := i + utf8.RuneLen(r)
				if end-start >= 16 {
					// Prefer JSON-looking: successful if content has ":" or "," ratio.
					chunk := s[start:end]
					if looksStructured(chunk) {
						out = append(out, span{start: start, end: end})
					}
				}
				start = -1
			}
		}
	}
	return out
}

func looksStructured(s string) bool {
	// Require quotes or colons common in JSON/YAML blobs.
	colon := 0
	quotes := 0
	for _, r := range s {
		switch r {
		case ':':
			colon++
		case '"', '\'':
			quotes++
		}
	}
	return colon >= 2 || (colon >= 1 && quotes >= 2)
}

func mergeSpans(spans []span) []span {
	if len(spans) == 0 {
		return nil
	}
	// sort by start then end
	// insertion sort is fine for small N
	for i := 1; i < len(spans); i++ {
		j := i
		for j > 0 && (spans[j].start < spans[j-1].start || (spans[j].start == spans[j-1].start && spans[j].end < spans[j-1].end)) {
			spans[j], spans[j-1] = spans[j-1], spans[j]
			j--
		}
	}
	out := make([]span, 0, len(spans))
	cur := spans[0]
	for i := 1; i < len(spans); i++ {
		s := spans[i]
		if s.start <= cur.end {
			if s.end > cur.end {
				cur.end = s.end
			}
			continue
		}
		out = append(out, cur)
		cur = s
	}
	out = append(out, cur)
	return out
}
