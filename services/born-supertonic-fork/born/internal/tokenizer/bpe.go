package tokenizer

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// BPETokenizer implements Byte-Pair Encoding tokenization.
//
// This is a pure Go implementation that can load HuggingFace tokenizer.json files.
type BPETokenizer struct {
	vocab         map[string]int32 // token -> ID
	merges        []pair           // BPE merge rules
	reverseVocab  map[int32]string // ID -> token
	normalizer    Normalizer       // text normalizer (from tokenizer.json)
	bosToken      int32
	eosToken      int32
	padToken      int32
	unkToken      int32
	specialTokens map[int32]bool
}

type pair struct {
	first  string
	second string
}

// NewBPETokenizer creates a new BPE tokenizer from vocab and merges.
func NewBPETokenizer(vocab map[string]int32, merges []pair) *BPETokenizer {
	reverseVocab := make(map[int32]string, len(vocab))
	for token, id := range vocab {
		reverseVocab[id] = token
	}

	return &BPETokenizer{
		vocab:         vocab,
		merges:        merges,
		reverseVocab:  reverseVocab,
		bosToken:      -1,
		eosToken:      -1,
		padToken:      -1,
		unkToken:      -1,
		specialTokens: make(map[int32]bool),
	}
}

// SetSpecialTokens configures special token IDs.
func (b *BPETokenizer) SetSpecialTokens(bos, eos, pad, unk int32) {
	b.bosToken = bos
	b.eosToken = eos
	b.padToken = pad
	b.unkToken = unk

	if bos >= 0 {
		b.specialTokens[bos] = true
	}
	if eos >= 0 {
		b.specialTokens[eos] = true
	}
	if pad >= 0 {
		b.specialTokens[pad] = true
	}
	if unk >= 0 {
		b.specialTokens[unk] = true
	}
}

// Encode converts text to token IDs using BPE.
//
//nolint:gocognit // BPE encoding requires nested loops for merge operations across multiple words.
func (b *BPETokenizer) Encode(text string) ([]int32, error) {
	if text == "" {
		return []int32{}, nil
	}

	if b.normalizer != nil {
		text = b.normalizer.Normalize(text)
	}

	// Split text into words.
	// After SentencePiece normalization, spaces become '▁' (U+2581).
	// Split on '▁' boundaries, keeping '▁' at the start of each word.
	words := splitWords(text)
	var tokens []int32

	for _, word := range words {
		// Convert word to byte-level representation.
		chars := []string{}
		for _, r := range word {
			chars = append(chars, string(r))
		}

		// Apply BPE merges.
		for len(chars) > 1 {
			// Find the best pair to merge.
			bestPair := pair{}
			bestIdx := -1
			bestRank := len(b.merges) + 1

			for i := 0; i < len(chars)-1; i++ {
				p := pair{chars[i], chars[i+1]}
				rank := b.getMergeRank(p)
				if rank < bestRank {
					bestPair = p
					bestIdx = i
					bestRank = rank
				}
			}

			if bestIdx == -1 {
				break
			}

			// Merge the pair.
			merged := bestPair.first + bestPair.second
			newChars := []string{}
			for i := 0; i < len(chars); i++ {
				if i == bestIdx {
					newChars = append(newChars, merged)
					i++ // Skip next char (it's merged).
				} else {
					newChars = append(newChars, chars[i])
				}
			}
			chars = newChars
		}

		// Convert chars to token IDs.
		for _, char := range chars {
			if id, ok := b.vocab[char]; ok {
				tokens = append(tokens, id)
			} else if b.unkToken >= 0 {
				tokens = append(tokens, b.unkToken)
			}
		}
	}

	return tokens, nil
}

// getMergeRank returns the rank of a merge pair (lower is higher priority).
func (b *BPETokenizer) getMergeRank(p pair) int {
	for i, merge := range b.merges {
		if merge.first == p.first && merge.second == p.second {
			return i
		}
	}
	return len(b.merges) + 1
}

// Decode converts token IDs back to text.
func (b *BPETokenizer) Decode(tokens []int32) (string, error) {
	var parts []string

	for _, token := range tokens {
		if text, ok := b.reverseVocab[token]; ok {
			parts = append(parts, text)
		} else {
			// Unknown token, use replacement.
			parts = append(parts, "�")
		}
	}

	return strings.Join(parts, ""), nil
}

// VocabSize returns the total vocabulary size.
func (b *BPETokenizer) VocabSize() int {
	return len(b.vocab)
}

// BosToken returns the beginning-of-sequence token ID.
func (b *BPETokenizer) BosToken() int32 {
	return b.bosToken
}

// EosToken returns the end-of-sequence token ID.
func (b *BPETokenizer) EosToken() int32 {
	return b.eosToken
}

// PadToken returns the padding token ID.
func (b *BPETokenizer) PadToken() int32 {
	return b.padToken
}

// UnkToken returns the unknown token ID.
func (b *BPETokenizer) UnkToken() int32 {
	return b.unkToken
}

// IsSpecialToken checks if a token ID is a special token.
func (b *BPETokenizer) IsSpecialToken(token int32) bool {
	return b.specialTokens[token]
}

// HuggingFaceTokenizerConfig represents the tokenizer.json structure.
type HuggingFaceTokenizerConfig struct {
	Model struct {
		Vocab  map[string]int `json:"vocab"`
		Merges []string       `json:"merges"`
	} `json:"model"`
	AddedTokens []struct {
		ID      int    `json:"id"`
		Content string `json:"content"`
		Special bool   `json:"special"`
	} `json:"added_tokens"`
	Normalizer json.RawMessage `json:"normalizer"`
}

// LoadBPEFromHuggingFace loads a BPE tokenizer from tokenizer.json.
//
// This is a simplified loader that handles the most common HuggingFace format.
func LoadBPEFromHuggingFace(path string) (*BPETokenizer, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: Path comes from trusted caller
	if err != nil {
		return nil, fmt.Errorf("failed to read tokenizer.json: %w", err)
	}

	var config HuggingFaceTokenizerConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse tokenizer.json: %w", err)
	}

	// Build vocab.
	vocab := make(map[string]int32, len(config.Model.Vocab))
	for token, id := range config.Model.Vocab {
		vocab[token] = int32(id) //nolint:gosec // G115: integer overflow conversion int -> int32
	}

	// Parse merges.
	var merges []pair
	for _, mergeStr := range config.Model.Merges {
		parts := strings.Fields(mergeStr)
		if len(parts) == 2 {
			merges = append(merges, pair{parts[0], parts[1]})
		}
	}

	tokenizer := NewBPETokenizer(vocab, merges)

	// Parse normalizer (Prepend, Replace, Sequence, etc.).
	if len(config.Normalizer) > 0 {
		norm, err := parseNormalizer(config.Normalizer)
		if err != nil {
			return nil, fmt.Errorf("failed to parse normalizer: %w", err)
		}
		tokenizer.normalizer = norm
	}

	configureSpecialTokens(tokenizer, config.AddedTokens)
	return tokenizer, nil
}

// configureSpecialTokens identifies BOS/EOS/PAD/UNK from the added_tokens list.
func configureSpecialTokens(tokenizer *BPETokenizer, addedTokens []struct {
	ID      int    `json:"id"`
	Content string `json:"content"`
	Special bool   `json:"special"`
}) {
	for _, addedToken := range addedTokens {
		id := int32(addedToken.ID) //nolint:gosec // G115: integer overflow conversion int -> int32
		if addedToken.Special {
			tokenizer.specialTokens[id] = true

			content := strings.ToLower(addedToken.Content)
			switch {
			case strings.Contains(content, "bos") || content == specialTokenBOS:
				tokenizer.bosToken = id
			case strings.Contains(content, "eos") || content == specialTokenEOS:
				tokenizer.eosToken = id
			case strings.Contains(content, "pad"):
				tokenizer.padToken = id
			case strings.Contains(content, "unk"):
				tokenizer.unkToken = id
			}
		}
	}
}

// splitWords splits text into words for BPE tokenization.
//
// For SentencePiece-normalized text (spaces replaced with '▁'), splits on '▁'
// boundaries while keeping '▁' at the start of each word.
// For regular text, falls back to whitespace splitting.
func splitWords(text string) []string {
	const sentencePieceSpace = '▁' // U+2581

	if strings.ContainsRune(text, sentencePieceSpace) {
		parts := strings.Split(text, string(sentencePieceSpace))
		var words []string
		for i, part := range parts {
			if part == "" {
				continue
			}
			if i > 0 {
				words = append(words, string(sentencePieceSpace)+part)
			} else {
				words = append(words, part)
			}
		}
		return words
	}

	return strings.Fields(text)
}

// ExampleBPEVocab creates a minimal BPE tokenizer for testing.
func ExampleBPEVocab() *BPETokenizer {
	// Minimal vocab for demonstration.
	vocab := map[string]int32{
		"h":   0,
		"e":   1,
		"l":   2,
		"o":   3,
		"w":   4,
		"r":   5,
		"d":   6,
		" ":   7,
		"he":  8,
		"ll":  9,
		"o ":  10,
		"wor": 11,
		"ld":  12,
	}

	merges := []pair{
		{"h", "e"},
		{"l", "l"},
		{"o", " "},
		{"w", "o"},
		{"l", "d"},
	}

	tokenizer := NewBPETokenizer(vocab, merges)
	tokenizer.SetSpecialTokens(-1, -1, -1, -1)

	return tokenizer
}
