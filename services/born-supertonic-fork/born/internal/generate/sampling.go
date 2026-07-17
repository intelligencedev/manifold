// Package generate provides text generation utilities for LLMs.
//
// This package implements sampling strategies and inference pipelines
// for autoregressive text generation.
package generate

import (
	"math"
	"math/rand"
	"sort"

	"github.com/born-ml/born/internal/tensor"
)

// SamplingConfig configures the sampling strategy for text generation.
type SamplingConfig struct {
	// Temperature controls randomness. 0 = greedy, 1 = normal, >1 = more random.
	Temperature float32

	// TopK limits sampling to top K tokens. 0 = disabled.
	TopK int

	// TopP (nucleus sampling) limits to tokens with cumulative prob < P. 1.0 = disabled.
	TopP float32

	// MinP filters tokens with prob < max_prob * MinP. 0 = disabled.
	MinP float32

	// Repetition control
	RepeatPenalty    float32 // Penalty for repeated tokens. 1.0 = no penalty.
	FrequencyPenalty float32 // Penalty based on frequency. 0 = disabled.
	PresencePenalty  float32 // Penalty for presence. 0 = disabled.
	RepeatWindow     int     // Number of tokens to consider. 0 = all.

	// Seed for reproducibility. -1 = random.
	Seed int64
}

// DefaultSamplingConfig returns sensible defaults for text generation.
func DefaultSamplingConfig() SamplingConfig {
	return SamplingConfig{
		Temperature:      1.0,
		TopK:             0,
		TopP:             1.0,
		MinP:             0.0,
		RepeatPenalty:    1.0,
		FrequencyPenalty: 0.0,
		PresencePenalty:  0.0,
		RepeatWindow:     64,
		Seed:             -1,
	}
}

// Sampler samples tokens from logits using configurable strategies.
type Sampler struct {
	config SamplingConfig
	rng    *rand.Rand
}

// NewSampler creates a new sampler with the given configuration.
func NewSampler(config SamplingConfig) *Sampler {
	var rng *rand.Rand
	if config.Seed >= 0 {
		rng = rand.New(rand.NewSource(config.Seed)) //nolint:gosec // Intentional deterministic seed for reproducibility
	} else {
		rng = rand.New(rand.NewSource(rand.Int63())) //nolint:gosec // User requested random seed
	}

	return &Sampler{
		config: config,
		rng:    rng,
	}
}

// Sample returns the next token ID from logits.
//
// Parameters:
//   - logits: raw model output, shape [vocab_size] or [..., vocab_size]
//   - previousTokens: tokens generated so far (for repetition penalty)
//
// The sampling process:
//  1. Apply repetition penalty
//  2. Apply temperature scaling
//  3. Apply Top-K filtering
//  4. Apply Top-P (nucleus) filtering
//  5. Apply Min-P filtering
//  6. Sample from distribution (or argmax if temperature=0)
func (s *Sampler) Sample(logits []float32, previousTokens []int32) int32 {
	// Make a copy to avoid modifying original
	logits = append([]float32{}, logits...)

	// 1. Apply repetition penalty
	if s.config.RepeatPenalty != 1.0 && len(previousTokens) > 0 {
		s.applyRepetitionPenalty(logits, previousTokens)
	}

	// 2. Apply frequency/presence penalties
	if s.config.FrequencyPenalty != 0 || s.config.PresencePenalty != 0 {
		s.applyFrequencyPenalty(logits, previousTokens)
	}

	// 3. Apply temperature
	if s.config.Temperature > 0 && s.config.Temperature != 1.0 {
		for i := range logits {
			logits[i] /= s.config.Temperature
		}
	}

	// Greedy decoding (temperature = 0)
	if s.config.Temperature == 0 {
		return s.argmax(logits)
	}

	// 4. Apply Top-K filter
	if s.config.TopK > 0 && s.config.TopK < len(logits) {
		logits = s.topKFilter(logits)
	}

	// 5. Apply Top-P (nucleus) filter
	if s.config.TopP < 1.0 && s.config.TopP > 0 {
		logits = s.topPFilter(logits)
	}

	// 6. Apply Min-P filter
	if s.config.MinP > 0 {
		logits = s.minPFilter(logits)
	}

	// Convert to probabilities
	probs := softmax(logits)

	// Sample from distribution
	return s.multinomial(probs)
}

// SampleTensor samples from a tensor of logits.
func (s *Sampler) SampleTensor(logits *tensor.Tensor[float32, tensor.Backend], previousTokens []int32) int32 {
	data := logits.Data()
	return s.Sample(data, previousTokens)
}

// argmax returns the index of the maximum value.
func (s *Sampler) argmax(logits []float32) int32 {
	maxIdx := 0
	maxVal := logits[0]
	for i, v := range logits[1:] {
		if v > maxVal {
			maxVal = v
			maxIdx = i + 1
		}
	}
	return int32(maxIdx)
}

// applyRepetitionPenalty penalizes tokens that appeared recently.
func (s *Sampler) applyRepetitionPenalty(logits []float32, prev []int32) {
	penalty := s.config.RepeatPenalty
	window := s.config.RepeatWindow

	// Get recent tokens
	recent := prev
	if window > 0 && len(prev) > window {
		recent = prev[len(prev)-window:]
	}

	// Get unique tokens
	seen := make(map[int32]bool)
	for _, tok := range recent {
		seen[tok] = true
	}

	// Apply penalty
	for tok := range seen {
		if int(tok) < len(logits) {
			if logits[tok] > 0 {
				logits[tok] /= penalty
			} else {
				logits[tok] *= penalty
			}
		}
	}
}

// applyFrequencyPenalty penalizes based on token frequency.
func (s *Sampler) applyFrequencyPenalty(logits []float32, prev []int32) {
	freqPen := s.config.FrequencyPenalty
	presPen := s.config.PresencePenalty
	window := s.config.RepeatWindow

	recent := prev
	if window > 0 && len(prev) > window {
		recent = prev[len(prev)-window:]
	}

	// Count frequencies
	freq := make(map[int32]int)
	for _, tok := range recent {
		freq[tok]++
	}

	// Apply penalties
	for tok, count := range freq {
		if int(tok) < len(logits) {
			// Frequency penalty: subtract based on count
			logits[tok] -= freqPen * float32(count)
			// Presence penalty: subtract if present at all
			if presPen != 0 {
				logits[tok] -= presPen
			}
		}
	}
}

// topKFilter keeps only top K logits, sets rest to -inf.
func (s *Sampler) topKFilter(logits []float32) []float32 {
	k := s.config.TopK
	if k >= len(logits) {
		return logits
	}

	// Find k-th largest value
	sorted := append([]float32{}, logits...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] > sorted[j] })
	threshold := sorted[k-1]

	// Filter
	for i := range logits {
		if logits[i] < threshold {
			logits[i] = float32(math.Inf(-1))
		}
	}

	return logits
}

// topPFilter implements nucleus sampling.
func (s *Sampler) topPFilter(logits []float32) []float32 {
	p := s.config.TopP

	// Get probabilities
	probs := softmax(logits)

	// Sort by probability (descending)
	type indexedProb struct {
		idx  int
		prob float32
	}
	indexed := make([]indexedProb, len(probs))
	for i, prob := range probs {
		indexed[i] = indexedProb{i, prob}
	}
	sort.Slice(indexed, func(i, j int) bool { return indexed[i].prob > indexed[j].prob })

	// Find cutoff
	cumSum := float32(0)
	cutoffIdx := len(indexed) - 1
	for i, ip := range indexed {
		cumSum += ip.prob
		if cumSum > p {
			cutoffIdx = i
			break
		}
	}

	// Always keep at least one token
	if cutoffIdx == 0 {
		cutoffIdx = 1
	}

	// Create mask of tokens to keep
	keep := make(map[int]bool)
	for i := 0; i <= cutoffIdx; i++ {
		keep[indexed[i].idx] = true
	}

	// Filter logits
	for i := range logits {
		if !keep[i] {
			logits[i] = float32(math.Inf(-1))
		}
	}

	return logits
}

// minPFilter keeps tokens with prob >= max_prob * minP.
func (s *Sampler) minPFilter(logits []float32) []float32 {
	minP := s.config.MinP

	probs := softmax(logits)

	// Find max probability
	maxProb := float32(0)
	for _, p := range probs {
		if p > maxProb {
			maxProb = p
		}
	}

	threshold := maxProb * minP

	// Filter
	for i := range logits {
		if probs[i] < threshold {
			logits[i] = float32(math.Inf(-1))
		}
	}

	return logits
}

// multinomial samples from a categorical distribution.
func (s *Sampler) multinomial(probs []float32) int32 {
	r := s.rng.Float32()

	cumSum := float32(0)
	for i, p := range probs {
		cumSum += p
		if r < cumSum {
			return int32(i)
		}
	}

	// Return last token if rounding errors.
	return int32(len(probs) - 1) //nolint:gosec // G115: integer overflow conversion int -> int32
}

// softmax converts logits to probabilities.
func softmax(logits []float32) []float32 {
	// Find max for numerical stability
	maxVal := logits[0]
	for _, v := range logits[1:] {
		if v > maxVal {
			maxVal = v
		}
	}

	// Compute exp and sum
	probs := make([]float32, len(logits))
	sum := float32(0)
	for i, v := range logits {
		if math.IsInf(float64(v), -1) {
			probs[i] = 0
		} else {
			probs[i] = float32(math.Exp(float64(v - maxVal)))
			sum += probs[i]
		}
	}

	// Normalize
	if sum > 0 {
		for i := range probs {
			probs[i] /= sum
		}
	}

	return probs
}
