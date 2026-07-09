package memory

import (
	"math"
	"sort"
	"strings"
)

func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot, magA, magB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		magA += float64(a[i]) * float64(a[i])
		magB += float64(b[i]) * float64(b[i])
	}
	if magA == 0 || magB == 0 {
		return 0
	}
	return dot / (math.Sqrt(magA) * math.Sqrt(magB))
}

func normalizeVector(v []float32) []float32 {
	if len(v) == 0 {
		return nil
	}
	var mag float64
	for _, x := range v {
		mag += float64(x) * float64(x)
	}
	if mag == 0 {
		return append([]float32(nil), v...)
	}
	scale := 1 / math.Sqrt(mag)
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = float32(float64(x) * scale)
	}
	return out
}

func dotProduct(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
	}
	return dot
}

func applyMMR(candidates []ScoredMemoryEntry, k int, lambda float64) []ScoredMemoryEntry {
	if k <= 0 || len(candidates) == 0 {
		return nil
	}
	if k > len(candidates) {
		k = len(candidates)
	}
	if lambda <= 0 || lambda > 1 {
		lambda = 0.7
	}

	selected := make([]ScoredMemoryEntry, 0, k)
	used := make([]bool, len(candidates))
	for len(selected) < k {
		bestIdx := -1
		bestScore := math.Inf(-1)
		for i, candidate := range candidates {
			if used[i] || candidate.Entry == nil {
				continue
			}
			maxSimilarity := 0.0
			for _, chosen := range selected {
				if chosen.Entry == nil {
					continue
				}
				if sim := dotProduct(candidate.Entry.Embedding, chosen.Entry.Embedding); sim > maxSimilarity {
					maxSimilarity = sim
				}
			}
			mmrScore := lambda*candidate.Score - (1-lambda)*maxSimilarity
			if bestIdx == -1 || mmrScore > bestScore {
				bestIdx = i
				bestScore = mmrScore
			}
		}
		if bestIdx == -1 {
			break
		}
		used[bestIdx] = true
		selected = append(selected, candidates[bestIdx])
	}
	return selected
}

func inMemoryKeywordSearch(entries []*MemoryEntry, query string, k int) []ScoredMemoryEntry {
	terms := keywordTerms(query)
	if len(terms) == 0 || len(entries) == 0 || k <= 0 {
		return nil
	}
	scores := make([]ScoredMemoryEntry, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		textTerms := keywordTerms(strings.Join([]string{entry.Input, entry.Summary, entry.StrategyCard}, " "))
		if len(textTerms) == 0 {
			continue
		}
		matches := 0
		for term := range terms {
			if _, ok := textTerms[term]; ok {
				matches++
			}
		}
		if matches == 0 {
			continue
		}
		score := float64(matches) / float64(len(terms))
		if !keywordMatchAdmitted(matches, score) {
			continue
		}
		scores = append(scores, ScoredMemoryEntry{Entry: entry, Score: score})
	}
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score > scores[j].Score
	})
	if k > len(scores) {
		k = len(scores)
	}
	return scores[:k]
}

// KeywordQueryTerms returns the stopword-filtered token set used for keyword retrieval.
func KeywordQueryTerms(text string) map[string]struct{} {
	return keywordTerms(text)
}

// KeywordMatchAdmitted reports whether a keyword match is strong enough to enter fusion.
func KeywordMatchAdmitted(matchedDistinctTerms int, score float64) bool {
	return keywordMatchAdmitted(matchedDistinctTerms, score)
}

func keywordMatchAdmitted(matchedDistinctTerms int, score float64) bool {
	return matchedDistinctTerms >= 2 || score >= 0.5
}

var keywordStopwords = map[string]struct{}{
	"the": {}, "a": {}, "an": {}, "and": {}, "or": {}, "but": {}, "if": {}, "then": {}, "else": {},
	"for": {}, "of": {}, "to": {}, "in": {}, "on": {}, "at": {}, "by": {}, "with": {}, "from": {},
	"as": {}, "is": {}, "are": {}, "was": {}, "were": {}, "be": {}, "been": {}, "being": {},
	"it": {}, "its": {}, "this": {}, "that": {}, "these": {}, "those": {},
	"i": {}, "you": {}, "we": {}, "they": {}, "he": {}, "she": {},
	"do": {}, "does": {}, "did": {}, "done": {},
	"can": {}, "could": {}, "should": {}, "would": {}, "will": {}, "shall": {},
	"may": {}, "might": {}, "must": {}, "not": {}, "no": {}, "yes": {},
	"so": {}, "such": {}, "than": {}, "too": {}, "very": {}, "just": {},
	"about": {}, "into": {}, "over": {}, "under": {}, "again": {}, "also": {},
	"how": {}, "what": {}, "when": {}, "where": {}, "which": {}, "who": {}, "whom": {}, "why": {},
	"all": {}, "any": {}, "both": {}, "each": {}, "few": {}, "more": {}, "most": {},
	"other": {}, "some": {}, "own": {}, "same": {}, "only": {}, "now": {}, "please": {},
	// high-frequency agentic vocabulary
	"task": {}, "memory": {}, "file": {}, "files": {}, "code": {}, "error": {},
	"test": {}, "tests": {}, "run": {}, "use": {}, "using": {}, "used": {},
	"make": {}, "made": {}, "create": {}, "created": {}, "add": {}, "added": {},
	"update": {}, "updated": {}, "fix": {}, "fixed": {}, "check": {}, "ensure": {},
	"implement": {}, "work": {}, "working": {},
}

func keywordTerms(text string) map[string]struct{} {
	fields := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.' || r == '/')
	})
	terms := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		field = strings.Trim(field, "._-/")
		if len(field) < 3 {
			continue
		}
		if _, stop := keywordStopwords[field]; stop {
			continue
		}
		terms[field] = struct{}{}
	}
	return terms
}

func rrfFuse(rankings [][]ScoredMemoryEntry, k int, constant int) []ScoredMemoryEntry {
	if k <= 0 || len(rankings) == 0 {
		return nil
	}
	if constant <= 0 {
		constant = 60
	}
	type fusedCandidate struct {
		item  ScoredMemoryEntry
		score float64
	}
	fused := make(map[string]fusedCandidate)
	for _, ranking := range rankings {
		for rank, candidate := range ranking {
			if candidate.Entry == nil || candidate.Entry.ID == "" {
				continue
			}
			key := candidate.Entry.ID
			current := fused[key]
			if current.item.Entry == nil {
				current.item = candidate
			} else {
				if candidate.HasDense && !current.item.HasDense {
					current.item.Dense = candidate.Dense
					current.item.HasDense = true
				}
				if candidate.HasKeyword && !current.item.HasKeyword {
					current.item.Keyword = candidate.Keyword
					current.item.HasKeyword = true
				}
			}
			current.score += 1 / float64(constant+rank+1)
			fused[key] = current
		}
	}
	if len(fused) == 0 {
		return nil
	}
	out := make([]ScoredMemoryEntry, 0, len(fused))
	maxScore := 0.0
	for _, candidate := range fused {
		if candidate.score > maxScore {
			maxScore = candidate.score
		}
		item := candidate.item
		item.Score = candidate.score
		out = append(out, item)
	}
	if maxScore > 0 {
		for i := range out {
			out[i].Score /= maxScore
			out[i].RRF = out[i].Score
		}
	} else {
		for i := range out {
			out[i].RRF = out[i].Score
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Score > out[j].Score
	})
	if k > len(out) {
		k = len(out)
	}
	return out[:k]
}
