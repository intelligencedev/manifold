package skills

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCache_GetOrLoad(t *testing.T) {
	t.Parallel()

	cache := NewCache()

	projectID := "test-project"
	gen := int64(1)
	skillsGen := int64(1)

	loadCount := 0
	loader := func() (*CachedSkills, error) {
		loadCount++
		return &CachedSkills{
			Generation:       gen,
			SkillsGeneration: skillsGen,
			Skills: []Metadata{
				{Name: "test-skill", Description: "A test skill"},
			},
			RenderedPrompt: "## Skills\n- test-skill: A test skill",
		}, nil
	}

	// First load
	result, err := cache.GetOrLoad(projectID, gen, skillsGen, loader)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, loadCount)
	assert.Len(t, result.Skills, 1)
	assert.Equal(t, "test-skill", result.Skills[0].Name)

	// Second load should use cache
	result2, err := cache.GetOrLoad(projectID, gen, skillsGen, loader)
	require.NoError(t, err)
	require.NotNil(t, result2)
	assert.Equal(t, 1, loadCount) // Should not have called loader again
	assert.Equal(t, result.RenderedPrompt, result2.RenderedPrompt)
}

func TestCache_Invalidate(t *testing.T) {
	t.Parallel()

	cache := NewCache()

	projectID := "test-project"

	// Load initial data
	loadCount := 0
	loader := func() (*CachedSkills, error) {
		loadCount++
		return &CachedSkills{
			Generation:       1,
			SkillsGeneration: 1,
			Skills:           []Metadata{{Name: "skill-" + string(rune('0'+loadCount))}},
			RenderedPrompt:   "prompt",
		}, nil
	}

	_, err := cache.GetOrLoad(projectID, 1, 1, loader)
	require.NoError(t, err)
	assert.Equal(t, 1, loadCount)

	// Invalidate
	cache.Invalidate(projectID)

	// Should reload
	_, err = cache.GetOrLoad(projectID, 1, 1, loader)
	require.NoError(t, err)
	assert.Equal(t, 2, loadCount)
}

func TestCache_DifferentGenerations(t *testing.T) {
	t.Parallel()

	cache := NewCache()

	projectID := "test-project"

	loadCount := 0
	loader := func() (*CachedSkills, error) {
		loadCount++
		return &CachedSkills{
			Generation:       int64(loadCount),
			SkillsGeneration: int64(loadCount),
			Skills:           []Metadata{{Name: "skill"}},
			RenderedPrompt:   "prompt",
		}, nil
	}

	// Load with generation 1
	_, err := cache.GetOrLoad(projectID, 1, 1, loader)
	require.NoError(t, err)
	assert.Equal(t, 1, loadCount)

	// Load same generation - should use cache
	_, err = cache.GetOrLoad(projectID, 1, 1, loader)
	require.NoError(t, err)
	assert.Equal(t, 1, loadCount)

	// Load with different generation - should reload
	_, err = cache.GetOrLoad(projectID, 2, 2, loader)
	require.NoError(t, err)
	assert.Equal(t, 2, loadCount)
}

func TestCachedSkills_CachedAt(t *testing.T) {
	t.Parallel()

	cache := NewCache()

	before := time.Now().UTC()

	loader := func() (*CachedSkills, error) {
		return &CachedSkills{
			Generation:       1,
			SkillsGeneration: 1,
			Skills:           []Metadata{{Name: "skill"}},
			RenderedPrompt:   "prompt",
		}, nil
	}

	result, err := cache.GetOrLoad("proj", 1, 1, loader)
	require.NoError(t, err)

	after := time.Now().UTC()

	// CachedAt should be set by the cache
	assert.False(t, result.CachedAt.IsZero())
	assert.True(t, result.CachedAt.After(before) || result.CachedAt.Equal(before))
	assert.True(t, result.CachedAt.Before(after) || result.CachedAt.Equal(after))
}
