package agentd

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

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
	All   bool     `json:"all"`
}

type skillSearchResult struct {
	SkillID          string  `json:"skill_id"`
	Name             string  `json:"name"`
	Description      string  `json:"description"`
	ShortDescription string  `json:"short_description,omitempty"`
	Source           string  `json:"source"`
	Path             string  `json:"path"`
	Score            float64 `json:"score,omitempty"`
	Exact            bool    `json:"exact,omitempty"`
}

type skillReadTool struct {
	projectDir string
}

type skillReadInput struct {
	SkillID  string   `json:"skill_id"`
	Path     string   `json:"path"`
	Paths    []string `json:"paths"`
	MaxBytes int      `json:"max_bytes"`
}

type skillReadEntry struct {
	Path      string `json:"path"`
	OK        bool   `json:"ok"`
	Content   string `json:"content,omitempty"`
	Encoding  string `json:"encoding,omitempty"`
	Bytes     int64  `json:"bytes,omitempty"`
	BytesRead int    `json:"bytes_read,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
	Error     string `json:"error,omitempty"`
}

type skillReadResult struct {
	OK        bool             `json:"ok"`
	Error     string           `json:"error,omitempty"`
	SkillID   string           `json:"skill_id,omitempty"`
	Name      string           `json:"name,omitempty"`
	Source    string           `json:"source,omitempty"`
	Path      string           `json:"path,omitempty"`
	Content   string           `json:"content,omitempty"`
	Encoding  string           `json:"encoding,omitempty"`
	Bytes     int64            `json:"bytes,omitempty"`
	BytesRead int              `json:"bytes_read,omitempty"`
	Truncated bool             `json:"truncated,omitempty"`
	Files     []skillReadEntry `json:"files,omitempty"`
}

func newSkillSearchTool(projectDir string) tools.Tool {
	return &skillSearchTool{projectDir: strings.TrimSpace(projectDir)}
}

func newSkillReadTool(projectDir string) tools.Tool {
	return &skillReadTool{projectDir: strings.TrimSpace(projectDir)}
}

func (t *skillSearchTool) Name() string { return "skill_search" }

func (t *skillSearchTool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": "Search project and universal skills by capability description or exact skill name. Set all=true when the user asks to list every available skill. After selecting a skill, use skill_read with its skill_id to open SKILL.md or related files.",
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
				"all": map[string]any{
					"type":        "boolean",
					"description": "Return every available skill instead of only the ranked subset. Use this when the user asks to list or show all skills.",
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
	return searchSkills(cached.Skills, input.Query, input.Names, input.All), nil
}

func (t *skillReadTool) Name() string { return "skill_read" }

func (t *skillReadTool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": "Read a selected skill's SKILL.md or files beneath that skill directory by skill_id. This is the only read path for universal skills outside the project sandbox.",
		"parameters": map[string]any{
			"type":     "object",
			"required": []string{"skill_id"},
			"properties": map[string]any{
				"skill_id":  map[string]any{"type": "string", "description": "The skill_id returned by skill_search or listed in the Skills prompt."},
				"path":      map[string]any{"type": "string", "description": "Optional single path under the skill directory. Defaults to SKILL.md."},
				"paths":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional multiple paths under the skill directory."},
				"max_bytes": map[string]any{"type": "integer", "minimum": 1, "maximum": maxSkillReadBytes, "description": "Maximum bytes to read per file."},
			},
		},
	}
}

const (
	defaultSkillReadBytes = 64 * 1024
	maxSkillReadBytes     = 4 * 1024 * 1024
)

func (t *skillReadTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var input skillReadInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	input.SkillID = strings.TrimSpace(input.SkillID)
	if input.SkillID == "" {
		return skillReadResult{OK: false, Error: "missing skill_id"}, nil
	}
	cached, err := prompts.CachedSkillsForProject(t.projectDir)
	if err != nil {
		return skillReadResult{OK: false, Error: err.Error()}, nil
	}
	md, ok := findSkillByID(cached, input.SkillID)
	if !ok {
		return skillReadResult{OK: false, SkillID: input.SkillID, Error: "unknown skill_id"}, nil
	}
	paths := make([]string, 0, 1+len(input.Paths))
	if input.Path != "" {
		paths = append(paths, input.Path)
	}
	paths = append(paths, input.Paths...)
	if len(paths) == 0 {
		paths = append(paths, skillsFileName())
	}
	maxBytes := input.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultSkillReadBytes
	}
	if maxBytes > maxSkillReadBytes {
		maxBytes = maxSkillReadBytes
	}

	files := make([]skillReadEntry, 0, len(paths))
	anyOK := false
	for _, p := range paths {
		entry := readSkillFile(md, p, maxBytes)
		if entry.OK {
			anyOK = true
		}
		files = append(files, entry)
	}
	result := skillReadResult{
		OK:      anyOK,
		SkillID: md.SkillID,
		Name:    md.Name,
		Source:  string(md.Source),
		Files:   files,
	}
	if len(files) == 1 {
		result.Path = files[0].Path
		result.Content = files[0].Content
		result.Encoding = files[0].Encoding
		result.Bytes = files[0].Bytes
		result.BytesRead = files[0].BytesRead
		result.Truncated = files[0].Truncated
		if !files[0].OK {
			result.Error = files[0].Error
		}
	}
	return result, nil
}

func findSkillByID(cached *skills.CachedSkills, skillID string) (skills.Metadata, bool) {
	if cached == nil {
		return skills.Metadata{}, false
	}
	for _, md := range cached.Skills {
		if md.SkillID == skillID {
			return md, true
		}
	}
	return skills.Metadata{}, false
}

func readSkillFile(md skills.Metadata, inputPath string, maxBytes int) skillReadEntry {
	cleaned, full, err := resolveSkillReadPath(md.SkillDir, inputPath)
	entry := skillReadEntry{Path: cleaned}
	if err != nil {
		entry.Error = fmt.Sprintf("invalid path: %v", err)
		return entry
	}
	info, err := os.Lstat(full)
	if err != nil {
		entry.Error = err.Error()
		return entry
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		entry.Error = "refusing to read symlink"
		return entry
	}
	if info.IsDir() {
		entry.Error = "path is a directory"
		return entry
	}
	data, truncated, err := readSkillLimited(full, maxBytes)
	if err != nil {
		entry.Error = err.Error()
		return entry
	}
	content, encoding := encodeSkillContent(data)
	entry.OK = true
	entry.Content = content
	entry.Encoding = encoding
	entry.Bytes = info.Size()
	entry.BytesRead = len(data)
	entry.Truncated = truncated
	return entry
}

func resolveSkillReadPath(skillDir, inputPath string) (string, string, error) {
	if strings.TrimSpace(skillDir) == "" {
		return "", "", fmt.Errorf("skill directory unavailable")
	}
	p := strings.TrimSpace(inputPath)
	if p == "" {
		p = skillsFileName()
	}
	if filepath.IsAbs(p) {
		return p, "", fmt.Errorf("absolute paths are not allowed")
	}
	cleaned := filepath.Clean(p)
	if cleaned == "." || cleaned == "" {
		return cleaned, "", fmt.Errorf("empty path")
	}
	if !filepath.IsLocal(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(os.PathSeparator)) {
		return filepath.ToSlash(cleaned), "", fmt.Errorf("path traversal is not allowed")
	}
	base, err := filepath.Abs(skillDir)
	if err != nil {
		return filepath.ToSlash(cleaned), "", err
	}
	full, err := filepath.Abs(filepath.Join(base, cleaned))
	if err != nil {
		return filepath.ToSlash(cleaned), "", err
	}
	if !isWithinSkillRoot(base, full) {
		return filepath.ToSlash(cleaned), "", fmt.Errorf("path escapes skill directory")
	}
	resolvedBase, err := filepath.EvalSymlinks(base)
	if err != nil {
		return filepath.ToSlash(cleaned), "", err
	}
	resolvedParent, err := filepath.EvalSymlinks(filepath.Dir(full))
	if err != nil {
		return filepath.ToSlash(cleaned), "", err
	}
	if !isWithinSkillRoot(resolvedBase, resolvedParent) {
		return filepath.ToSlash(cleaned), "", fmt.Errorf("path escapes skill directory")
	}
	return filepath.ToSlash(cleaned), full, nil
}

func isWithinSkillRoot(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

func skillsFileName() string {
	return "SKILL.md"
}

func readSkillLimited(path string, maxBytes int) ([]byte, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	lr := &io.LimitedReader{R: f, N: int64(maxBytes) + 1}
	data, err := io.ReadAll(lr)
	if err != nil {
		return nil, false, err
	}
	truncated := len(data) > maxBytes
	if truncated {
		data = data[:maxBytes]
	}
	return data, truncated, nil
}

func encodeSkillContent(data []byte) (string, string) {
	if utf8.Valid(data) && bytes.IndexByte(data, 0) == -1 {
		return string(data), "utf-8"
	}
	return base64.StdEncoding.EncodeToString(data), "base64"
}

func searchSkills(skillsList []skills.Metadata, query string, names []string, all bool) []skillSearchResult {
	if all {
		out := make([]skillSearchResult, 0, len(skillsList))
		for _, md := range skillsList {
			out = append(out, skillSearchResultFromMetadata(md))
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Source == out[j].Source {
				return out[i].Name < out[j].Name
			}
			return sourceRank(out[i].Source) < sourceRank(out[j].Source)
		})
		return out
	}
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
				result := skillSearchResultFromMetadata(md)
				result.Score = 100
				result.Exact = true
				resultsByName[md.Name] = result
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
		result := skillSearchResultFromMetadata(md)
		result.Score = score
		scored = append(scored, result)
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

func skillSearchResultFromMetadata(md skills.Metadata) skillSearchResult {
	return skillSearchResult{
		SkillID:          md.SkillID,
		Name:             md.Name,
		Description:      md.Description,
		ShortDescription: md.ShortDescription,
		Source:           string(md.Source),
		Path:             md.Path,
	}
}

func sourceRank(source string) int {
	switch source {
	case string(skills.SourceProject):
		return 0
	case string(skills.SourceManifold):
		return 1
	case string(skills.SourceAgents):
		return 2
	default:
		return 3
	}
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
