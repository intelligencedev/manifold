package tokenizer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// HFTokenizerType identifies the tokenizer implementation type.
type HFTokenizerType string

const (
	// HFTypeBPE indicates Byte-Pair Encoding tokenizer.
	HFTypeBPE HFTokenizerType = "BPE"

	// HFTypeWordPiece indicates WordPiece tokenizer (BERT-style).
	HFTypeWordPiece HFTokenizerType = "WordPiece"

	// HFTypeUnigram indicates Unigram tokenizer (SentencePiece-style).
	HFTypeUnigram HFTokenizerType = "Unigram"

	// HFTypeUnknown indicates an unknown or unsupported tokenizer type.
	HFTypeUnknown HFTokenizerType = "Unknown"
)

// Special token strings used in HuggingFace tokenizer.json files.
const (
	specialTokenBOS    = "<s>"
	specialTokenAltBOS = "<bos>"
	specialTokenCLS    = "[CLS]"
	specialTokenEOS    = "</s>"
	specialTokenAltEOS = "<eos>"
	specialTokenSEP    = "[SEP]"
	modelGPT35Turbo    = "gpt-3.5-turbo"
	modelGPT4          = "gpt-4"
)

// HFTokenizerMetadata contains metadata from tokenizer.json.
type HFTokenizerMetadata struct {
	Type          HFTokenizerType
	VocabSize     int
	HasBOS        bool
	HasEOS        bool
	HasPAD        bool
	HasUNK        bool
	ModelName     string
	TokenizerType string
}

// DetectHFTokenizerType determines the tokenizer type from tokenizer.json.
//
//nolint:gocognit,gocyclo,cyclop // JSON parsing requires nested type assertions for complex structures.
func DetectHFTokenizerType(path string) (*HFTokenizerMetadata, error) {
	//nolint:gosec // Loading tokenizer from user-specified path is intentional.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read tokenizer.json: %w", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse tokenizer.json: %w", err)
	}

	metadata := &HFTokenizerMetadata{
		Type: HFTypeUnknown,
	}

	// Check model type.
	if model, ok := raw["model"].(map[string]interface{}); ok {
		if tokType, ok := model["type"].(string); ok {
			metadata.TokenizerType = tokType
			switch tokType {
			case string(HFTypeBPE):
				metadata.Type = HFTypeBPE
			case string(HFTypeWordPiece):
				metadata.Type = HFTypeWordPiece
			case "Unigram":
				metadata.Type = HFTypeUnigram
			}
		}

		// Get vocab size.
		if vocab, ok := model["vocab"].(map[string]interface{}); ok {
			metadata.VocabSize = len(vocab)
		}
	}

	// Check for special tokens.
	if addedTokens, ok := raw["added_tokens"].([]interface{}); ok {
		for _, tokenRaw := range addedTokens {
			if token, ok := tokenRaw.(map[string]interface{}); ok {
				if content, ok := token["content"].(string); ok {
					switch content {
					case specialTokenBOS, specialTokenAltBOS, specialTokenCLS:
						metadata.HasBOS = true
					case specialTokenEOS, specialTokenAltEOS, specialTokenSEP:
						metadata.HasEOS = true
					case "<pad>", "[PAD]":
						metadata.HasPAD = true
					case "<unk>", "[UNK]":
						metadata.HasUNK = true
					}
				}
			}
		}
	}

	return metadata, nil
}

// LoadFromHuggingFace loads a tokenizer from a HuggingFace model directory.
//
// The directory should contain tokenizer.json and optionally tokenizer_config.json.
func LoadFromHuggingFace(modelPath string) (Tokenizer, error) {
	tokenizerPath := filepath.Join(modelPath, "tokenizer.json")

	// Detect tokenizer type.
	metadata, err := DetectHFTokenizerType(tokenizerPath)
	if err != nil {
		return nil, err
	}

	// Load based on type.
	switch metadata.Type {
	case HFTypeBPE:
		return LoadBPEFromHuggingFace(tokenizerPath)
	case HFTypeWordPiece:
		return nil, fmt.Errorf("WordPiece tokenizer not yet implemented")
	case HFTypeUnigram:
		return nil, fmt.Errorf("unigram tokenizer not yet implemented (requires SentencePiece)")
	default:
		return nil, fmt.Errorf("unknown tokenizer type: %s", metadata.TokenizerType)
	}
}

// TryLoadTikToken attempts to load a tiktoken-compatible tokenizer.
//
// This is a fallback for models that use OpenAI-style tokenizers.
func TryLoadTikToken(modelName string) (Tokenizer, error) {
	// Map common model names to tiktoken encodings.
	encodingMap := map[string]string{
		modelGPT4:                encodingCL100kBase,
		modelGPT35Turbo:          encodingCL100kBase,
		"gpt-3":                  encodingP50kBase,
		"text-davinci-003":       encodingP50kBase,
		"text-embedding-ada-002": encodingCL100kBase,
	}

	if encoding, ok := encodingMap[modelName]; ok {
		return NewTikToken(encoding)
	}

	// Try to use the model name directly.
	return NewTikTokenForModel(modelName)
}

// AutoLoadTokenizer attempts to automatically load the correct tokenizer.
//
// It tries multiple strategies:
//  1. Load from HuggingFace model directory (tokenizer.json)
//  2. Load tiktoken by model name
//  3. Load tiktoken by encoding name
func AutoLoadTokenizer(pathOrName string) (Tokenizer, error) {
	// Strategy 1: Try as HuggingFace model directory.
	if info, err := os.Stat(pathOrName); err == nil && info.IsDir() {
		tokenizerPath := filepath.Join(pathOrName, "tokenizer.json")
		if _, err := os.Stat(tokenizerPath); err == nil {
			tokenizer, err := LoadFromHuggingFace(pathOrName)
			if err == nil {
				return tokenizer, nil
			}
		}
	}

	// Strategy 2: Try as tiktoken model name.
	if tokenizer, err := TryLoadTikToken(pathOrName); err == nil {
		return tokenizer, nil
	}

	// Strategy 3: Try as tiktoken encoding name.
	if tokenizer, err := NewTikToken(pathOrName); err == nil {
		return tokenizer, nil
	}

	return nil, fmt.Errorf("failed to auto-load tokenizer from %q", pathOrName)
}
