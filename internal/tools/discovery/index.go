package discovery

import (
	"math"
	"sort"
	"strings"
	"unicode"

	"manifold/internal/llm"
)

const (
	bm25K1 = 1.2
	bm25B  = 0.75
)

type ToolSearchResult struct {
	Name              string  `json:"name"`
	Description       string  `json:"description"`
	ParametersSummary string  `json:"parameters_summary,omitempty"`
	Score             float64 `json:"score"`
}

type toolDoc struct {
	result   ToolSearchResult
	length   int
	search   string
	nameFold string
}

type posting struct {
	name string
	tf   int
}

type ToolIndex struct {
	docs      map[string]toolDoc
	postings  map[string][]posting
	avgDocLen float64
	names     []string
}

func NewToolIndex(schemas []llm.ToolSchema) *ToolIndex {
	idx := &ToolIndex{
		docs:     make(map[string]toolDoc, len(schemas)),
		postings: make(map[string][]posting),
		names:    make([]string, 0, len(schemas)),
	}
	if len(schemas) == 0 {
		return idx
	}

	totalLen := 0
	for _, schema := range schemas {
		if strings.TrimSpace(schema.Name) == "" {
			continue
		}
		textParts := []string{schema.Name, schema.Description}
		paramSummary := summarizeParameters(schema.Parameters)
		if paramSummary != "" {
			textParts = append(textParts, paramSummary)
		}
		searchText := strings.TrimSpace(strings.Join(textParts, "\n"))
		terms := termFrequencies(tokenize(searchText))
		length := 0
		for _, tf := range terms {
			length += tf
		}
		if length == 0 {
			length = 1
		}
		doc := toolDoc{
			result: ToolSearchResult{
				Name:              schema.Name,
				Description:       strings.TrimSpace(schema.Description),
				ParametersSummary: paramSummary,
			},
			length:   length,
			search:   strings.ToLower(searchText),
			nameFold: strings.ToLower(schema.Name),
		}
		idx.docs[schema.Name] = doc
		idx.names = append(idx.names, schema.Name)
		totalLen += length
		for term, tf := range terms {
			idx.postings[term] = append(idx.postings[term], posting{name: schema.Name, tf: tf})
		}
	}
	if len(idx.docs) > 0 {
		idx.avgDocLen = float64(totalLen) / float64(len(idx.docs))
	}
	sort.Strings(idx.names)
	return idx
}

func (i *ToolIndex) Lookup(name string) (ToolSearchResult, bool) {
	if i == nil {
		return ToolSearchResult{}, false
	}
	doc, ok := i.docs[name]
	if !ok {
		return ToolSearchResult{}, false
	}
	return doc.result, true
}

func (i *ToolIndex) ListAll() []ToolSearchResult {
	if i == nil || len(i.names) == 0 {
		return nil
	}
	out := make([]ToolSearchResult, 0, len(i.names))
	for _, name := range i.names {
		if doc, ok := i.docs[name]; ok {
			out = append(out, doc.result)
		}
	}
	return out
}

func (i *ToolIndex) Search(query string, limit int) []ToolSearchResult {
	if i == nil || len(i.docs) == 0 {
		return nil
	}
	if limit <= 0 {
		limit = 10
	}
	query = strings.TrimSpace(query)
	if query == "" {
		all := i.ListAll()
		if len(all) > limit {
			all = all[:limit]
		}
		return all
	}
	queryTerms := tokenize(query)
	if len(queryTerms) == 0 {
		return nil
	}
	queryFold := strings.ToLower(query)
	scores := make(map[string]float64)
	for _, term := range queryTerms {
		postings := i.postings[term]
		if len(postings) == 0 {
			continue
		}
		idf := inverseDocumentFrequency(len(i.docs), len(postings))
		for _, post := range postings {
			doc := i.docs[post.name]
			tf := float64(post.tf)
			docLen := float64(doc.length)
			denom := tf + bm25K1*(1-bm25B+bm25B*(docLen/maxFloat(1.0, i.avgDocLen)))
			if denom == 0 {
				continue
			}
			scores[post.name] += idf * ((tf * (bm25K1 + 1)) / denom)
		}
	}
	for name, doc := range i.docs {
		switch {
		case doc.nameFold == queryFold:
			scores[name] += 8
		case strings.Contains(doc.nameFold, queryFold):
			scores[name] += 4
		case strings.Contains(doc.search, queryFold):
			scores[name] += 1.5
		}
	}
	if len(scores) == 0 {
		return nil
	}
	items := make([]ToolSearchResult, 0, len(scores))
	for name, score := range scores {
		result := i.docs[name].result
		result.Score = score
		items = append(items, result)
	}
	sort.SliceStable(items, func(a, b int) bool {
		if items[a].Score == items[b].Score {
			return items[a].Name < items[b].Name
		}
		return items[a].Score > items[b].Score
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}

func summarizeParameters(schema map[string]any) string {
	props, _ := schema["properties"].(map[string]any)
	if len(props) == 0 {
		return collectSchemaStrings(schema)
	}
	parts := make([]string, 0, len(props))
	for name, raw := range props {
		segment := strings.TrimSpace(name)
		if m, ok := raw.(map[string]any); ok {
			desc := firstNonEmptyString(m["description"], m["title"])
			if desc != "" {
				segment += ": " + desc
			}
		}
		parts = append(parts, strings.TrimSpace(segment))
	}
	sort.Strings(parts)
	return strings.Join(parts, "; ")
}

func collectSchemaStrings(value any) string {
	parts := make([]string, 0, 8)
	collectStrings(&parts, value)
	return strings.Join(parts, " ")
}

func collectStrings(parts *[]string, value any) {
	switch typed := value.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed != "" {
			*parts = append(*parts, trimmed)
		}
	case map[string]any:
		for _, key := range []string{"title", "description", "type"} {
			if v, ok := typed[key]; ok {
				collectStrings(parts, v)
			}
		}
		if props, ok := typed["properties"].(map[string]any); ok {
			for name, v := range props {
				collectStrings(parts, name)
				collectStrings(parts, v)
			}
		}
		if items, ok := typed["items"]; ok {
			collectStrings(parts, items)
		}
	case []any:
		for _, item := range typed {
			collectStrings(parts, item)
		}
	}
}

func firstNonEmptyString(values ...any) string {
	for _, value := range values {
		if s, ok := value.(string); ok {
			s = strings.TrimSpace(s)
			if s != "" {
				return s
			}
		}
	}
	return ""
}

func termFrequencies(tokens []string) map[string]int {
	out := make(map[string]int, len(tokens))
	for _, token := range tokens {
		if token == "" {
			continue
		}
		out[token]++
	}
	return out
}

func tokenize(text string) []string {
	var out []string
	var current []rune
	flush := func() {
		if len(current) == 0 {
			return
		}
		out = append(out, strings.ToLower(string(current)))
		current = current[:0]
	}
	for _, r := range text {
		switch {
		case r == '_' || r == '-' || unicode.IsSpace(r) || (!unicode.IsLetter(r) && !unicode.IsDigit(r)):
			flush()
		case len(current) > 0 && unicode.IsUpper(r) && unicode.IsLower(current[len(current)-1]):
			flush()
			current = append(current, unicode.ToLower(r))
		default:
			current = append(current, unicode.ToLower(r))
		}
	}
	flush()
	return out
}

func inverseDocumentFrequency(totalDocs, docsWithTerm int) float64 {
	if totalDocs == 0 || docsWithTerm == 0 {
		return 0
	}
	return math.Log(1 + (float64(totalDocs)-float64(docsWithTerm)+0.5)/(float64(docsWithTerm)+0.5))
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
