package tokenizer

import (
	"encoding/json"
	"fmt"
)

// normalizerJSON is the raw JSON shape of a normalizer in tokenizer.json.
type normalizerJSON struct {
	Type string `json:"type"`

	// Sequence
	Normalizers []json.RawMessage `json:"normalizers,omitempty"`

	// Prepend
	PrependStr string `json:"prepend,omitempty"`

	// Replace
	Pattern *patternJSON `json:"pattern,omitempty"`
	Content string       `json:"content,omitempty"`
}

type patternJSON struct {
	String string `json:"String,omitempty"`
	Regex  string `json:"Regex,omitempty"`
}

// parseNormalizer parses a normalizer from its JSON representation.
// Returns (nil, nil) for absent or unsupported types — nil normalizer means no-op.
//
//nolint:nilnil // nil normalizer is a valid no-op; callers check for nil before calling Normalize.
func parseNormalizer(raw json.RawMessage) (Normalizer, error) {
	if raw == nil || string(raw) == "null" {
		return nil, nil
	}

	var nj normalizerJSON
	if err := json.Unmarshal(raw, &nj); err != nil {
		return nil, fmt.Errorf("normalizer: parse JSON: %w", err)
	}

	return buildNormalizer(nj)
}

//nolint:nilnil // nil = unsupported normalizer type, silently skipped by design.
func buildNormalizer(nj normalizerJSON) (Normalizer, error) {
	switch nj.Type {
	case "Sequence":
		return parseSequenceNormalizer(nj.Normalizers)

	case "Prepend":
		return &PrependNormalizer{Prepend: nj.PrependStr}, nil

	case "Replace":
		if nj.Pattern == nil || nj.Pattern.Regex != "" {
			return nil, nil
		}
		return &ReplaceNormalizer{
			Pattern: nj.Pattern.String,
			Content: nj.Content,
		}, nil

	case "Lowercase":
		return &LowercaseNormalizer{}, nil

	case "Strip", "StripAccents":
		return &StripNormalizer{}, nil

	default:
		return nil, nil
	}
}

//nolint:nilnil // empty sequence = no normalizer needed.
func parseSequenceNormalizer(items []json.RawMessage) (Normalizer, error) {
	var normalizers []Normalizer
	for _, item := range items {
		n, err := parseNormalizer(item)
		if err != nil {
			return nil, err
		}
		if n != nil {
			normalizers = append(normalizers, n)
		}
	}
	if len(normalizers) == 0 {
		return nil, nil
	}
	if len(normalizers) == 1 {
		return normalizers[0], nil
	}
	return &SequenceNormalizer{Normalizers: normalizers}, nil
}
