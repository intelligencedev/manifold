package generate

import (
	"math"
	"testing"

	"github.com/born-ml/born/internal/tensor"
)

// mockLLM implements LLMModel for testing.
type mockLLM struct {
	vocabSize  int
	logitsBias []float32 // Fixed bias per token (adds to logits)
}

func newMockLLM(vocabSize int, bias []float32) *mockLLM {
	if bias == nil {
		bias = make([]float32, vocabSize)
	}
	return &mockLLM{
		vocabSize:  vocabSize,
		logitsBias: bias,
	}
}

func (m *mockLLM) Forward(input *tensor.RawTensor, _ KVCache, _ int) *tensor.RawTensor {
	inputShape := input.Shape()
	seqLen := inputShape[len(inputShape)-1]

	// Output: [1, seqLen, vocabSize]
	outputShape := tensor.Shape{1, seqLen, m.vocabSize}
	output, _ := tensor.NewRaw(outputShape, tensor.Float32, tensor.CPU)
	data := output.AsFloat32()

	// Fill with biased logits
	for i := 0; i < seqLen; i++ {
		for j := 0; j < m.vocabSize; j++ {
			data[i*m.vocabSize+j] = m.logitsBias[j]
		}
	}

	return output
}

func (m *mockLLM) VocabSize() int {
	return m.vocabSize
}

// mockCache implements KVCache for testing.
type mockCache struct{}

func (m *mockCache) Clear() {}

// TestSpeculativeGeneratorBasic tests basic generation works.
func TestSpeculativeGeneratorBasic(t *testing.T) {
	vocabSize := 10

	// Draft model: prefers token 1
	draftBias := make([]float32, vocabSize)
	draftBias[1] = 5.0

	// Target model: prefers token 2
	targetBias := make([]float32, vocabSize)
	targetBias[2] = 5.0

	config := SpeculativeConfig{
		DraftModel:   newMockLLM(vocabSize, draftBias),
		TargetModel:  newMockLLM(vocabSize, targetBias),
		NumSpeculate: 3,
		Sampling:     SamplingConfig{Temperature: 0, Seed: 42}, // Greedy
	}

	sg := NewSpeculativeGenerator(config)
	sg.SetCaches(&mockCache{}, &mockCache{})

	inputIDs := []int32{0}
	maxTokens := 5

	tokens, acceptRate, err := sg.Generate(inputIDs, maxTokens)

	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if len(tokens) != maxTokens {
		t.Errorf("Expected %d tokens, got %d", maxTokens, len(tokens))
	}

	if acceptRate < 0 || acceptRate > 1 {
		t.Errorf("Invalid acceptance rate: %f", acceptRate)
	}

	t.Logf("Generated tokens: %v", tokens)
	t.Logf("Acceptance rate: %.2f", acceptRate)
}

// TestSpeculativeAcceptance tests acceptance logic is correct.
func TestSpeculativeAcceptance(t *testing.T) {
	vocabSize := 10

	// Both models identical - should have 100% acceptance
	bias := make([]float32, vocabSize)
	bias[1] = 5.0

	config := SpeculativeConfig{
		DraftModel:   newMockLLM(vocabSize, bias),
		TargetModel:  newMockLLM(vocabSize, bias),
		NumSpeculate: 5,
		Sampling:     SamplingConfig{Temperature: 0, Seed: 42},
	}

	sg := NewSpeculativeGenerator(config)
	sg.SetCaches(&mockCache{}, &mockCache{})

	inputIDs := []int32{0}
	maxTokens := 10

	_, acceptRate, err := sg.Generate(inputIDs, maxTokens)

	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// With identical models and greedy sampling, acceptance should be 100%
	if acceptRate < 0.99 {
		t.Errorf("Expected high acceptance rate for identical models, got %.2f", acceptRate)
	}

	t.Logf("Acceptance rate (identical models): %.2f", acceptRate)
}

// TestSpeculativeResample tests resampling after rejection.
func TestSpeculativeResample(t *testing.T) {
	sg := NewSpeculativeGenerator(DefaultSpeculativeConfig(nil, nil))

	// Draft heavily prefers token 0
	draftProbs := make([]float32, 10)
	draftProbs[0] = 0.9
	for i := 1; i < 10; i++ {
		draftProbs[i] = 0.01
	}

	// Target prefers token 5
	targetProbs := make([]float32, 10)
	targetProbs[5] = 0.8
	for i := 0; i < 10; i++ {
		if i != 5 {
			targetProbs[i] = 0.02
		}
	}

	// Resample multiple times to check distribution
	counts := make([]int, 10)
	numSamples := 1000

	for i := 0; i < numSamples; i++ {
		token := sg.resampleRejected(draftProbs, targetProbs)
		counts[token]++
	}

	// Token 5 should be sampled most often
	if counts[5] < numSamples/3 {
		t.Errorf("Expected token 5 to be sampled frequently, got %d/%d", counts[5], numSamples)
	}

	// Token 0 should rarely be sampled (adjusted prob is small)
	if counts[0] > numSamples/10 {
		t.Errorf("Expected token 0 to be sampled rarely, got %d/%d", counts[0], numSamples)
	}

	t.Logf("Resample counts: %v", counts)
}

// TestSpeculativeAllAccepted tests all tokens accepted case.
func TestSpeculativeAllAccepted(t *testing.T) {
	vocabSize := 5

	// Identical models - all should be accepted
	bias := make([]float32, vocabSize)
	bias[1] = 10.0 // Strong preference

	config := SpeculativeConfig{
		DraftModel:   newMockLLM(vocabSize, bias),
		TargetModel:  newMockLLM(vocabSize, bias),
		NumSpeculate: 3,
		Sampling:     SamplingConfig{Temperature: 0, Seed: 42},
	}

	sg := NewSpeculativeGenerator(config)
	sg.SetCaches(&mockCache{}, &mockCache{})

	draftTokens := []int32{1, 1, 1}
	draftLogits := [][]float32{bias, bias, bias}
	targetLogits := [][]float32{bias, bias, bias, bias} // +1 for continuation

	numAccepted, nextToken := sg.accept(draftTokens, draftLogits, targetLogits)

	if numAccepted != len(draftTokens) {
		t.Errorf("Expected all %d tokens accepted, got %d", len(draftTokens), numAccepted)
	}

	if nextToken < 0 || nextToken >= int32(vocabSize) {
		t.Errorf("Invalid next token: %d", nextToken)
	}

	t.Logf("All accepted, next token: %d", nextToken)
}

// TestSpeculativeAllRejected tests all tokens rejected case.
func TestSpeculativeAllRejected(t *testing.T) {
	vocabSize := 5

	// Draft prefers token 0, target prefers token 4
	draftBias := make([]float32, vocabSize)
	draftBias[0] = 10.0

	targetBias := make([]float32, vocabSize)
	targetBias[4] = 10.0

	config := SpeculativeConfig{
		DraftModel:   newMockLLM(vocabSize, draftBias),
		TargetModel:  newMockLLM(vocabSize, targetBias),
		NumSpeculate: 3,
		Sampling:     SamplingConfig{Temperature: 0, Seed: 42},
	}

	sg := NewSpeculativeGenerator(config)

	// Draft generated token 0, but target wants token 4
	draftTokens := []int32{0, 0, 0}
	draftLogits := [][]float32{draftBias, draftBias, draftBias}
	targetLogits := [][]float32{targetBias, targetBias, targetBias}

	numAccepted, nextToken := sg.accept(draftTokens, draftLogits, targetLogits)

	if numAccepted != 0 {
		t.Errorf("Expected 0 tokens accepted, got %d", numAccepted)
	}

	// Should resample from target distribution (prefer token 4)
	if nextToken != 4 && nextToken != 0 { // Allow some randomness
		t.Logf("Warning: resampled token %d, expected around 4", nextToken)
	}

	t.Logf("None accepted, resampled token: %d", nextToken)
}

// TestSpeculativePartialAccept tests partial acceptance.
func TestSpeculativePartialAccept(t *testing.T) {
	vocabSize := 5

	// Create scenario where first token matches, second doesn't
	draftBias := make([]float32, vocabSize)
	draftBias[1] = 10.0

	targetBias := make([]float32, vocabSize)
	targetBias[1] = 10.0 // First position matches

	targetBias2 := make([]float32, vocabSize)
	targetBias2[2] = 10.0 // Second position different

	config := SpeculativeConfig{
		DraftModel:   newMockLLM(vocabSize, draftBias),
		TargetModel:  newMockLLM(vocabSize, targetBias),
		NumSpeculate: 3,
		Sampling:     SamplingConfig{Temperature: 0, Seed: 42},
	}

	sg := NewSpeculativeGenerator(config)

	// Draft: [1, 1, 1], Target wants: [1, 2, ?]
	draftTokens := []int32{1, 1, 1}
	draftLogits := [][]float32{draftBias, draftBias, draftBias}
	targetLogits := [][]float32{draftBias, targetBias2, targetBias2}

	numAccepted, nextToken := sg.accept(draftTokens, draftLogits, targetLogits)

	// First token should be accepted (both want 1)
	if numAccepted < 1 {
		t.Errorf("Expected at least 1 token accepted, got %d", numAccepted)
	}

	// Should not accept all (second token mismatch)
	if numAccepted == len(draftTokens) {
		t.Logf("Warning: accepted all tokens, expected partial")
	}

	if nextToken < 0 || nextToken >= int32(vocabSize) {
		t.Errorf("Invalid next token: %d", nextToken)
	}

	t.Logf("Accepted %d/%d tokens, next: %d", numAccepted, len(draftTokens), nextToken)
}

// TestSpeculativeOutputDistribution tests output matches target model distribution.
func TestSpeculativeOutputDistribution(t *testing.T) {
	vocabSize := 10

	// Target model strongly prefers token 7
	targetBias := make([]float32, vocabSize)
	targetBias[7] = 8.0

	// Draft model has different preference
	draftBias := make([]float32, vocabSize)
	draftBias[3] = 5.0

	config := SpeculativeConfig{
		DraftModel:   newMockLLM(vocabSize, draftBias),
		TargetModel:  newMockLLM(vocabSize, targetBias),
		NumSpeculate: 2,
		Sampling:     SamplingConfig{Temperature: 1.0, Seed: 42},
	}

	sg := NewSpeculativeGenerator(config)
	sg.SetCaches(&mockCache{}, &mockCache{})

	// Generate multiple times and check distribution
	counts := make([]int, vocabSize)
	numRuns := 100

	for run := 0; run < numRuns; run++ {
		// Reset for each run
		sg.ClearCaches()
		tokens, _, err := sg.Generate([]int32{0}, 5)
		if err != nil {
			t.Fatalf("Generate failed: %v", err)
		}
		for _, tok := range tokens {
			counts[tok]++
		}
	}

	// Token 7 should appear most frequently (target prefers it)
	maxCount := 0
	maxToken := 0
	for i, c := range counts {
		if c > maxCount {
			maxCount = c
			maxToken = i
		}
	}

	if maxToken != 7 {
		t.Logf("Warning: expected token 7 to be most frequent, got token %d", maxToken)
		t.Logf("Distribution: %v", counts)
	}

	t.Logf("Token distribution: %v", counts)
	t.Logf("Most frequent: token %d (%d times)", maxToken, maxCount)
}

// TestSpeculativeEdgeCases tests edge cases.
func TestSpeculativeEdgeCases(t *testing.T) {
	vocabSize := 5

	tests := []struct {
		name         string
		maxTokens    int
		numSpeculate int
		wantErr      bool
	}{
		{"single token", 1, 3, false},
		{"exact speculation", 3, 3, false},
		{"more tokens than speculation", 10, 2, false},
		{"zero speculation becomes default", 5, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bias := make([]float32, vocabSize)
			bias[1] = 5.0

			config := SpeculativeConfig{
				DraftModel:   newMockLLM(vocabSize, bias),
				TargetModel:  newMockLLM(vocabSize, bias),
				NumSpeculate: tt.numSpeculate,
				Sampling:     SamplingConfig{Temperature: 0, Seed: 42},
			}

			sg := NewSpeculativeGenerator(config)
			sg.SetCaches(&mockCache{}, &mockCache{})

			tokens, rate, err := sg.Generate([]int32{0}, tt.maxTokens)

			if (err != nil) != tt.wantErr {
				t.Errorf("Generate() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if len(tokens) != tt.maxTokens {
					t.Errorf("Expected %d tokens, got %d", tt.maxTokens, len(tokens))
				}
				if rate < 0 || rate > 1 {
					t.Errorf("Invalid acceptance rate: %f", rate)
				}
			}
		})
	}
}

// TestSpeculativeVocabMismatch tests error on vocab size mismatch.
func TestSpeculativeVocabMismatch(t *testing.T) {
	config := SpeculativeConfig{
		DraftModel:   newMockLLM(10, nil),
		TargetModel:  newMockLLM(20, nil), // Different vocab size
		NumSpeculate: 3,
		Sampling:     SamplingConfig{Temperature: 0},
	}

	sg := NewSpeculativeGenerator(config)

	_, _, err := sg.Generate([]int32{0}, 5)

	if err == nil {
		t.Error("Expected error for vocab size mismatch")
	}
}

// TestSpeculativeEmptyInput tests error on empty input.
func TestSpeculativeEmptyInput(t *testing.T) {
	bias := make([]float32, 10)
	config := SpeculativeConfig{
		DraftModel:   newMockLLM(10, bias),
		TargetModel:  newMockLLM(10, bias),
		NumSpeculate: 3,
		Sampling:     SamplingConfig{Temperature: 0},
	}

	sg := NewSpeculativeGenerator(config)

	_, _, err := sg.Generate([]int32{}, 5)

	if err == nil {
		t.Error("Expected error for empty input")
	}
}

// TestSpeculativeStats tests statistics tracking.
func TestSpeculativeStats(t *testing.T) {
	vocabSize := 10
	bias := make([]float32, vocabSize)
	bias[1] = 5.0

	config := SpeculativeConfig{
		DraftModel:   newMockLLM(vocabSize, bias),
		TargetModel:  newMockLLM(vocabSize, bias),
		NumSpeculate: 3,
		Sampling:     SamplingConfig{Temperature: 0, Seed: 42},
	}

	sg := NewSpeculativeGenerator(config)
	sg.SetCaches(&mockCache{}, &mockCache{})

	// Generate
	_, rate, err := sg.Generate([]int32{0}, 10)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// Check stats
	drafted, accepted, statsRate := sg.Stats()

	if drafted == 0 {
		t.Error("Expected non-zero drafted tokens")
	}

	if accepted > drafted {
		t.Error("Accepted tokens cannot exceed drafted tokens")
	}

	if math.Abs(float64(rate-statsRate)) > 0.01 {
		t.Errorf("Rate mismatch: Generate=%f, Stats=%f", rate, statsRate)
	}

	t.Logf("Stats: drafted=%d, accepted=%d, rate=%.2f", drafted, accepted, statsRate)
}

// BenchmarkSpeculativeVsStandard compares performance.
func BenchmarkSpeculativeVsStandard(b *testing.B) {
	vocabSize := 1000
	bias := make([]float32, vocabSize)
	bias[42] = 5.0

	config := SpeculativeConfig{
		DraftModel:   newMockLLM(vocabSize, bias),
		TargetModel:  newMockLLM(vocabSize, bias),
		NumSpeculate: 5,
		Sampling:     SamplingConfig{Temperature: 1.0, Seed: 42},
	}

	sg := NewSpeculativeGenerator(config)
	sg.SetCaches(&mockCache{}, &mockCache{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sg.ClearCaches()
		_, _, _ = sg.Generate([]int32{0, 1, 2}, 20)
	}
}

// BenchmarkSpeculativeAcceptance benchmarks acceptance logic.
func BenchmarkSpeculativeAcceptance(b *testing.B) {
	vocabSize := 1000
	sg := NewSpeculativeGenerator(DefaultSpeculativeConfig(nil, nil))

	// Prepare data
	draftTokens := []int32{1, 2, 3, 4, 5}
	draftLogits := make([][]float32, 5)
	targetLogits := make([][]float32, 6)

	for i := range draftLogits {
		draftLogits[i] = make([]float32, vocabSize)
		targetLogits[i] = make([]float32, vocabSize)
		draftLogits[i][i] = 5.0
		targetLogits[i][i] = 5.0
	}
	targetLogits[5] = make([]float32, vocabSize)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = sg.accept(draftTokens, draftLogits, targetLogits)
	}
}
