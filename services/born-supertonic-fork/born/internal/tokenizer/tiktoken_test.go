package tokenizer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTikToken_NewTikToken(t *testing.T) {
	tests := []struct {
		name              string
		encoding          string
		wantErr           bool
		expectedVocabSize int
	}{
		{
			name:              "cl100k_base",
			encoding:          "cl100k_base",
			wantErr:           false,
			expectedVocabSize: 100256,
		},
		{
			name:              "p50k_base",
			encoding:          "p50k_base",
			wantErr:           false,
			expectedVocabSize: 50257,
		},
		{
			name:     "invalid encoding",
			encoding: "invalid_encoding_xyz",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tok, err := NewTikToken(tt.encoding)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, tok)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, tok)
			assert.Equal(t, tt.expectedVocabSize, tok.VocabSize())
			assert.Equal(t, tt.encoding, tok.Name())
		})
	}
}

func TestTikToken_Roundtrip(t *testing.T) {
	tok, err := NewTikToken("cl100k_base")
	require.NoError(t, err)

	tests := []struct {
		name string
		text string
	}{
		{
			name: "simple text",
			text: "Hello, world!",
		},
		{
			name: "with newlines",
			text: "Hello\nWorld\n",
		},
		{
			name: "unicode",
			text: "Hello 世界! 🌍",
		},
		{
			name: "empty string",
			text: "",
		},
		{
			name: "long text",
			text: "The quick brown fox jumps over the lazy dog. " +
				"This is a longer piece of text to test tokenization.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode.
			tokens, err := tok.Encode(tt.text)
			require.NoError(t, err)

			// Decode.
			decoded, err := tok.Decode(tokens)
			require.NoError(t, err)

			// Verify roundtrip.
			assert.Equal(t, tt.text, decoded)
		})
	}
}

func TestTikToken_SpecialTokens(t *testing.T) {
	tok, err := NewTikToken("cl100k_base")
	require.NoError(t, err)

	t.Run("BOS token", func(t *testing.T) {
		bos := tok.BosToken()
		assert.Equal(t, int32(-1), bos, "tiktoken doesn't use BOS")
	})

	t.Run("EOS token", func(t *testing.T) {
		eos := tok.EosToken()
		assert.Equal(t, int32(100257), eos)
		assert.True(t, tok.IsSpecialToken(eos))
	})

	t.Run("PAD token", func(t *testing.T) {
		pad := tok.PadToken()
		assert.Equal(t, int32(-1), pad, "tiktoken doesn't define PAD")
	})

	t.Run("UNK token", func(t *testing.T) {
		unk := tok.UnkToken()
		assert.Equal(t, int32(-1), unk, "tiktoken uses BPE fallback")
	})

	t.Run("special token detection", func(t *testing.T) {
		// cl100k_base special tokens: 100256-100276 (ChatML tokens).
		assert.True(t, tok.IsSpecialToken(100256))
		assert.True(t, tok.IsSpecialToken(100270))
		assert.True(t, tok.IsSpecialToken(100276))
		assert.False(t, tok.IsSpecialToken(0))
		assert.False(t, tok.IsSpecialToken(1000))
	})
}

func TestTikToken_NewTikTokenForModel(t *testing.T) {
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
			name:      "invalid model",
			modelName: "invalid-model-xyz",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tok, err := NewTikTokenForModel(tt.modelName)
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

func TestTikToken_EmptyInput(t *testing.T) {
	tok, err := NewTikToken("cl100k_base")
	require.NoError(t, err)

	tokens, err := tok.Encode("")
	require.NoError(t, err)
	assert.Empty(t, tokens)

	decoded, err := tok.Decode([]int32{})
	require.NoError(t, err)
	assert.Equal(t, "", decoded)
}

func TestTikToken_VocabSize(t *testing.T) {
	tests := []struct {
		encoding          string
		expectedVocabSize int
	}{
		{"cl100k_base", 100256},
		{"p50k_base", 50257},
		{"r50k_base", 50257},
	}

	for _, tt := range tests {
		t.Run(tt.encoding, func(t *testing.T) {
			tok, err := NewTikToken(tt.encoding)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedVocabSize, tok.VocabSize())
		})
	}
}
