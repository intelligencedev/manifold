package supertonic

import (
	"regexp"
	"strings"
)

// AvailableLangs mirrors helper.mjs AVAILABLE_LANGS.
var AvailableLangs = map[string]bool{
	"en": true, "ko": true, "ja": true, "ar": true, "bg": true, "cs": true,
	"da": true, "de": true, "el": true, "es": true, "et": true, "fi": true,
	"fr": true, "hi": true, "hr": true, "hu": true, "id": true, "it": true,
	"lt": true, "lv": true, "nl": true, "pl": true, "pt": true, "ro": true,
	"ru": true, "sk": true, "sl": true, "sv": true, "tr": true, "uk": true,
	"vi": true, "na": true,
}

var (
	emojiRe   = regexp.MustCompile(`[\x{1F600}-\x{1F64F}\x{1F300}-\x{1F5FF}\x{1F680}-\x{1F6FF}\x{1F700}-\x{1F77F}\x{1F780}-\x{1F7FF}\x{1F800}-\x{1F8FF}\x{1F900}-\x{1F9FF}\x{1FA00}-\x{1FA6F}\x{1FA70}-\x{1FAFF}\x{2600}-\x{26FF}\x{2700}-\x{27BF}\x{1F1E6}-\x{1F1FF}]+`)
	specialRe = regexp.MustCompile("[♥☆♡©\\\\]")
	wsRe      = regexp.MustCompile(`\s+`)
	endPunct  = regexp.MustCompile(`[.!?;:,'")\]}…。」』】〉》›»]$`)
	paragraph = regexp.MustCompile(`\n\s*\n+`)
	boundary  = regexp.MustCompile(`[.!?]+\s+`)
	initialRe = regexp.MustCompile(`^[A-Z]\.$`)
)

var replacements = []struct{ from, to string }{
	{"–", "-"}, {"‑", "-"}, {"—", "-"}, {"_", " "},
	{"“", "\""}, {"”", "\""}, {"‘", "'"}, {"’", "'"},
	{"´", "'"}, {"`", "'"}, {"[", " "}, {"]", " "}, {"|", " "}, {"/", " "},
	{"#", " "}, {"→", " "}, {"←", " "},
}

var exprReplacements = []struct{ from, to string }{
	{"@", " at "}, {"e.g.,", "for example, "}, {"i.e.,", "that is, "},
}

// preprocessText ports helper.mjs UnicodeProcessor.preprocessText. NFKD
// normalization is intentionally omitted (identity for ASCII/Latin; add
// golang.org/x/text/unicode/norm for full multilingual parity).
func preprocessText(text, lang string) (string, bool) {
	text = emojiRe.ReplaceAllString(text, "")
	for _, r := range replacements {
		text = strings.ReplaceAll(text, r.from, r.to)
	}
	text = specialRe.ReplaceAllString(text, "")
	for _, r := range exprReplacements {
		text = strings.ReplaceAll(text, r.from, r.to)
	}
	text = strings.ReplaceAll(text, " ,", ",")
	text = strings.ReplaceAll(text, " .", ".")
	text = strings.ReplaceAll(text, " !", "!")
	text = strings.ReplaceAll(text, " ?", "?")
	text = strings.ReplaceAll(text, " ;", ";")
	text = strings.ReplaceAll(text, " :", ":")
	text = strings.ReplaceAll(text, " '", "'")
	for strings.Contains(text, "\"\"") {
		text = strings.Replace(text, "\"\"", "\"", 1)
	}
	for strings.Contains(text, "''") {
		text = strings.Replace(text, "''", "'", 1)
	}
	for strings.Contains(text, "``") {
		text = strings.Replace(text, "``", "`", 1)
	}
	text = strings.TrimSpace(wsRe.ReplaceAllString(text, " "))
	if !endPunct.MatchString(text) {
		text += "."
	}
	if !AvailableLangs[lang] {
		return "", false
	}
	return "<" + lang + ">" + text + "</" + lang + ">", true
}

// tokenize maps each code point of the preprocessed text to its indexer id
// (-1 when out of range), mirroring helper.mjs codePointAt lookup. Returns ids
// and a same-length all-ones mask.
func tokenize(indexer []int64, text string) ([]int64, []float32) {
	runes := []rune(text)
	ids := make([]int64, len(runes))
	mask := make([]float32, len(runes))
	for i, r := range runes {
		if int(r) < len(indexer) {
			ids[i] = indexer[r]
		} else {
			ids[i] = -1
		}
		mask[i] = 1.0
	}
	return ids, mask
}

var abbreviations = map[string]bool{
	"Mr.": true, "Mrs.": true, "Ms.": true, "Dr.": true, "Prof.": true,
	"Sr.": true, "Jr.": true, "Ph.D.": true, "etc.": true, "e.g.": true,
	"i.e.": true, "vs.": true, "Inc.": true, "Ltd.": true, "Co.": true,
	"Corp.": true, "St.": true, "Ave.": true, "Blvd.": true,
}

func isAbbrev(tok string) bool {
	return abbreviations[tok] || initialRe.MatchString(tok)
}

func sentences(para string) []string {
	var out []string
	start := 0
	for _, m := range boundary.FindAllStringIndex(para, -1) {
		punct := regexp.MustCompile(`^[.!?]+`).FindString(para[m[0]:])
		end := m[0] + len(punct)
		tokStart := strings.LastIndex(para[start:m[0]], " ")
		if tokStart == -1 {
			tokStart = start
		} else {
			tokStart = start + tokStart + 1
		}
		tok := strings.TrimSpace(para[tokStart : m[0]+1])
		if isAbbrev(tok) {
			continue
		}
		if piece := strings.TrimSpace(para[start:end]); piece != "" {
			out = append(out, piece)
		}
		start = m[1]
	}
	if tail := strings.TrimSpace(para[start:]); tail != "" {
		out = append(out, tail)
	}
	return out
}

// chunkText ports helper.mjs chunkText: split into paragraphs and sentences,
// then group sentences into chunks no longer than maxLen.
func chunkText(text string, maxLen int) []string {
	var chunks []string
	for _, para := range paragraph.Split(strings.TrimSpace(text), -1) {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		current := ""
		for _, s := range sentences(para) {
			if len(current)+len(s)+1 <= maxLen {
				if current == "" {
					current = s
				} else {
					current += " " + s
				}
			} else {
				if current != "" {
					chunks = append(chunks, strings.TrimSpace(current))
				}
				current = s
			}
		}
		if current != "" {
			chunks = append(chunks, strings.TrimSpace(current))
		}
	}
	return chunks
}
