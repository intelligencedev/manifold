package databases

import (
	"context"
	"encoding/json"
	"manifold/internal/playground/registry"
	"sort"
	"strings"
)

func (s *PlaygroundStore) CreatePrompt(ctx context.Context, prompt registry.Prompt) (registry.Prompt, error) {
	data, err := json.Marshal(prompt)
	if err != nil {
		return registry.Prompt{}, err
	}
	uid := userIDFromContext(ctx)
	_, err = s.exec(ctx, `INSERT INTO playground_prompts (id, user_id, payload) VALUES ($1, $2, $3)`, prompt.ID, uid, data)
	if err != nil {
		if isPGConstraint(err) {
			return registry.Prompt{}, registry.ErrPromptExists
		}
		return registry.Prompt{}, err
	}
	return prompt, nil
}

// GetPrompt loads a prompt by ID.
func (s *PlaygroundStore) GetPrompt(ctx context.Context, id string) (registry.Prompt, bool, error) {
	uid := userIDFromContext(ctx)
	payload, ok, err := s.queryOnePayload(ctx, `SELECT payload FROM playground_prompts WHERE id=$1 AND user_id=$2`, id, uid)
	if err != nil || !ok {
		return registry.Prompt{}, ok, err
	}
	var prompt registry.Prompt
	if err := json.Unmarshal(payload, &prompt); err != nil {
		return registry.Prompt{}, false, err
	}
	return prompt, true, nil
}

// ListPrompts fetches all prompts and filters in memory.
func (s *PlaygroundStore) ListPrompts(ctx context.Context, filter registry.ListFilter) ([]registry.Prompt, error) {
	uid := userIDFromContext(ctx)
	payloads, err := s.queryPayloads(ctx, `SELECT payload FROM playground_prompts WHERE user_id=$1`, uid)
	if err != nil {
		return nil, err
	}

	var prompts []registry.Prompt
	for _, payload := range payloads {
		var prompt registry.Prompt
		if err := json.Unmarshal(payload, &prompt); err != nil {
			return nil, err
		}
		prompts = append(prompts, prompt)
	}

	filtered := prompts[:0]
	for _, prompt := range prompts {
		if filter.Query != "" && !strings.Contains(strings.ToLower(prompt.Name+prompt.Description), strings.ToLower(filter.Query)) {
			continue
		}
		if filter.Tag != "" {
			found := false
			for _, tag := range prompt.Tags {
				if strings.EqualFold(tag, filter.Tag) {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		filtered = append(filtered, prompt)
	}
	prompts = filtered
	sort.Slice(prompts, func(i, j int) bool { return prompts[i].CreatedAt.After(prompts[j].CreatedAt) })

	page := filter.Page
	perPage := filter.PerPage
	if perPage <= 0 {
		perPage = len(prompts)
	}
	if page <= 0 {
		page = 1
	}
	start := (page - 1) * perPage
	if start > len(prompts) {
		return []registry.Prompt{}, nil
	}
	end := min(start+perPage, len(prompts))
	return prompts[start:end], nil
}

// CreatePromptVersion stores the version payload.
func (s *PlaygroundStore) CreatePromptVersion(ctx context.Context, version registry.PromptVersion) (registry.PromptVersion, error) {
	data, err := json.Marshal(version)
	if err != nil {
		return registry.PromptVersion{}, err
	}
	uid := userIDFromContext(ctx)
	_, err = s.exec(ctx, `INSERT INTO playground_prompt_versions (id, prompt_id, created_at, user_id, payload) VALUES ($1,$2,$3,$4,$5)`, version.ID, version.PromptID, version.CreatedAt.UTC(), uid, data)
	if err != nil {
		return registry.PromptVersion{}, err
	}
	return version, nil
}

// ListPromptVersions returns all versions for a prompt newest first.
func (s *PlaygroundStore) ListPromptVersions(ctx context.Context, promptID string) ([]registry.PromptVersion, error) {
	uid := userIDFromContext(ctx)
	payloads, err := s.queryPayloads(ctx, `SELECT payload FROM playground_prompt_versions WHERE prompt_id=$1 AND user_id=$2 ORDER BY created_at DESC`, promptID, uid)
	if err != nil {
		return nil, err
	}

	var versions []registry.PromptVersion
	for _, payload := range payloads {
		var version registry.PromptVersion
		if err := json.Unmarshal(payload, &version); err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	return versions, nil
}

// GetPromptVersion fetches a prompt version by ID.
func (s *PlaygroundStore) GetPromptVersion(ctx context.Context, id string) (registry.PromptVersion, bool, error) {
	uid := userIDFromContext(ctx)
	payload, ok, err := s.queryOnePayload(ctx, `SELECT payload FROM playground_prompt_versions WHERE id=$1 AND user_id=$2`, id, uid)
	if err != nil || !ok {
		return registry.PromptVersion{}, ok, err
	}
	var version registry.PromptVersion
	if err := json.Unmarshal(payload, &version); err != nil {
		return registry.PromptVersion{}, false, err
	}
	return version, true, nil
}
