//go:build enterprise
// +build enterprise

package skills

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseSkillContent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		wantErr bool
		check   func(t *testing.T, md Metadata)
	}{
		{
			name: "valid skill",
			content: `---
name: test-skill
description: A test skill for testing
metadata:
  short-description: Short desc
---

# Test Skill

This is a test skill.
`,
			wantErr: false,
			check: func(t *testing.T, md Metadata) {
				assert.Equal(t, "test-skill", md.Name)
				assert.Equal(t, "A test skill for testing", md.Description)
				assert.Equal(t, "Short desc", md.ShortDescription)
				assert.Equal(t, ScopeRepo, md.Scope)
			},
		},
		{
			name: "missing name",
			content: `---
description: A test skill
---
`,
			wantErr: true,
		},
		{
			name: "missing description",
			content: `---
name: test-skill
---
`,
			wantErr: true,
		},
		{
			name:    "missing frontmatter",
			content: `# Just markdown`,
			wantErr: true,
		},
		{
			name:    "empty content",
			content: ``,
			wantErr: true,
		},
		{
			name: "minimal valid skill",
			content: `---
name: minimal
description: Minimal skill
---
`,
			wantErr: false,
			check: func(t *testing.T, md Metadata) {
				assert.Equal(t, "minimal", md.Name)
				assert.Equal(t, "Minimal skill", md.Description)
				assert.Empty(t, md.ShortDescription)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md, err := parseSkillContent([]byte(tt.content), "/test/path/SKILL.md", ScopeRepo)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, "/test/path/SKILL.md", md.Path)
			if tt.check != nil {
				tt.check(t, md)
			}
		})
	}
}

func TestNewS3SkillsLoader(t *testing.T) {
	t.Parallel()

	loader := NewS3SkillsLoader(nil)
	assert.NotNil(t, loader)
	assert.Nil(t, loader.projectSvc)
}

func TestS3SkillsLoader_LoadSkillsOnly_NilService(t *testing.T) {
	t.Parallel()

	loader := &S3SkillsLoader{projectSvc: nil}
	outcome := loader.LoadSkillsOnly(nil, 0, "")

	assert.Empty(t, outcome.Skills)
	assert.Empty(t, outcome.Errors)
}
