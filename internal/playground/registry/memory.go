package registry

import (
	"context"
	"sort"
	"strings"
)

// InMemoryStore offers a simple store for unit tests and prototypes.
type InMemoryStore struct {
	prompts     map[string]Prompt
	versions    map[string][]PromptVersion
	versionByID map[string]PromptVersion
}

// NewInMemoryStore constructs an empty in-memory store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		prompts:     make(map[string]Prompt),
		versions:    make(map[string][]PromptVersion),
		versionByID: make(map[string]PromptVersion),
	}
}

// CreatePrompt inserts the prompt if the ID was not used yet.
func (s *InMemoryStore) CreatePrompt(_ context.Context, prompt Prompt) (Prompt, error) {
	if _, ok := s.prompts[prompt.ID]; ok {
		return Prompt{}, ErrPromptExists
	}
	s.prompts[prompt.ID] = prompt
	return prompt, nil
}

// GetPrompt fetches a prompt by ID.
func (s *InMemoryStore) GetPrompt(_ context.Context, id string) (Prompt, bool, error) {
	prompt, ok := s.prompts[id]
	return prompt, ok, nil
}

// ListPrompts returns prompts sorted by creation time descending.
func (s *InMemoryStore) ListPrompts(_ context.Context, filter ListFilter) ([]Prompt, error) {
	var out []Prompt
	for _, p := range s.prompts {
		if filter.Query != "" && !matchesQuery(p, filter.Query) {
			continue
		}
		if filter.Tag != "" && !hasTag(p, filter.Tag) {
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return paginate(out, filter.Page, filter.PerPage), nil
}

// CreatePromptVersion appends a new version to the list.
func (s *InMemoryStore) CreatePromptVersion(_ context.Context, version PromptVersion) (PromptVersion, error) {
	if _, ok := s.prompts[version.PromptID]; !ok {
		return PromptVersion{}, ErrPromptNotFound
	}
	s.versions[version.PromptID] = append(s.versions[version.PromptID], version)
	s.versionByID[version.ID] = version
	return version, nil
}

// ListPromptVersions returns all versions sorted newest first.
func (s *InMemoryStore) ListPromptVersions(_ context.Context, promptID string) ([]PromptVersion, error) {
	versions := append([]PromptVersion(nil), s.versions[promptID]...)
	sort.Slice(versions, func(i, j int) bool { return versions[i].CreatedAt.After(versions[j].CreatedAt) })
	return versions, nil
}

// GetPromptVersion fetches a prompt version by ID.
func (s *InMemoryStore) GetPromptVersion(_ context.Context, id string) (PromptVersion, bool, error) {
	version, ok := s.versionByID[id]
	return version, ok, nil
}

func matchesQuery(prompt Prompt, q string) bool {
	return strings.Contains(strings.ToLower(prompt.Name), strings.ToLower(q)) || strings.Contains(strings.ToLower(prompt.Description), strings.ToLower(q))
}

func hasTag(prompt Prompt, tag string) bool {
	for _, t := range prompt.Tags {
		if strings.EqualFold(t, tag) {
			return true
		}
	}
	return false
}

func paginate[T any](items []T, page, perPage int) []T {
	if perPage <= 0 {
		return items
	}
	if page <= 0 {
		page = 1
	}
	start := (page - 1) * perPage
	if start >= len(items) {
		return []T{}
	}
	end := start + perPage
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}
