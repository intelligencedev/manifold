package agentd

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"unicode"

	"manifold/internal/agent/prompts"
	"manifold/internal/skills"
	"manifold/internal/tools"
)

type skillSearchTool struct {
	projectDir string
}

type skillSearchInput struct {
	Query string   `json:"query"`
	Names []string `json:"names"`
}

type skillSearchResult struct {
	Name             string  `json:"name"`
	Description      string  `json:"description"`
	ShortDescription string  `json:"short_description,omitempty"`
	Path             string  `json:"path"`
	Score            float64 `json:"score,omitempty"`
	Exact            bool    `json:"exact,omitempty"`
}

func newSkillSearchTool(projectDir string) tools.Tool {
	return &skillSearchTool{projectDir: strings.TrimSpace(projectDir)}
}

func (t *skillSearchTool) Name() string { return "skill_search" }

func (t *skillSearchTool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": "Search project skills by capability description or exact skill name. After selecting a skill, open its SKILL.md file to use it.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Natural-language description of the workflow or domain knowledge you need, such as 'plan a migration' or 'extract text from PDFs'.",
				},
				"names": map[string]any{
					"type":        "array",
					"description": "Optional exact skill names to look up directly if you already know them.",
					"items":       map[string]any{"type": "string"},
				},
			},
		},
	}
}

func (t *skillSearchTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var input skillSearchInput
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &input); err != nil {
			return nil, err
		}
	}
	cached, err := prompts.CachedSkillsForProject(t.projectDir)
	if err != nil || cached == nil || len(cached.Skills) == 0 {
		return []skillSearchResult{}, err
	}
	return searchSkills(cached.Skills, input.Query, input.Names), nil
}

func searchSkills(skillsList []skills.Metadata, query string, names []string) []skillSearchResult {
	resultsByName := make(map[string]skillSearchResult, len(skillsList))
	trimmedQuery := strings.TrimSpace(query)
	if trimmedQuery != "" {
		for _, md := range rankSkills(skillsList, trimmedQuery, 10) {
			resultsByName[md.Name] = md
		}
	}
	for _, name := range names {
		trimmedName := strings.TrimSpace(name)
		if trimmedName == "" {
			continue
		}
		for _, md := range skillsList {
			if strings.EqualFold(md.Name, trimmedName) {
				resultsByName[md.Name] = skillSearchResult{
					Name:             md.Name,
					Description:      md.Description,
					ShortDescription: md.ShortDescription,
					Path:             md.Path,
					Score:            100,
					Exact:            true,
				}
				break
			}
		}
	}
	out := make([]skillSearchResult, 0, len(resultsByName))
	for _, result := range resultsByName {
		out = append(out, result)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Name < out[j].Name
		}
		return out[i].Score > out[j].Score
	})
	return out
}

func rankSkills(skillsList []skills.Metadata, query string, limit int) []skillSearchResult {
	queryTokens := tokenizeSkillSearch(query)
	if len(queryTokens) == 0 {
		return nil
	}
	scored := make([]skillSearchResult, 0, len(skillsList))
	for _, md := range skillsList {
		score := scoreSkill(md, query, queryTokens)
		if score <= 0 {
			continue
		}
		scored = append(scored, skillSearchResult{
			Name:             md.Name,
			Description:      md.Description,
			ShortDescription: md.ShortDescription,
			Path:             md.Path,
			Score:            score,
		})
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].Score == scored[j].Score {
			return scored[i].Name < scored[j].Name
		}
		return scored[i].Score > scored[j].Score
	})
	if limit > 0 && len(scored) > limit {
		return scored[:limit]
	}
	return scored
}

func scoreSkill(md skills.Metadata, rawQuery string, queryTokens []string) float64 {
	name := strings.ToLower(md.Name)
	shortDesc := strings.ToLower(md.ShortDescription)
	description := strings.ToLower(md.Description)
	path := strings.ToLower(md.Path)
	query := strings.ToLower(strings.TrimSpace(rawQuery))
	score := 0.0
	if name == query {
		score += 100
	}
	for _, token := range queryTokens {
		score += weightedSkillTokenScore(name, token, 8, 16)
		score += weightedSkillTokenScore(shortDesc, token, 5, 9)
		score += weightedSkillTokenScore(description, token, 3, 6)
		score += weightedSkillTokenScore(path, token, 1, 3)
	}
	if strings.Contains(description, query) || strings.Contains(shortDesc, query) {
		score += 8
	}
	return score
}

func weightedSkillTokenScore(text, token string, containsWeight, exactWeight float64) float64 {
	if text == "" || token == "" || !strings.Contains(text, token) {
		return 0
	}
	if text == token {
		return exactWeight
	}
	return containsWeight
}

func tokenizeSkillSearch(text string) []string {
	fields := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	uniq := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if len(field) < 2 {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		uniq = append(uniq, field)
	}
	return uniq
}
