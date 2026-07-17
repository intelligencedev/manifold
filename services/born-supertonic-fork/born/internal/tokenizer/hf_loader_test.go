package tokenizer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectHFTokenizerType(t *testing.T) {
	// Create a temporary tokenizer.json for testing.
	tmpDir := t.TempDir()
	tokenizerPath := filepath.Join(tmpDir, "tokenizer.json")

	config := map[string]interface{}{
		"model": map[string]interface{}{
			"type": "BPE",
			"vocab": map[string]int{
				"a": 0,
				"b": 1,
			},
		},
		"added_tokens": []map[string]interface{}{
			{"id": 2, "content": "<s>", "special": true},
			{"id": 3, "content": "</s>", "special": true},
		},
	}

	data, err := json.Marshal(config)
	require.NoError(t, err)
	err = os.WriteFile(tokenizerPath, data, 0o600)
	require.NoError(t, err)

	metadata, err := DetectHFTokenizerType(tokenizerPath)
	require.NoError(t, err)
	assert.Equal(t, HFTypeBPE, metadata.Type)
	assert.Equal(t, "BPE", metadata.TokenizerType)
	assert.Equal(t, 2, metadata.VocabSize)
	assert.True(t, metadata.HasBOS)
	assert.True(t, metadata.HasEOS)
}

func TestDetectHFTokenizerType_WordPiece(t *testing.T) {
	tmpDir := t.TempDir()
	tokenizerPath := filepath.Join(tmpDir, "tokenizer.json")

	config := map[string]interface{}{
		"model": map[string]interface{}{
			"type": "WordPiece",
			"vocab": map[string]int{
				"[CLS]": 0,
				"[SEP]": 1,
			},
		},
		"added_tokens": []map[string]interface{}{
			{"id": 0, "content": "[CLS]", "special": true},
			{"id": 1, "content": "[SEP]", "special": true},
		},
	}

	data, err := json.Marshal(config)
	require.NoError(t, err)
	err = os.WriteFile(tokenizerPath, data, 0o600)
	require.NoError(t, err)

	metadata, err := DetectHFTokenizerType(tokenizerPath)
	require.NoError(t, err)
	assert.Equal(t, HFTypeWordPiece, metadata.Type)
	assert.True(t, metadata.HasBOS)
	assert.True(t, metadata.HasEOS)
}

func TestDetectHFTokenizerType_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	tokenizerPath := filepath.Join(tmpDir, "tokenizer.json")

	err := os.WriteFile(tokenizerPath, []byte("invalid json"), 0o600)
	require.NoError(t, err)

	_, err = DetectHFTokenizerType(tokenizerPath)
	assert.Error(t, err)
}

func TestDetectHFTokenizerType_FileNotFound(t *testing.T) {
	_, err := DetectHFTokenizerType("/nonexistent/path/tokenizer.json")
	assert.Error(t, err)
}

func TestTryLoadTikToken(t *testing.T) {
	tests := []struct {
		name      string
		modelName string
		wantErr   bool
	}{
		{
			name:      "gpt-4",
			modelName: "gpt-4",
			wantErr:   false,
		},
		{
			name:      "gpt-3.5-turbo",
			modelName: "gpt-3.5-turbo",
			wantErr:   false,
		},
		{
			name:      "unknown model",
			modelName: "unknown-model-xyz",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tok, err := TryLoadTikToken(tt.modelName)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, tok)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, tok)
		})
	}
}

func TestAutoLoadTokenizer_TikToken(t *testing.T) {
	// Test auto-loading tiktoken by encoding name.
	tok, err := AutoLoadTokenizer("cl100k_base")
	require.NoError(t, err)
	require.NotNil(t, tok)

	// Verify it works.
	tokens, err := tok.Encode("test")
	require.NoError(t, err)
	assert.NotEmpty(t, tokens)
}

func TestAutoLoadTokenizer_ModelName(t *testing.T) {
	// Test auto-loading tiktoken by model name.
	tok, err := AutoLoadTokenizer("gpt-4")
	require.NoError(t, err)
	require.NotNil(t, tok)

	tokens, err := tok.Encode("test")
	require.NoError(t, err)
	assert.NotEmpty(t, tokens)
}

func TestAutoLoadTokenizer_HuggingFace(t *testing.T) {
	// Create a temporary HuggingFace model directory.
	tmpDir := t.TempDir()
	tokenizerPath := filepath.Join(tmpDir, "tokenizer.json")

	config := map[string]interface{}{
		"model": map[string]interface{}{
			"type": "BPE",
			"vocab": map[string]int{
				"a": 0,
				"b": 1,
				"c": 2,
			},
			"merges": []string{
				"a b",
			},
		},
		"added_tokens": []map[string]interface{}{},
	}

	data, err := json.Marshal(config)
	require.NoError(t, err)
	err = os.WriteFile(tokenizerPath, data, 0o600)
	require.NoError(t, err)

	// Auto-load should find the tokenizer.json.
	tok, err := AutoLoadTokenizer(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, tok)
	assert.Equal(t, 3, tok.VocabSize())
}

func TestAutoLoadTokenizer_Invalid(t *testing.T) {
	_, err := AutoLoadTokenizer("/nonexistent/path/xyz")
	assert.Error(t, err)
}

func TestLoadFromHuggingFace_BPE(t *testing.T) {
	tmpDir := t.TempDir()
	tokenizerPath := filepath.Join(tmpDir, "tokenizer.json")

	config := map[string]interface{}{
		"model": map[string]interface{}{
			"type": "BPE",
			"vocab": map[string]int{
				"hello": 0,
				"world": 1,
			},
			"merges": []string{},
		},
		"added_tokens": []map[string]interface{}{
			{"id": 100, "content": "<bos>", "special": true},
			{"id": 101, "content": "<eos>", "special": true},
		},
	}

	data, err := json.Marshal(config)
	require.NoError(t, err)
	err = os.WriteFile(tokenizerPath, data, 0o600)
	require.NoError(t, err)

	tok, err := LoadFromHuggingFace(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, tok)
	assert.Equal(t, 2, tok.VocabSize())
	assert.Equal(t, int32(100), tok.BosToken())
	assert.Equal(t, int32(101), tok.EosToken())
}

func TestLoadFromHuggingFace_WordPiece(t *testing.T) {
	tmpDir := t.TempDir()
	tokenizerPath := filepath.Join(tmpDir, "tokenizer.json")

	config := map[string]interface{}{
		"model": map[string]interface{}{
			"type": "WordPiece",
			"vocab": map[string]int{
				"a": 0,
			},
		},
	}

	data, err := json.Marshal(config)
	require.NoError(t, err)
	err = os.WriteFile(tokenizerPath, data, 0o600)
	require.NoError(t, err)

	_, err = LoadFromHuggingFace(tmpDir)
	// WordPiece not implemented yet.
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not yet implemented")
}

func TestHFTokenizerType_Constants(t *testing.T) {
	assert.Equal(t, HFTokenizerType("BPE"), HFTypeBPE)
	assert.Equal(t, HFTokenizerType("WordPiece"), HFTypeWordPiece)
	assert.Equal(t, HFTokenizerType("Unigram"), HFTypeUnigram)
	assert.Equal(t, HFTokenizerType("Unknown"), HFTypeUnknown)
}
