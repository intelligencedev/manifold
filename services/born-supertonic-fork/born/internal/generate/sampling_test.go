package generate

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGreedySampling(t *testing.T) {
	config := SamplingConfig{Temperature: 0} // Greedy
	sampler := NewSampler(config)

	// [0.1, 0.3, 0.6] after softmax -> should always return 2
	logits := []float32{-1, 0, 1}

	for i := 0; i < 10; i++ {
		token := sampler.Sample(logits, nil)
		assert.Equal(t, int32(2), token, "Greedy should always pick max")
	}
}

func TestGreedySampling_LargeVocab(t *testing.T) {
	config := SamplingConfig{Temperature: 0}
	sampler := NewSampler(config)

	// Simulate large vocab with clear max
	logits := make([]float32, 50000)
	for i := range logits {
		logits[i] = float32(i) * 0.001
	}
	logits[12345] = 100.0 // Clear max

	token := sampler.Sample(logits, nil)
	assert.Equal(t, int32(12345), token)
}

func TestTopKSampling(t *testing.T) {
	config := SamplingConfig{
		Temperature: 1.0,
		TopK:        2,
		Seed:        42,
	}
	sampler := NewSampler(config)

	logits := []float32{1, 2, 3, 4, 5}

	// Should only sample from top 2 tokens (indices 3, 4)
	counts := make(map[int32]int)
	for i := 0; i < 100; i++ {
		token := sampler.Sample(logits, nil)
		counts[token]++
	}

	// Tokens 0, 1, 2 should never be sampled
	assert.Equal(t, 0, counts[0]+counts[1]+counts[2], "Should not sample from filtered tokens")
	assert.Greater(t, counts[3]+counts[4], 0, "Should sample from top-k tokens")
}

func TestTopPSampling(t *testing.T) {
	config := SamplingConfig{
		Temperature: 1.0,
		TopP:        0.5,
		Seed:        42,
	}
	sampler := NewSampler(config)

	// Create logits where top token has >50% probability
	logits := []float32{-10, -10, -10, 0, 5}

	counts := make(map[int32]int)
	for i := 0; i < 100; i++ {
		token := sampler.Sample(logits, nil)
		counts[token]++
	}

	// Token 4 should dominate (highest prob)
	assert.Greater(t, counts[4], 50, "Highest prob token should be sampled most")
}

func TestMinPSampling(t *testing.T) {
	config := SamplingConfig{
		Temperature: 1.0,
		MinP:        0.5,
		Seed:        42,
	}
	sampler := NewSampler(config)

	// Create logits with one dominant token
	logits := []float32{0, 0, 0, 0, 10}

	counts := make(map[int32]int)
	for i := 0; i < 100; i++ {
		token := sampler.Sample(logits, nil)
		counts[token]++
	}

	// Token 4 should be sampled almost exclusively due to min-p filter
	assert.Greater(t, counts[4], 90)
}

func TestTemperatureSampling(t *testing.T) {
	t.Run("low temperature", func(t *testing.T) {
		config := SamplingConfig{
			Temperature: 0.1,
			Seed:        42,
		}
		sampler := NewSampler(config)

		logits := []float32{1, 2, 3}

		// Low temperature should heavily favor max
		counts := make(map[int32]int)
		for i := 0; i < 100; i++ {
			token := sampler.Sample(logits, nil)
			counts[token]++
		}

		assert.Greater(t, counts[2], 90, "Low temp should favor max")
	})

	t.Run("high temperature", func(t *testing.T) {
		config := SamplingConfig{
			Temperature: 2.0,
			Seed:        42,
		}
		sampler := NewSampler(config)

		logits := []float32{1, 2, 3}

		counts := make(map[int32]int)
		for i := 0; i < 100; i++ {
			token := sampler.Sample(logits, nil)
			counts[token]++
		}

		// High temperature should distribute more evenly
		assert.Greater(t, counts[0]+counts[1], 5, "High temp should distribute samples")
	})
}

func TestRepetitionPenalty(t *testing.T) {
	config := SamplingConfig{
		Temperature:   0, // Greedy to test penalty effect
		RepeatPenalty: 2.0,
	}
	sampler := NewSampler(config)

	// All tokens equal, but token 0 was repeated
	logits := []float32{1.0, 1.0, 1.0}
	prev := []int32{0, 0, 0} // Token 0 repeated

	token := sampler.Sample(logits, prev)

	// Token 0 should be penalized, so another token should be chosen
	assert.NotEqual(t, int32(0), token, "Penalized token should not be chosen")
}

func TestFrequencyPenalty(t *testing.T) {
	config := SamplingConfig{
		Temperature:      0,
		RepeatPenalty:    1.0, // Disable repeat penalty
		FrequencyPenalty: 2.0, // Strong frequency penalty
	}
	sampler := NewSampler(config)

	// Token 0 has slightly higher logit but high frequency
	// 1.5 - (2.0 * 5) = 1.5 - 10 = -8.5
	// Token 1 stays at 1.0
	logits := []float32{1.5, 1.0, 0.5}
	prev := []int32{0, 0, 0, 0, 0} // Token 0 appeared 5 times

	token := sampler.Sample(logits, prev)

	// Token 0 should be heavily penalized, token 1 should win
	assert.Equal(t, int32(1), token, "Frequency penalized token should not be chosen")
}

func TestPresencePenalty(t *testing.T) {
	config := SamplingConfig{
		Temperature:     0,
		RepeatPenalty:   1.0, // Disable repeat penalty
		PresencePenalty: 5.0, // Strong presence penalty
	}
	sampler := NewSampler(config)

	// Token 0: 2.0 - 5.0 = -3.0
	// Token 1: 1.9 (untouched)
	logits := []float32{2.0, 1.9, 1.0}
	prev := []int32{0} // Token 0 appeared once

	token := sampler.Sample(logits, prev)

	// Token 0 should be penalized enough for token 1 to win
	assert.Equal(t, int32(1), token, "Presence penalty should make token 1 win")
}

func TestRepeatWindow(t *testing.T) {
	config := SamplingConfig{
		Temperature:   0,
		RepeatPenalty: 10.0,
		RepeatWindow:  3,
	}
	sampler := NewSampler(config)

	logits := []float32{5.0, 1.0, 1.0}
	// Token 0 appeared early but outside window
	prev := []int32{0, 1, 2, 1, 2}

	token := sampler.Sample(logits, prev)

	// Token 0 should NOT be penalized (outside window)
	assert.Equal(t, int32(0), token)
}

func TestDeterministicWithSeed(t *testing.T) {
	config := SamplingConfig{
		Temperature: 1.0,
		TopK:        10,
		Seed:        12345,
	}

	logits := make([]float32, 1000)
	for i := range logits {
		logits[i] = float32(i) * 0.01
	}

	// Same seed should give same results
	sampler1 := NewSampler(config)
	sampler2 := NewSampler(config)

	for i := 0; i < 10; i++ {
		t1 := sampler1.Sample(logits, nil)
		t2 := sampler2.Sample(logits, nil)
		assert.Equal(t, t1, t2, "Same seed should give same results")
	}
}

func TestSoftmax(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		logits := []float32{0, 0, 0}
		probs := softmax(logits)

		// Should be uniform
		for _, p := range probs {
			assert.InDelta(t, 1.0/3.0, p, 0.001)
		}
	})

	t.Run("numerical stability", func(t *testing.T) {
		// Large values should not overflow
		logits := []float32{1000, 1001, 1002}
		probs := softmax(logits)

		sum := float32(0)
		for _, p := range probs {
			assert.False(t, math.IsNaN(float64(p)), "Should not be NaN")
			assert.False(t, math.IsInf(float64(p), 0), "Should not be Inf")
			sum += p
		}
		assert.InDelta(t, 1.0, sum, 0.001, "Should sum to 1")
	})

	t.Run("with negative infinity", func(t *testing.T) {
		logits := []float32{0, float32(math.Inf(-1)), 0}
		probs := softmax(logits)

		assert.InDelta(t, 0.5, probs[0], 0.001)
		assert.Equal(t, float32(0), probs[1])
		assert.InDelta(t, 0.5, probs[2], 0.001)
	})
}

func TestDefaultSamplingConfig(t *testing.T) {
	config := DefaultSamplingConfig()

	assert.Equal(t, float32(1.0), config.Temperature)
	assert.Equal(t, 0, config.TopK)
	assert.Equal(t, float32(1.0), config.TopP)
	assert.Equal(t, float32(0.0), config.MinP)
	assert.Equal(t, float32(1.0), config.RepeatPenalty)
	assert.Equal(t, float32(0.0), config.FrequencyPenalty)
	assert.Equal(t, float32(0.0), config.PresencePenalty)
	assert.Equal(t, 64, config.RepeatWindow)
	assert.Equal(t, int64(-1), config.Seed)
}

func TestCombinedSampling(t *testing.T) {
	// Test combining multiple strategies
	config := SamplingConfig{
		Temperature:   0.8,
		TopK:          5,
		TopP:          0.9,
		RepeatPenalty: 1.1,
		Seed:          42,
	}
	sampler := NewSampler(config)

	logits := make([]float32, 100)
	for i := range logits {
		logits[i] = float32(i) * 0.1
	}

	prev := []int32{95, 96, 97, 98, 99} // Highest tokens repeated

	// Should not panic and should sample from allowed range
	token := sampler.Sample(logits, prev)
	require.GreaterOrEqual(t, token, int32(0))
	require.Less(t, token, int32(100))
}

func BenchmarkSampling(b *testing.B) {
	config := SamplingConfig{
		Temperature:   1.0,
		TopK:          50,
		TopP:          0.9,
		RepeatPenalty: 1.1,
		Seed:          42,
	}
	sampler := NewSampler(config)

	logits := make([]float32, 50000) // Typical vocab size
	for i := range logits {
		logits[i] = float32(i) * 0.0001
	}
	prev := make([]int32, 100)
	for i := range prev {
		prev[i] = int32(i * 500)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sampler.Sample(logits, prev)
	}
}

func BenchmarkSoftmax(b *testing.B) {
	logits := make([]float32, 50000)
	for i := range logits {
		logits[i] = float32(i) * 0.0001
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		softmax(logits)
	}
}
