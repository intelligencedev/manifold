package registry

import (
	"context"
	"sort"
	"strings"

	"manifold/internal/auth"
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
func (s *InMemoryStore) CreatePrompt(ctx context.Context, prompt Prompt) (Prompt, error) {
	if u, ok := auth.CurrentUser(ctx); ok && u != nil {
		prompt.OwnerID = u.ID
	}
	if _, ok := s.prompts[prompt.ID]; ok {
		return Prompt{}, ErrPromptExists
	}
	s.prompts[prompt.ID] = prompt
	return prompt, nil
}

// GetPrompt fetches a prompt by ID.
func (s *InMemoryStore) GetPrompt(ctx context.Context, id string) (Prompt, bool, error) {
	prompt, ok := s.prompts[id]
	if !ok {
		return Prompt{}, false, nil
	}
	if u, okU := auth.CurrentUser(ctx); okU && u != nil {
		if prompt.OwnerID != u.ID {
			return Prompt{}, false, nil
		}
	}
	return prompt, true, nil
}

// ListPrompts returns prompts sorted by creation time descending.
func (s *InMemoryStore) ListPrompts(ctx context.Context, filter ListFilter) ([]Prompt, error) {
	var uid int64
	if u, ok := auth.CurrentUser(ctx); ok && u != nil {
		uid = u.ID
	}
	var out []Prompt
	for _, p := range s.prompts {
		if uid != 0 && p.OwnerID != uid {
			continue
		}
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
func (s *InMemoryStore) CreatePromptVersion(ctx context.Context, version PromptVersion) (PromptVersion, error) {
	if _, ok := s.prompts[version.PromptID]; !ok {
		return PromptVersion{}, ErrPromptNotFound
	}
	if u, ok := auth.CurrentUser(ctx); ok && u != nil {
		version.OwnerID = u.ID
	}
	s.versions[version.PromptID] = append(s.versions[version.PromptID], version)
	s.versionByID[version.ID] = version
	return version, nil
}

// ListPromptVersions returns all versions sorted newest first.
func (s *InMemoryStore) ListPromptVersions(ctx context.Context, promptID string) ([]PromptVersion, error) {
	versions := append([]PromptVersion(nil), s.versions[promptID]...)
	if u, ok := auth.CurrentUser(ctx); ok && u != nil {
		filtered := versions[:0]
		for _, v := range versions {
			if v.OwnerID == u.ID {
				filtered = append(filtered, v)
			}
		}
		versions = filtered
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i].CreatedAt.After(versions[j].CreatedAt) })
	return versions, nil
}

// GetPromptVersion fetches a prompt version by ID.
func (s *InMemoryStore) GetPromptVersion(ctx context.Context, id string) (PromptVersion, bool, error) {
	version, ok := s.versionByID[id]
	if !ok {
		return PromptVersion{}, false, nil
	}
	if u, okU := auth.CurrentUser(ctx); okU && u != nil {
		if version.OwnerID != u.ID {
			return PromptVersion{}, false, nil
		}
	}
	return version, true, nil
}

// DeletePrompt removes a prompt and all of its versions.
func (s *InMemoryStore) DeletePrompt(ctx context.Context, id string) error {
	uid := int64(0)
	if u, ok := auth.CurrentUser(ctx); ok && u != nil {
		uid = u.ID
	}
	if p, ok := s.prompts[id]; ok {
		if uid == 0 || p.OwnerID == uid {
			delete(s.prompts, id)
			delete(s.versions, id)
		}
	}
	// remove versionByID entries for this prompt
	for vid, v := range s.versionByID {
		if v.PromptID == id && (uid == 0 || v.OwnerID == uid) {
			delete(s.versionByID, vid)
		}
	}
	return nil
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
