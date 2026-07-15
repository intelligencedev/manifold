// Package lexminify implements deterministic character/word-level minification
// for composed LLM message contents. It does not summarize, embed, or call models.
//
// Levels are progressive (each higher level implies all lower ones):
//
//	Level 0: disabled
//	Level 1 (L0): whitespace polish
//	Level 2 (L1): filler-phrase removal / substitution
//	Level 3 (L2): careful stopword drop (never negations/polarity)
//	Level 4 (L3): telegram/caveman + common abbreviations
//	Level 5 (L4): vowel skeleton on long words
//	Level 6 (L5): aggressive punctuation/spacing strip on non-protected prose
//
// Callers typically minify a provider-visible COPY of messages, not the
// permanent conversation history kept by the agent loop.
package lexminify

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"manifold/internal/llm"
)

// Public max levels (0 = off).
const (
	Off = 0
	// L0Whitespace collapses whitespace / blank lines.
	L0Whitespace = 1
	// L1Fillers removes or shortens low-value filler phrases.
	L1Fillers = 2
	// L2Stopwords drops a careful stopword set.
	L2Stopwords = 3
	// L3Telegram drops residual function words and applies abbreviations.
	L3Telegram = 4
	// L4Vowels strips vowels from long words.
	L4Vowels = 5
	// L5Aggressive densifies residual punctuation / spacing.
	L5Aggressive = 6
)

// Zone is a bitmask controlling which message regions are minified.
type Zone uint

const (
	// ZoneRuntimeContext targets [RUNTIME CONTEXT] blocks on the current user message.
	ZoneRuntimeContext Zone = 1 << iota
	// ZoneHistory targets non-current conversation messages (user/assistant history).
	ZoneHistory
	// ZoneCurrentRequest is retained for API compatibility. The live user body
	// under [CURRENT REQUEST] is never minified regardless of this bit.
	ZoneCurrentRequest
	// ZoneTool targets tool-role payloads. Included in DefaultZones; structured
	// JSON/YAML/URL spans remain protected by the span scanner even when on.
	ZoneTool
	// ZoneAssistant targets assistant-role history content (when not tool-call-only sidecars).
	ZoneAssistant
	// ZoneSystemPrompt targets system/developer prefix messages. Included in DefaultZones
	// for testing; clear the bit to preserve KV-cache-stable prefixes.
	ZoneSystemPrompt
)

// DefaultZones is applied when Options.Zones is 0.
// Heaviest compression lands on runtime context, conversation history, tool
// results, assistant history, and the system prompt. The live [CURRENT REQUEST]
// body (user-typed prompt) is never minified.
const DefaultZones = ZoneRuntimeContext | ZoneHistory | ZoneTool | ZoneAssistant | ZoneSystemPrompt

// Options controls MinifyMessages / MinifyString.
type Options struct {
	// Level is 0 (off) .. 6 (L5). Values above 6 are clamped to 6.
	Level int
	// Zones selects which message regions are minified. Zero means DefaultZones.
	Zones Zone
	// CurrentRequestMaxLevel is retained for API compatibility. The live user body
	// under [CURRENT REQUEST] is always left untouched (Off); only prefixed runtime
	// context and other default zones are minified.
	CurrentRequestMaxLevel int
}

// Result is a diagnostic view of a MinifyMessages call.
type Result struct {
	Messages      []llm.Message
	Level         int
	Zones         Zone
	Changed       bool
	MessagesTouched int
	RunesBefore   int
	RunesAfter    int
}

// MinifyString applies progressive levels to a single free-text string.
func MinifyString(s string, level int) string {
	return minifyUnprotected(s, clampLevel(level))
}

// MinifyMessages returns a shallow-copied message slice with selected contents minified.
// The input slice and message fields other than Content are not mutated.
func MinifyMessages(msgs []llm.Message, opts Options) Result {
	level := clampLevel(opts.Level)
	zones := opts.Zones
	if zones == 0 {
		zones = DefaultZones
	}
	curMax := opts.CurrentRequestMaxLevel
	if curMax < 0 {
		curMax = Off
	}
	if zones&ZoneCurrentRequest == 0 {
		curMax = Off
	} else if curMax == 0 {
		curMax = L0Whitespace
	}
	if curMax > level {
		curMax = level
	}

	out := make([]llm.Message, len(msgs))
	copy(out, msgs)
	if level <= Off || len(out) == 0 {
		return Result{Messages: out, Level: level, Zones: zones}
	}

	lastUser := lastUserIndex(out)
	prefixEnd := staticPrefixEnd(out)
	runesBefore := 0
	runesAfter := 0
	touched := 0
	changed := false

	for i := range out {
		runesBefore += utf8.RuneCountInString(out[i].Content)
		role := strings.ToLower(strings.TrimSpace(out[i].Role))
		orig := out[i].Content
		next := orig

		switch {
		case i < prefixEnd, role == "system", role == "developer":
			// System/developer prefix is minified only when ZoneSystemPrompt is set.
			// DefaultZones includes it for testing; drop the bit to keep a stable KV-cache prefix.
			if zones&ZoneSystemPrompt != 0 {
				next = minifyUnprotected(orig, level)
			}
		case role == "tool":
			if zones&ZoneTool != 0 {
				next = minifyUnprotected(orig, level)
			}
		case role == "assistant":
			if zones&(ZoneHistory|ZoneAssistant) != 0 && i != lastUser {
				// Assistants are historical once not the live turn.
				next = minifyUnprotected(orig, level)
			}
		case role == "user":
			if i == lastUser {
				next = minifyCurrentUserContent(orig, level, curMax, zones)
			} else if zones&ZoneHistory != 0 {
				next = minifyUnprotected(orig, level)
			}
		}

		if next != orig {
			out[i].Content = next
			changed = true
			touched++
		}
		runesAfter += utf8.RuneCountInString(out[i].Content)
	}

	return Result{
		Messages:        out,
		Level:           level,
		Zones:           zones,
		Changed:         changed,
		MessagesTouched: touched,
		RunesBefore:     runesBefore,
		RunesAfter:      runesAfter,
	}
}

func clampLevel(level int) int {
	if level < Off {
		return Off
	}
	if level > L5Aggressive {
		return L5Aggressive
	}
	return level
}

func staticPrefixEnd(msgs []llm.Message) int {
	end := 0
	for end < len(msgs) {
		switch strings.ToLower(strings.TrimSpace(msgs[end].Role)) {
		case "system", "developer":
			end++
		default:
			return end
		}
	}
	return end
}

func lastUserIndex(msgs []llm.Message) int {
	for i := len(msgs) - 1; i >= 0; i-- {
		if strings.EqualFold(strings.TrimSpace(msgs[i].Role), "user") {
			return i
		}
	}
	return -1
}

const (
	runtimeContextMarker    = "[RUNTIME CONTEXT]"
	currentRequestMarker    = "[CURRENT REQUEST]"
	conversationHistMarker  = "[CONVERSATION HISTORY]"
)

func minifyCurrentUserContent(content string, level, currentMax int, zones Zone) string {
	if level <= Off || content == "" {
		return content
	}

	// Hard invariant: the live user body under [CURRENT REQUEST] is never minified.
	// ZoneCurrentRequest / CurrentRequestMaxLevel remain as API plumbing only and
	// cannot compress text the user actually typed.
	userBodyLevel := Off
	_ = currentMax

	type part struct {
		text  string
		level int
	}
	var parts []part

	// Walk marker positions in order of appearance.
	remaining := content
	for len(remaining) > 0 {
		nextMarker, idx := nextSectionMarker(remaining)
		if idx < 0 {
			// Trailing unmarked text and wholly unmarked live user bodies stay
			// protected so typed prompt text is never minified.
			parts = append(parts, part{text: remaining, level: userBodyLevel})
			break
		}
		if idx > 0 {
			// Text before the next marker is only history-compressible when it is
			// pure conversation-history annotation; otherwise leave it alone so a
			// missing CURRENT REQUEST marker cannot pull the typed prompt into a
			// compressible runtime/history zone.
			preLevel := Off
			if strings.Contains(remaining[:idx], conversationHistMarker) && zones&ZoneHistory != 0 {
				preLevel = level
			}
			parts = append(parts, part{text: remaining[:idx], level: preLevel})
		}
		// Marker starts a section that runs until the next marker or EOF.
		rest := remaining[idx:]
		nextNextIdx := indexAnyMarker(rest[len(nextMarker):])
		var section string
		if nextNextIdx < 0 {
			section = rest
			remaining = ""
		} else {
			// offset relative to rest after current marker prefix
			abs := len(nextMarker) + nextNextIdx
			section = rest[:abs]
			remaining = rest[abs:]
		}
		secLevel := Off
		switch nextMarker {
		case runtimeContextMarker:
			if zones&ZoneRuntimeContext != 0 {
				secLevel = level
			}
		case conversationHistMarker:
			if zones&ZoneHistory != 0 {
				secLevel = level
			}
		case currentRequestMarker:
			// Never minify the user's actual typed prompt section.
			secLevel = userBodyLevel
		}
		parts = append(parts, part{text: section, level: secLevel})
	}

	var b strings.Builder
	b.Grow(len(content))
	for _, p := range parts {
		if p.level <= Off {
			b.WriteString(p.text)
			continue
		}
		b.WriteString(minifyUnprotected(p.text, p.level))
	}
	return b.String()
}

func nextSectionMarker(s string) (marker string, idx int) {
	idx = -1
	for _, m := range []string{runtimeContextMarker, currentRequestMarker, conversationHistMarker} {
		if i := strings.Index(s, m); i >= 0 && (idx < 0 || i < idx) {
			idx = i
			marker = m
		}
	}
	return marker, idx
}

func indexAnyMarker(s string) int {
	_, idx := nextSectionMarker(s)
	return idx
}

// minifyUnprotected runs the progressive pipeline while restoring protected spans.
func minifyUnprotected(s string, level int) string {
	if level <= Off || s == "" {
		return s
	}
	spans := findProtectedSpans(s)
	if len(spans) == 0 {
		return transformText(s, level)
	}

	var b strings.Builder
	b.Grow(len(s))
	cursor := 0
	for _, sp := range spans {
		if sp.start > cursor {
			b.WriteString(transformText(s[cursor:sp.start], level))
		}
		b.WriteString(s[sp.start:sp.end])
		cursor = sp.end
	}
	if cursor < len(s) {
		b.WriteString(transformText(s[cursor:], level))
	}
	return b.String()
}

func transformText(s string, level int) string {
	if s == "" || level <= Off {
		return s
	}
	out := s
	if level >= L0Whitespace {
		out = applyL0(out)
	}
	if level >= L1Fillers {
		out = applyL1(out)
	}
	if level >= L2Stopwords {
		out = applyL2(out)
	}
	if level >= L3Telegram {
		out = applyL3(out)
	}
	if level >= L4Vowels {
		out = applyL4(out)
	}
	if level >= L5Aggressive {
		out = applyL5(out)
	}
	if level >= L0Whitespace {
		// final whitespace pass to clean artifacts from later transforms
		out = applyL0(out)
	}
	return out
}

func applyL0(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	newlineRun := 0
	for _, r := range s {
		switch {
		case r == '\r':
			// drop CR; rely on LF
			continue
		case r == '\n':
			newlineRun++
			if newlineRun <= 2 {
				b.WriteRune('\n')
			}
			prevSpace = false
		case unicode.IsSpace(r):
			if newlineRun > 0 {
				// indent after newline: collapse to nothing (no leading spaces on lines)
				continue
			}
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
		default:
			newlineRun = 0
			prevSpace = false
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

func applyL1(s string) string {
	// Longest-first phrase rewrites (lowercase keys).
	lower := strings.ToLower(s)
	if !containsAnyPhrase(lower) {
		return s
	}
	// Work on lowercased copy, preserve non-letter separators layout approximately
	// by rewriting the lowercased version then restoring as-is (loss of case is fine
	// under compression).
	out := lower
	for _, p := range fillerPhrases {
		out = strings.ReplaceAll(out, p.from, p.to)
	}
	// Collapse leftover double spaces introduced by empty replacements.
	out = collapseASCIISpaces(out)
	return out
}

func applyL2(s string) string {
	return mapWords(s, func(word string) (string, bool) {
		lw := strings.ToLower(word)
		if protectedWords[lw] {
			return word, true
		}
		if stopwords[lw] {
			return "", false
		}
		return word, true
	})
}

func applyL3(s string) string {
	// abbreviations first on whole string lower.
	out := strings.ToLower(s)
	for _, p := range abbrevPhrases {
		out = strings.ReplaceAll(out, p.from, p.to)
	}
	out = mapWords(out, func(word string) (string, bool) {
		lw := strings.ToLower(word)
		// Prefer abbreviation of meaning-preserving short forms before drop guards.
		if repl := abbrevWords[lw]; repl != "" {
			return repl, true
		}
		if protectedWords[lw] {
			return word, true
		}
		if telegramDrop[lw] {
			return "", false
		}
		return word, true
	})
	return out
}

func applyL4(s string) string {
	return mapWords(s, func(word string) (string, bool) {
		if wordHasDigit(word) {
			return word, true
		}
		if utf8.RuneCountInString(word) < 5 {
			return word, true
		}
		lw := strings.ToLower(word)
		if protectedWords[lw] {
			return word, true
		}
		return stripVowelsKeepFirst(word), true
	})
}

func applyL5(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	prevPunct := false
	for _, r := range s {
		switch {
		case r == '\n':
			// keep single newlines, drop pure doubling residual
			if !prevPunct {
				// keep newline as soft break -> space denser: convert double breaks already L0
				b.WriteRune('\n')
			}
			prevSpace = false
			prevPunct = true
		case unicode.IsSpace(r):
			if !prevSpace && !prevPunct {
				b.WriteByte(' ')
				prevSpace = true
			}
		case strings.ContainsRune(",;:!.?", r):
			// keep one punctuation, no space before
			if prevSpace {
				// trim space before punct
				buf := b.String()
				if strings.HasSuffix(buf, " ") {
					b.Reset()
					b.WriteString(strings.TrimSuffix(buf, " "))
				}
			}
			if !prevPunct || r == '.' {
				b.WriteRune(r)
			}
			prevPunct = true
			prevSpace = false
		default:
			b.WriteRune(r)
			prevSpace = false
			prevPunct = false
		}
	}
	return strings.TrimSpace(b.String())
}

func mapWords(s string, fn func(word string) (string, bool)) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		r, size := utf8.DecodeRuneInString(s[i:])
		if isWordRune(r) {
			j := i + size
			for j < len(s) {
				r2, sz2 := utf8.DecodeRuneInString(s[j:])
				if !isWordRune(r2) {
					break
				}
				j += sz2
			}
			word := s[i:j]
			repl, keep := fn(word)
			if keep && repl != "" {
				// avoid leading space accumulation: if previous is space and we keep
				b.WriteString(repl)
			} else if !keep {
				// drop word; also drop one following space if present
				// (handled after by not writing word; collapse later)
			}
			i = j
			continue
		}
		b.WriteRune(r)
		i += size
	}
	return collapseASCIISpaces(b.String())
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || r == '\'' || r == '-'
}

func wordHasDigit(word string) bool {
	for _, r := range word {
		if unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func stripVowelsKeepFirst(word string) string {
	runes := []rune(word)
	if len(runes) < 5 {
		return word
	}
	out := make([]rune, 0, len(runes))
	out = append(out, runes[0])
	for _, r := range runes[1:] {
		lr := unicode.ToLower(r)
		if lr == 'a' || lr == 'e' || lr == 'i' || lr == 'o' || lr == 'u' || lr == 'y' {
			continue
		}
		out = append(out, r)
	}
	if len(out) < 2 {
		return word
	}
	return string(out)
}

func collapseASCIISpaces(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' {
			if prevSpace {
				continue
			}
			b.WriteByte(' ')
			prevSpace = true
			continue
		}
		prevSpace = false
		b.WriteRune(r)
	}
	// clean " ." / " ," residue
	out := b.String()
	out = strings.ReplaceAll(out, " ,", ",")
	out = strings.ReplaceAll(out, " .", ".")
	out = strings.ReplaceAll(out, " ;", ";")
	out = strings.ReplaceAll(out, " :", ":")
	out = strings.ReplaceAll(out, " !", "!")
	out = strings.ReplaceAll(out, " ?", "?")
	out = strings.ReplaceAll(out, "\n ", "\n")
	out = strings.ReplaceAll(out, " \n", "\n")
	return strings.TrimSpace(out)
}
