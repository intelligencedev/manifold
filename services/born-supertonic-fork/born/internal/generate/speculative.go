package generate

import (
	"fmt"
	"math/rand"
)

// SpeculativeConfig configures speculative decoding.
type SpeculativeConfig struct {
	// DraftModel is the small fast model for speculation.
	DraftModel LLMModel

	// TargetModel is the large accurate model for verification.
	TargetModel LLMModel

	// NumSpeculate is the number of tokens to speculate (default: 5).
	NumSpeculate int

	// Sampling is the sampling configuration.
	Sampling SamplingConfig
}

// DefaultSpeculativeConfig returns sensible defaults for speculative decoding.
func DefaultSpeculativeConfig(draftModel, targetModel LLMModel) SpeculativeConfig {
	return SpeculativeConfig{
		DraftModel:   draftModel,
		TargetModel:  targetModel,
		NumSpeculate: 5,
		Sampling:     DefaultSamplingConfig(),
	}
}

// SpeculativeGenerator generates text using speculative decoding.
//
// Algorithm:
//  1. Draft model generates K tokens quickly
//  2. Target model verifies all K tokens in parallel (single forward pass)
//  3. Accept matching tokens using modified rejection sampling
//  4. Repeat from first rejected token
//
// This provides speedup by:
//   - Draft model is faster (smaller)
//   - Target model processes K tokens in one forward pass (parallel)
//   - Only rejected tokens require sequential decoding
//
// Example:
//
//	config := SpeculativeConfig{
//	    DraftModel:   smallModel,   // Fast 1B model
//	    TargetModel:  largeModel,   // Accurate 7B model
//	    NumSpeculate: 5,            // Speculate 5 tokens ahead
//	    Sampling:     DefaultSamplingConfig(),
//	}
//	generator := NewSpeculativeGenerator(config)
//	generator.SetCaches(draftCache, targetCache)
//
//	tokens, acceptRate, err := generator.Generate(inputIDs, maxTokens)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Generated %d tokens with %.1f%% acceptance rate\n",
//	    len(tokens), acceptRate*100)
type SpeculativeGenerator struct {
	config        SpeculativeConfig
	draftSampler  *Sampler
	targetSampler *Sampler
	draftCache    KVCache
	targetCache   KVCache
	rng           *rand.Rand

	// Stats
	totalDrafted  int
	totalAccepted int
}

// NewSpeculativeGenerator creates a new speculative decoding generator.
func NewSpeculativeGenerator(config SpeculativeConfig) *SpeculativeGenerator {
	if config.NumSpeculate <= 0 {
		config.NumSpeculate = 5
	}

	var rng *rand.Rand
	if config.Sampling.Seed >= 0 {
		//nolint:gosec // math/rand is appropriate for ML sampling with deterministic seed
		rng = rand.New(rand.NewSource(config.Sampling.Seed))
	} else {
		//nolint:gosec // math/rand is appropriate for ML sampling with random seed
		rng = rand.New(rand.NewSource(rand.Int63()))
	}

	return &SpeculativeGenerator{
		config:        config,
		draftSampler:  NewSampler(config.Sampling),
		targetSampler: NewSampler(config.Sampling),
		rng:           rng,
	}
}

// SetCaches sets KV caches for draft and target models.
func (sg *SpeculativeGenerator) SetCaches(draft, target KVCache) {
	sg.draftCache = draft
	sg.targetCache = target
}

// ClearCaches clears both KV caches.
func (sg *SpeculativeGenerator) ClearCaches() {
	if sg.draftCache != nil {
		sg.draftCache.Clear()
	}
	if sg.targetCache != nil {
		sg.targetCache.Clear()
	}
	sg.totalDrafted = 0
	sg.totalAccepted = 0
}

// Generate generates text using speculative decoding.
// Returns generated tokens and acceptance rate.
func (sg *SpeculativeGenerator) Generate(
	inputIDs []int32,
	maxTokens int,
) ([]int32, float32, error) {
	if len(inputIDs) == 0 {
		return nil, 0, fmt.Errorf("empty input")
	}

	if sg.config.DraftModel.VocabSize() != sg.config.TargetModel.VocabSize() {
		return nil, 0, fmt.Errorf("draft and target models must have same vocab size")
	}

	// Reset stats
	sg.totalDrafted = 0
	sg.totalAccepted = 0

	// Prefill: process input with both models
	inputTensor := createInputTensor(inputIDs)
	_ = sg.config.DraftModel.Forward(inputTensor, sg.draftCache, 0)
	_ = sg.config.TargetModel.Forward(inputTensor, sg.targetCache, 0)

	generated := make([]int32, 0, maxTokens)
	prevTokens := append([]int32{}, inputIDs...)
	startPos := len(inputIDs)

	for len(generated) < maxTokens {
		// 1. Draft model generates K tokens
		numSpeculate := minInt(sg.config.NumSpeculate, maxTokens-len(generated))
		draftTokens, draftLogits := sg.speculate(prevTokens, startPos, numSpeculate)

		// 2. Target model verifies all K tokens in parallel
		targetLogits := sg.verify(prevTokens, draftTokens, startPos)

		// 3. Accept tokens using modified rejection sampling
		numAccepted, resampledToken := sg.accept(draftTokens, draftLogits, targetLogits)

		// Add accepted tokens
		for i := 0; i < numAccepted; i++ {
			generated = append(generated, draftTokens[i])
			prevTokens = append(prevTokens, draftTokens[i])
		}

		// Add resampled token (either from rejection or continuation)
		if len(generated) < maxTokens {
			generated = append(generated, resampledToken)
			prevTokens = append(prevTokens, resampledToken)
		}

		startPos += numAccepted + 1
	}

	// Calculate acceptance rate
	var acceptanceRate float32
	if sg.totalDrafted > 0 {
		acceptanceRate = float32(sg.totalAccepted) / float32(sg.totalDrafted)
	}

	return generated, acceptanceRate, nil
}

// speculate generates K tokens using draft model.
// Returns tokens and their logits.
func (sg *SpeculativeGenerator) speculate(
	inputIDs []int32,
	startPos int,
	numTokens int,
) ([]int32, [][]float32) {
	tokens := make([]int32, 0, numTokens)
	allLogits := make([][]float32, 0, numTokens)
	prevTokens := append([]int32{}, inputIDs...)

	for i := 0; i < numTokens; i++ {
		// Forward pass
		lastToken := prevTokens[len(prevTokens)-1]
		input := createInputTensor([]int32{lastToken})
		logits := sg.config.DraftModel.Forward(input, sg.draftCache, startPos+i)

		// Extract logits
		logitsSlice := getLastLogits(logits)
		allLogits = append(allLogits, append([]float32{}, logitsSlice...))

		// Sample next token
		token := sg.draftSampler.Sample(logitsSlice, prevTokens)
		tokens = append(tokens, token)
		prevTokens = append(prevTokens, token)
	}

	sg.totalDrafted += numTokens
	return tokens, allLogits
}

// verify runs target model on input + draft tokens.
// Returns logits for all positions (including one extra for continuation).
func (sg *SpeculativeGenerator) verify(
	inputIDs []int32,
	draftTokens []int32,
	startPos int,
) [][]float32 {
	// Create input: [last_input_token] + draft_tokens
	allTokens := append([]int32{inputIDs[len(inputIDs)-1]}, draftTokens...)
	input := createInputTensor(allTokens)

	// Forward pass - processes all tokens in parallel
	logits := sg.config.TargetModel.Forward(input, sg.targetCache, startPos-1)

	// Extract logits for each position
	shape := logits.Shape()
	vocabSize := shape[len(shape)-1]
	seqLen := len(allTokens)
	data := logits.AsFloat32()

	result := make([][]float32, seqLen)
	for i := 0; i < seqLen; i++ {
		start := i * vocabSize
		result[i] = append([]float32{}, data[start:start+vocabSize]...)
	}

	return result
}

// accept determines how many draft tokens to accept.
// Uses modified rejection sampling: accept if r < min(1, p_target / p_draft).
// Returns: (numAccepted, resampledToken).
func (sg *SpeculativeGenerator) accept(
	draftTokens []int32,
	draftLogits [][]float32,
	targetLogits [][]float32,
) (int, int32) {
	numAccepted := 0

	for i, token := range draftTokens {
		draftProbs := softmax(draftLogits[i])
		targetProbs := softmax(targetLogits[i])

		draftProb := draftProbs[token]
		targetProb := targetProbs[token]

		// Modified rejection sampling
		r := sg.rng.Float32()
		acceptProb := minFloat32(1.0, targetProb/maxFloat32(draftProb, 1e-10))

		if r < acceptProb {
			numAccepted++
		} else {
			// Reject: resample from adjusted distribution
			resampledToken := sg.resampleRejected(draftProbs, targetProbs)
			sg.totalAccepted += numAccepted
			return numAccepted, resampledToken
		}
	}

	// All accepted - sample next from target
	lastTargetLogits := targetLogits[len(draftTokens)]
	nextToken := sg.targetSampler.Sample(lastTargetLogits, nil)
	sg.totalAccepted += numAccepted
	return numAccepted, nextToken
}

// resampleRejected samples from adjusted distribution after rejection.
// Uses: p_adjusted = max(0, p_target - p_draft) / sum(max(0, p_target - p_draft)).
func (sg *SpeculativeGenerator) resampleRejected(draftProbs, targetProbs []float32) int32 {
	// Compute adjusted probabilities: max(0, target - draft)
	adjusted := make([]float32, len(targetProbs))
	sum := float32(0)
	for i := range adjusted {
		adjusted[i] = maxFloat32(0, targetProbs[i]-draftProbs[i])
		sum += adjusted[i]
	}

	// Normalize
	if sum > 0 {
		for i := range adjusted {
			adjusted[i] /= sum
		}
		return sg.multinomial(adjusted)
	}

	// Fallback to target distribution if adjusted sum is zero
	return sg.multinomial(targetProbs)
}

// multinomial samples from a categorical distribution.
func (sg *SpeculativeGenerator) multinomial(probs []float32) int32 {
	r := sg.rng.Float32()

	cumSum := float32(0)
	for i, p := range probs {
		cumSum += p
		if r < cumSum {
			return int32(i)
		}
	}

	// Return last token if rounding errors
	//nolint:gosec // Vocab size < 2^31, safe conversion
	return int32(len(probs) - 1)
}

// minInt returns the minimum of two int values.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// minFloat32 returns the minimum of two float32 values.
func minFloat32(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

// maxFloat32 returns the maximum of two float32 values.
func maxFloat32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

// AcceptanceRate returns the current acceptance rate statistics.
func (sg *SpeculativeGenerator) AcceptanceRate() float32 {
	if sg.totalDrafted == 0 {
		return 0
	}
	return float32(sg.totalAccepted) / float32(sg.totalDrafted)
}

// Stats returns detailed generation statistics.
func (sg *SpeculativeGenerator) Stats() (drafted, accepted int, rate float32) {
	return sg.totalDrafted, sg.totalAccepted, sg.AcceptanceRate()
}
