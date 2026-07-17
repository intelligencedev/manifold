package generate

import (
	"fmt"
	"strings"

	"github.com/born-ml/born/internal/tensor"
	"github.com/born-ml/born/internal/tokenizer"
)

// GenerateConfig configures text generation.
//
//nolint:revive // GenerateConfig is clearer than Config
type GenerateConfig struct {
	// MaxTokens is the maximum number of tokens to generate.
	MaxTokens int

	// MinTokens is the minimum number of tokens before stopping.
	MinTokens int

	// StopStrings are strings that trigger stopping.
	StopStrings []string

	// StopTokens are token IDs that trigger stopping.
	StopTokens []int32

	// Stream enables streaming generation.
	Stream bool

	// EchoPrompt includes the prompt in output.
	EchoPrompt bool

	// Sampling is the sampling configuration.
	Sampling SamplingConfig
}

// DefaultGenerateConfig returns sensible defaults for generation.
func DefaultGenerateConfig() GenerateConfig {
	return GenerateConfig{
		MaxTokens:   256,
		MinTokens:   0,
		StopStrings: nil,
		StopTokens:  nil,
		Stream:      false,
		EchoPrompt:  false,
		Sampling:    DefaultSamplingConfig(),
	}
}

// GenerateResult is a single result from streaming generation.
//
//nolint:revive // GenerateResult is clearer than Result
type GenerateResult struct {
	Token   string // Decoded token text
	TokenID int32  // Token ID
	Done    bool   // Is generation complete
	Reason  string // Stop reason: "eos", "max_tokens", "stop_string", "stop_token"
	Error   error  // Error if any
}

// KVCache is an interface for key-value caches used in generation.
type KVCache interface {
	Clear()
}

// LLMModel is the interface for language models used in generation.
type LLMModel interface {
	// Forward runs a forward pass and returns logits.
	// Input shape: [batch, seq_len]
	// Output shape: [batch, seq_len, vocab_size]
	Forward(input *tensor.RawTensor, cache KVCache, startPos int) *tensor.RawTensor

	// VocabSize returns the vocabulary size.
	VocabSize() int
}

// TextGenerator generates text using an LLM.
type TextGenerator struct {
	model     LLMModel
	tokenizer tokenizer.Tokenizer
	sampler   *Sampler
	cache     KVCache

	// Config
	maxSeqLen int
}

// GeneratorOption configures a TextGenerator.
type GeneratorOption func(*generatorOptions)

type generatorOptions struct {
	maxSeqLen int
}

// WithMaxSeqLen sets the maximum sequence length.
func WithMaxSeqLen(n int) GeneratorOption {
	return func(o *generatorOptions) {
		o.maxSeqLen = n
	}
}

// NewTextGenerator creates a new text generator.
func NewTextGenerator(
	model LLMModel,
	tok tokenizer.Tokenizer,
	samplingConfig SamplingConfig,
	opts ...GeneratorOption,
) *TextGenerator {
	options := &generatorOptions{
		maxSeqLen: 2048,
	}
	for _, opt := range opts {
		opt(options)
	}

	return &TextGenerator{
		model:     model,
		tokenizer: tok,
		sampler:   NewSampler(samplingConfig),
		cache:     nil,
		maxSeqLen: options.maxSeqLen,
	}
}

// SetCache sets the KV cache for generation.
func (g *TextGenerator) SetCache(cache KVCache) {
	g.cache = cache
}

// ClearCache clears the KV cache.
func (g *TextGenerator) ClearCache() {
	if g.cache != nil {
		g.cache.Clear()
	}
}

// Generate generates text from a prompt.
func (g *TextGenerator) Generate(prompt string, config GenerateConfig) (string, error) {
	inputIDs, err := g.tokenizer.Encode(prompt)
	if err != nil {
		return "", fmt.Errorf("encode prompt: %w", err)
	}

	var result strings.Builder
	if config.EchoPrompt {
		result.WriteString(prompt)
	}

	err = g.generate(inputIDs, config, func(res GenerateResult) bool {
		if res.Error != nil {
			return false
		}
		result.WriteString(res.Token)
		return !res.Done
	})

	if err != nil {
		return "", err
	}

	return result.String(), nil
}

// GenerateStream generates text and returns a channel of results.
func (g *TextGenerator) GenerateStream(prompt string, config GenerateConfig) (<-chan GenerateResult, error) {
	inputIDs, err := g.tokenizer.Encode(prompt)
	if err != nil {
		return nil, fmt.Errorf("encode prompt: %w", err)
	}

	ch := make(chan GenerateResult, 1)

	go func() {
		defer close(ch)

		if config.EchoPrompt {
			// Send prompt tokens first
			promptText, _ := g.tokenizer.Decode(inputIDs)
			ch <- GenerateResult{Token: promptText}
		}

		_ = g.generate(inputIDs, config, func(res GenerateResult) bool {
			ch <- res
			return !res.Done && res.Error == nil
		})
	}()

	return ch, nil
}

// Chat generates a response for chat messages.
// Uses the provided ChatTemplate to format messages into a prompt.
func (g *TextGenerator) Chat(messages []tokenizer.ChatMessage, template tokenizer.ChatTemplate, config GenerateConfig) (string, error) {
	prompt := template.Apply(messages)
	return g.Generate(prompt, config)
}

// ChatStream generates a streaming response for chat messages.
// Uses the provided ChatTemplate to format messages into a prompt.
func (g *TextGenerator) ChatStream(messages []tokenizer.ChatMessage, template tokenizer.ChatTemplate, config GenerateConfig) (<-chan GenerateResult, error) {
	prompt := template.Apply(messages)
	return g.GenerateStream(prompt, config)
}

// generate is the core generation loop.
func (g *TextGenerator) generate(
	inputIDs []int32,
	config GenerateConfig,
	callback func(GenerateResult) bool,
) error {
	// Validate input
	if len(inputIDs) == 0 {
		return fmt.Errorf("empty input")
	}

	if len(inputIDs) >= g.maxSeqLen {
		return fmt.Errorf("input too long: %d >= %d", len(inputIDs), g.maxSeqLen)
	}

	// Convert input to tensor
	promptLen := len(inputIDs)
	inputTensor := createInputTensor(inputIDs)

	// Prefill: process entire prompt
	var logits *tensor.RawTensor
	if g.model != nil {
		logits = g.model.Forward(inputTensor, g.cache, 0)
	}

	// Decode: generate tokens one by one
	generated := make([]int32, 0, config.MaxTokens)
	prevTokens := append([]int32{}, inputIDs...) // For repetition penalty

	for i := 0; i < config.MaxTokens; i++ {
		// Sample next token
		var nextToken int32
		if logits != nil {
			// Get logits for last position
			lastLogits := getLastLogits(logits)
			nextToken = g.sampler.Sample(lastLogits, prevTokens)
		} else {
			// No model - just return EOS (for testing)
			nextToken = g.tokenizer.EosToken()
		}

		generated = append(generated, nextToken)
		prevTokens = append(prevTokens, nextToken)

		// Decode token
		tokenStr, _ := g.tokenizer.Decode([]int32{nextToken})

		// Check stop conditions
		done, reason := g.checkStopConditions(nextToken, generated, config)

		// Callback
		shouldContinue := callback(GenerateResult{
			Token:   tokenStr,
			TokenID: nextToken,
			Done:    done,
			Reason:  reason,
		})

		if done || !shouldContinue {
			break
		}

		// Prepare for next iteration
		if g.model != nil {
			nextInput := createInputTensor([]int32{nextToken})
			logits = g.model.Forward(nextInput, g.cache, promptLen+i)
		}
	}

	return nil
}

// checkStopConditions checks if generation should stop.
func (g *TextGenerator) checkStopConditions(
	token int32,
	generated []int32,
	config GenerateConfig,
) (bool, string) {
	// Check minimum tokens
	if len(generated) < config.MinTokens {
		return false, ""
	}

	// Check EOS token
	if token == g.tokenizer.EosToken() {
		return true, "eos"
	}

	// Check stop tokens
	for _, stopToken := range config.StopTokens {
		if token == stopToken {
			return true, "stop_token"
		}
	}

	// Check stop strings
	if len(config.StopStrings) > 0 {
		fullText, _ := g.tokenizer.Decode(generated)
		for _, stopStr := range config.StopStrings {
			if strings.HasSuffix(fullText, stopStr) {
				return true, "stop_string"
			}
		}
	}

	// Check max tokens
	if len(generated) >= config.MaxTokens {
		return true, "max_tokens"
	}

	return false, ""
}

// createInputTensor creates a tensor from token IDs.
func createInputTensor(tokens []int32) *tensor.RawTensor {
	// Create [1, seq_len] tensor
	shape := tensor.Shape{1, len(tokens)}
	raw, _ := tensor.NewRaw(shape, tensor.Int32, tensor.CPU)

	// Copy data
	data := raw.AsInt32()
	copy(data, tokens)

	return raw
}

// getLastLogits extracts logits for the last position.
func getLastLogits(logits *tensor.RawTensor) []float32 {
	shape := logits.Shape()
	if len(shape) == 3 {
		// [batch, seq_len, vocab_size]
		vocabSize := shape[2]
		seqLen := shape[1]
		data := logits.AsFloat32()
		// Get last position for batch 0
		start := (seqLen - 1) * vocabSize
		return data[start : start+vocabSize]
	} else if len(shape) == 2 {
		// [seq_len, vocab_size]
		vocabSize := shape[1]
		seqLen := shape[0]
		data := logits.AsFloat32()
		start := (seqLen - 1) * vocabSize
		return data[start : start+vocabSize]
	}
	// Assume [vocab_size]
	return logits.AsFloat32()
}
