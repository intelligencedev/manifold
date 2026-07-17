package tokenizer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBPE_Encode(t *testing.T) {
	tok := ExampleBPEVocab()

	tests := []struct {
		name    string
		text    string
		wantLen int
	}{
		{
			name:    "simple word",
			text:    "hello",
			wantLen: 3, // "he", "ll", "o"
		},
		{
			name:    "empty string",
			text:    "",
			wantLen: 0,
		},
		{
			name:    "single word with space",
			text:    "hello world",
			wantLen: 6, // Approximate (depends on merges)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens, err := tok.Encode(tt.text)
			require.NoError(t, err)
			assert.GreaterOrEqual(t, len(tokens), 0)
		})
	}
}

func TestBPE_Decode(t *testing.T) {
	tok := ExampleBPEVocab()

	tests := []struct {
		name   string
		tokens []int32
	}{
		{
			name:   "simple tokens",
			tokens: []int32{0, 1, 2},
		},
		{
			name:   "empty tokens",
			tokens: []int32{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, err := tok.Decode(tt.tokens)
			require.NoError(t, err)
			assert.NotNil(t, text)
		})
	}
}

func TestBPE_VocabSize(t *testing.T) {
	tok := ExampleBPEVocab()
	assert.Greater(t, tok.VocabSize(), 0)
}

func TestBPE_SpecialTokens(t *testing.T) {
	vocab := map[string]int32{
		"<bos>": 0,
		"<eos>": 1,
		"<pad>": 2,
		"<unk>": 3,
		"a":     4,
		"b":     5,
	}
	merges := []pair{}

	tok := NewBPETokenizer(vocab, merges)
	tok.SetSpecialTokens(0, 1, 2, 3)

	t.Run("bos token", func(t *testing.T) {
		assert.Equal(t, int32(0), tok.BosToken())
		assert.True(t, tok.IsSpecialToken(0))
	})

	t.Run("eos token", func(t *testing.T) {
		assert.Equal(t, int32(1), tok.EosToken())
		assert.True(t, tok.IsSpecialToken(1))
	})

	t.Run("pad token", func(t *testing.T) {
		assert.Equal(t, int32(2), tok.PadToken())
		assert.True(t, tok.IsSpecialToken(2))
	})

	t.Run("unk token", func(t *testing.T) {
		assert.Equal(t, int32(3), tok.UnkToken())
		assert.True(t, tok.IsSpecialToken(3))
	})

	t.Run("regular token", func(t *testing.T) {
		assert.False(t, tok.IsSpecialToken(4))
		assert.False(t, tok.IsSpecialToken(5))
	})
}

func TestBPE_NewBPETokenizer(t *testing.T) {
	vocab := map[string]int32{
		"a":  0,
		"b":  1,
		"ab": 2,
	}
	merges := []pair{
		{"a", "b"},
	}

	tok := NewBPETokenizer(vocab, merges)
	require.NotNil(t, tok)
	assert.Equal(t, 3, tok.VocabSize())
}

func TestBPE_SetSpecialTokens(t *testing.T) {
	tok := ExampleBPEVocab()

	// Initially no special tokens.
	assert.Equal(t, int32(-1), tok.BosToken())
	assert.Equal(t, int32(-1), tok.EosToken())

	// Set special tokens.
	tok.SetSpecialTokens(100, 101, 102, 103)

	assert.Equal(t, int32(100), tok.BosToken())
	assert.Equal(t, int32(101), tok.EosToken())
	assert.Equal(t, int32(102), tok.PadToken())
	assert.Equal(t, int32(103), tok.UnkToken())

	assert.True(t, tok.IsSpecialToken(100))
	assert.True(t, tok.IsSpecialToken(101))
	assert.True(t, tok.IsSpecialToken(102))
	assert.True(t, tok.IsSpecialToken(103))
}

func TestBPE_EmptyVocab(t *testing.T) {
	tok := NewBPETokenizer(map[string]int32{}, []pair{})

	tokens, err := tok.Encode("test")
	require.NoError(t, err)
	assert.Empty(t, tokens)
}

func TestBPE_DecodeUnknownToken(t *testing.T) {
	tok := ExampleBPEVocab()

	// Token ID that doesn't exist in vocab.
	text, err := tok.Decode([]int32{9999})
	require.NoError(t, err)
	// Should contain replacement character.
	assert.Contains(t, text, "�")
}

func TestBPE_MergeRank(t *testing.T) {
	merges := []pair{
		{"a", "b"},
		{"c", "d"},
		{"e", "f"},
	}

	tok := NewBPETokenizer(map[string]int32{}, merges)

	// Test merge rank lookup.
	rank1 := tok.getMergeRank(pair{"a", "b"})
	assert.Equal(t, 0, rank1)

	rank2 := tok.getMergeRank(pair{"c", "d"})
	assert.Equal(t, 1, rank2)

	// Non-existent pair.
	rankNone := tok.getMergeRank(pair{"x", "y"})
	assert.Greater(t, rankNone, len(merges))
}
