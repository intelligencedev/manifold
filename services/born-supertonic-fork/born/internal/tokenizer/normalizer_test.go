package tokenizer

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrependNormalizer(t *testing.T) {
	n := &PrependNormalizer{Prepend: "\u2581"}
	assert.Equal(t, "\u2581hello", n.Normalize("hello"))
	assert.Equal(t, "\u2581", n.Normalize(""))
}

func TestReplaceNormalizer(t *testing.T) {
	n := &ReplaceNormalizer{Pattern: " ", Content: "\u2581"}
	assert.Equal(t, "hello\u2581world", n.Normalize("hello world"))
	assert.Equal(t, "nospaces", n.Normalize("nospaces"))
}

func TestSequenceNormalizer(t *testing.T) {
	// SentencePiece: Prepend '▁' then replace spaces with '▁'.
	n := &SequenceNormalizer{
		Normalizers: []Normalizer{
			&PrependNormalizer{Prepend: "\u2581"},
			&ReplaceNormalizer{Pattern: " ", Content: "\u2581"},
		},
	}
	assert.Equal(t, "\u2581The\u2581capital", n.Normalize("The capital"))
	assert.Equal(t, "\u2581hello", n.Normalize("hello"))
}

func TestLowercaseNormalizer(t *testing.T) {
	n := &LowercaseNormalizer{}
	assert.Equal(t, "hello world", n.Normalize("Hello World"))
}

func TestStripNormalizer(t *testing.T) {
	n := &StripNormalizer{}
	assert.Equal(t, "hello", n.Normalize("  hello  "))
}

func TestParseNormalizer_SentencePiece(t *testing.T) {
	// Exact JSON from TinyLlama tokenizer.json.
	raw := json.RawMessage(`{
		"type": "Sequence",
		"normalizers": [
			{"type": "Prepend", "prepend": "▁"},
			{"type": "Replace", "pattern": {"String": " "}, "content": "▁"}
		]
	}`)

	n, err := parseNormalizer(raw)
	require.NoError(t, err)
	require.NotNil(t, n)

	assert.Equal(t, "\u2581The\u2581capital\u2581of\u2581France\u2581is", n.Normalize("The capital of France is"))
}

func TestParseNormalizer_Null(t *testing.T) {
	n, err := parseNormalizer(json.RawMessage(`null`))
	require.NoError(t, err)
	assert.Nil(t, n)
}

func TestParseNormalizer_Prepend(t *testing.T) {
	raw := json.RawMessage(`{"type": "Prepend", "prepend": "▁"}`)
	n, err := parseNormalizer(raw)
	require.NoError(t, err)
	require.NotNil(t, n)
	assert.Equal(t, "\u2581hello", n.Normalize("hello"))
}

func TestParseNormalizer_Replace(t *testing.T) {
	raw := json.RawMessage(`{"type": "Replace", "pattern": {"String": " "}, "content": "_"}`)
	n, err := parseNormalizer(raw)
	require.NoError(t, err)
	require.NotNil(t, n)
	assert.Equal(t, "a_b_c", n.Normalize("a b c"))
}

func TestParseNormalizer_UnsupportedRegex(t *testing.T) {
	raw := json.RawMessage(`{"type": "Replace", "pattern": {"Regex": "\\s+"}, "content": " "}`)
	n, err := parseNormalizer(raw)
	require.NoError(t, err)
	assert.Nil(t, n, "regex Replace not supported, should return nil")
}

func TestParseNormalizer_UnknownType(t *testing.T) {
	raw := json.RawMessage(`{"type": "SomeFutureNormalizer"}`)
	n, err := parseNormalizer(raw)
	require.NoError(t, err)
	assert.Nil(t, n, "unknown type should be silently skipped")
}

func TestSplitWords_SentencePiece(t *testing.T) {
	words := splitWords("\u2581The\u2581capital\u2581of\u2581France")
	assert.Equal(t, []string{"\u2581The", "\u2581capital", "\u2581of", "\u2581France"}, words)
}

func TestSplitWords_Regular(t *testing.T) {
	words := splitWords("hello world")
	assert.Equal(t, []string{"hello", "world"}, words)
}

func TestSplitWords_SingleWord(t *testing.T) {
	words := splitWords("\u2581hello")
	assert.Equal(t, []string{"\u2581hello"}, words)
}
