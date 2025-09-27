package registry

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestComputeContentHashStable(t *testing.T) {
	t.Parallel()

	template := "Hello {{name}}"
	varsA := map[string]VariableSchema{
		"name": {Type: "string", Description: "user name", Required: true},
	}
	varsB := map[string]VariableSchema{
		"name": {Description: "user name", Required: true, Type: "string"},
	}

	hashA, err := ComputeContentHash(template, varsA)
	require.NoError(t, err)
	hashB, err := ComputeContentHash(template, varsB)
	require.NoError(t, err)
	require.Equal(t, hashA, hashB)
}

func TestValidateTemplateMissingVariable(t *testing.T) {
	t.Parallel()

	template := "Hello {{name}} {{surname}}"
	vars := map[string]VariableSchema{
		"name": {Type: "string"},
	}

	hash, err := ComputeContentHash(template, vars)
	require.Empty(t, hash)
	require.Error(t, err)
	require.Contains(t, err.Error(), "surname")
}

func TestValidateTemplateRejectsEmpty(t *testing.T) {
	t.Parallel()

	hash, err := ComputeContentHash("   ", nil)
	require.Error(t, err)
	require.Empty(t, hash)
}
