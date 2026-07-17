package generate

import (
	"testing"

	"github.com/born-ml/born/internal/tensor"
	"github.com/born-ml/born/internal/tokenizer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// specialTokens holds special token IDs for testing.
type specialTokens struct {
	BOS int32
	EOS int32
	PAD int32
	UNK int32
}

// mockTokenizer is a simple tokenizer for testing.
type mockTokenizer struct {
	vocab    map[string]int32
	invVocab map[int32]string
	special  specialTokens
}

func newMockTokenizer() *mockTokenizer {
	vocab := map[string]int32{
		"<pad>":  0,
		"<bos>":  1,
		"<eos>":  2,
		"<unk>":  3,
		"hello":  4,
		"world":  5,
		"test":   6,
		"the":    7,
		"a":      8,
		" ":      9,
		"!":      10,
		"answer": 11,
		"is":     12,
		"42":     13,
	}

	invVocab := make(map[int32]string)
	for k, v := range vocab {
		invVocab[v] = k
	}

	return &mockTokenizer{
		vocab:    vocab,
		invVocab: invVocab,
		special: specialTokens{
			BOS: 1,
			EOS: 2,
			PAD: 0,
			UNK: 3,
		},
	}
}

func (t *mockTokenizer) Encode(text string) ([]int32, error) {
	// Simple word-level tokenization for testing
	tokens := []int32{}
	for _, word := range []string{text} {
		if id, ok := t.vocab[word]; ok {
			tokens = append(tokens, id)
		} else {
			tokens = append(tokens, t.special.UNK)
		}
	}
	if len(tokens) == 0 {
		tokens = append(tokens, t.special.UNK)
	}
	return tokens, nil
}

func (t *mockTokenizer) Decode(tokens []int32) (string, error) {
	result := ""
	for _, tok := range tokens {
		if s, ok := t.invVocab[tok]; ok {
			result += s
		}
	}
	return result, nil
}

func (t *mockTokenizer) VocabSize() int                  { return len(t.vocab) }
func (t *mockTokenizer) GetVocab() map[string]int32      { return t.vocab }
func (t *mockTokenizer) BosToken() int32                 { return t.special.BOS }
func (t *mockTokenizer) EosToken() int32                 { return t.special.EOS }
func (t *mockTokenizer) PadToken() int32                 { return t.special.PAD }
func (t *mockTokenizer) UnkToken() int32                 { return t.special.UNK }
func (t *mockTokenizer) IsSpecialToken(token int32) bool { return token < 4 }

// mockChatTemplate is a simple chat template for testing.
type mockChatTemplate struct{}

func (t *mockChatTemplate) Apply(messages []tokenizer.ChatMessage) string {
	result := ""
	for _, m := range messages {
		result += m.Role + ": " + m.Content + "\n"
	}
	return result
}

func (t *mockChatTemplate) Name() string {
	return "mock"
}

// mockModel is a simple model for testing.
type mockModel struct {
	vocabSize    int
	responseSeq  []int32 // Predetermined response tokens
	currentIndex int
}

func newMockModel(responseSeq []int32) *mockModel {
	return &mockModel{
		vocabSize:   100,
		responseSeq: responseSeq,
	}
}

func (m *mockModel) Forward(_ *tensor.RawTensor, _ KVCache, _ int) *tensor.RawTensor {
	// Return logits that favor the next token in responseSeq
	logits := make([]float32, m.vocabSize)
	for i := range logits {
		logits[i] = -10.0
	}

	// Set high logit for next response token
	if m.currentIndex < len(m.responseSeq) {
		logits[m.responseSeq[m.currentIndex]] = 10.0
		m.currentIndex++
	} else {
		// Return EOS
		logits[2] = 10.0 // EOS token
	}

	// Create output tensor [1, 1, vocab_size]
	shape := tensor.Shape{1, 1, m.vocabSize}
	raw, _ := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	data := raw.AsFloat32()
	copy(data, logits)

	return raw
}

func (m *mockModel) VocabSize() int {
	return m.vocabSize
}

func (m *mockModel) Reset() {
	m.currentIndex = 0
}

func TestTextGenerator_Generate(t *testing.T) {
	tok := newMockTokenizer()
	model := newMockModel([]int32{7, 9, 11, 9, 12, 9, 13}) // "the answer is 42"

	config := SamplingConfig{
		Temperature: 0, // Greedy
	}

	gen := NewTextGenerator(model, tok, config)

	result, err := gen.Generate("test", GenerateConfig{
		MaxTokens: 10,
	})

	require.NoError(t, err)
	assert.NotEmpty(t, result)
}

func TestTextGenerator_GenerateWithEOS(t *testing.T) {
	tok := newMockTokenizer()
	model := newMockModel([]int32{4, 5}) // "hello world" then EOS

	config := SamplingConfig{
		Temperature: 0,
	}

	gen := NewTextGenerator(model, tok, config)

	result, err := gen.Generate("test", GenerateConfig{
		MaxTokens: 100, // High limit - should stop at EOS
	})

	require.NoError(t, err)
	// Should contain hello and world before EOS stops generation
	assert.Contains(t, result, "hello")
}

func TestTextGenerator_GenerateStream(t *testing.T) {
	tok := newMockTokenizer()
	model := newMockModel([]int32{4, 5}) // "hello world"

	config := SamplingConfig{
		Temperature: 0,
	}

	gen := NewTextGenerator(model, tok, config)

	stream, err := gen.GenerateStream("test", GenerateConfig{
		MaxTokens: 10,
		Stream:    true,
	})

	require.NoError(t, err)

	tokens := make([]string, 0, 10)
	for result := range stream {
		require.NoError(t, result.Error)
		tokens = append(tokens, result.Token)
		if result.Done {
			break
		}
	}

	assert.NotEmpty(t, tokens)
}

func TestTextGenerator_StopString(t *testing.T) {
	tok := newMockTokenizer()
	model := newMockModel([]int32{4, 5, 10}) // "hello world !"

	config := SamplingConfig{
		Temperature: 0,
	}

	gen := NewTextGenerator(model, tok, config)

	result, err := gen.Generate("test", GenerateConfig{
		MaxTokens:   100,
		StopStrings: []string{"world"},
	})

	require.NoError(t, err)
	assert.Contains(t, result, "world")
}

func TestTextGenerator_StopToken(t *testing.T) {
	tok := newMockTokenizer()
	model := newMockModel([]int32{4, 5, 6, 7}) // Should stop at 6 (test)

	config := SamplingConfig{
		Temperature: 0,
	}

	gen := NewTextGenerator(model, tok, config)

	generated := make([]int32, 0, 10)
	stream, _ := gen.GenerateStream("test", GenerateConfig{
		MaxTokens:  100,
		StopTokens: []int32{6}, // Stop at "test"
	})

	for result := range stream {
		generated = append(generated, result.TokenID)
		if result.Done {
			assert.Equal(t, "stop_token", result.Reason)
			break
		}
	}

	// Should have stopped at token 6
	assert.Contains(t, generated, int32(6))
}

func TestTextGenerator_MaxTokens(t *testing.T) {
	tok := newMockTokenizer()
	model := newMockModel([]int32{4, 4, 4, 4, 4, 4, 4, 4, 4, 4}) // Many tokens

	config := SamplingConfig{
		Temperature: 0,
	}

	gen := NewTextGenerator(model, tok, config)

	var count int
	var lastReason string
	stream, _ := gen.GenerateStream("test", GenerateConfig{
		MaxTokens: 3,
	})

	for result := range stream {
		count++
		if result.Done {
			lastReason = result.Reason
			break
		}
	}

	assert.Equal(t, 3, count)
	assert.Equal(t, "max_tokens", lastReason)
}

func TestTextGenerator_MinTokens(t *testing.T) {
	tok := newMockTokenizer()
	// EOS token (2) at position 0, but min tokens should prevent early stop
	model := newMockModel([]int32{2, 4, 5})

	config := SamplingConfig{
		Temperature: 0,
	}

	gen := NewTextGenerator(model, tok, config)

	var count int
	stream, _ := gen.GenerateStream("test", GenerateConfig{
		MaxTokens: 10,
		MinTokens: 2,
	})

	for result := range stream {
		count++
		if result.Done {
			break
		}
	}

	// Should generate at least 2 tokens despite EOS
	assert.GreaterOrEqual(t, count, 2)
}

func TestTextGenerator_Chat(t *testing.T) {
	tok := newMockTokenizer()
	model := newMockModel([]int32{4, 5}) // "hello world"
	template := &mockChatTemplate{}

	config := SamplingConfig{
		Temperature: 0,
	}

	gen := NewTextGenerator(model, tok, config)

	messages := []tokenizer.ChatMessage{
		{Role: "user", Content: "Hello!"},
	}

	result, err := gen.Chat(messages, template, GenerateConfig{
		MaxTokens: 10,
	})

	require.NoError(t, err)
	assert.NotEmpty(t, result)
}

func TestTextGenerator_EchoPrompt(t *testing.T) {
	tok := newMockTokenizer()
	model := newMockModel([]int32{5}) // "world"

	config := SamplingConfig{
		Temperature: 0,
	}

	gen := NewTextGenerator(model, tok, config)

	result, err := gen.Generate("hello", GenerateConfig{
		MaxTokens:  5,
		EchoPrompt: true,
	})

	require.NoError(t, err)
	assert.NotEmpty(t, result) // "hello" + generated
}

func TestGenerateConfig_Defaults(t *testing.T) {
	config := DefaultGenerateConfig()

	assert.Equal(t, 256, config.MaxTokens)
	assert.Equal(t, 0, config.MinTokens)
	assert.Nil(t, config.StopStrings)
	assert.Nil(t, config.StopTokens)
	assert.False(t, config.Stream)
	assert.False(t, config.EchoPrompt)
}

func TestGetLastLogits(t *testing.T) {
	t.Run("3D tensor", func(t *testing.T) {
		shape := tensor.Shape{1, 3, 5} // batch=1, seq=3, vocab=5
		raw, _ := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
		data := raw.AsFloat32()
		// Fill with values
		for i := range data {
			data[i] = float32(i)
		}

		lastLogits := getLastLogits(raw)
		assert.Equal(t, 5, len(lastLogits))
		// Last position starts at index 2*5=10
		assert.Equal(t, float32(10), lastLogits[0])
	})

	t.Run("2D tensor", func(t *testing.T) {
		shape := tensor.Shape{3, 5} // seq=3, vocab=5
		raw, _ := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
		data := raw.AsFloat32()
		for i := range data {
			data[i] = float32(i)
		}

		lastLogits := getLastLogits(raw)
		assert.Equal(t, 5, len(lastLogits))
		assert.Equal(t, float32(10), lastLogits[0])
	})

	t.Run("1D tensor", func(t *testing.T) {
		shape := tensor.Shape{5} // vocab=5
		raw, _ := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
		data := raw.AsFloat32()
		for i := range data {
			data[i] = float32(i)
		}

		lastLogits := getLastLogits(raw)
		assert.Equal(t, 5, len(lastLogits))
		assert.Equal(t, float32(0), lastLogits[0])
	})
}

func TestCreateInputTensor(t *testing.T) {
	tokens := []int32{1, 2, 3, 4, 5}
	raw := createInputTensor(tokens)

	assert.Equal(t, tensor.Shape{1, 5}, raw.Shape())
	assert.Equal(t, tensor.Int32, raw.DType())

	data := raw.AsInt32()
	assert.Equal(t, tokens, data)
}

func BenchmarkGenerator_Generate(b *testing.B) {
	tok := newMockTokenizer()
	model := newMockModel([]int32{4, 5, 6, 7, 8, 9, 10, 11, 12, 13})

	config := SamplingConfig{
		Temperature: 0,
	}

	gen := NewTextGenerator(model, tok, config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		model.Reset()
		_, _ = gen.Generate("test", GenerateConfig{
			MaxTokens: 10,
		})
	}
}
