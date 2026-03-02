package llm

import (
	"encoding/json"
	"strconv"
	"strings"
)

var extraParamIntKeys = map[string]struct{}{
	"maxtokens":       {},
	"maxoutputtokens": {},
	"topk":            {},
	"candidatecount":  {},
	"logprobs":        {},
}

var extraParamFloatKeys = map[string]struct{}{
	"temperature":      {},
	"topp":             {},
	"presencepenalty":  {},
	"frequencypenalty": {},
}

// NormalizeExtraParams returns a deep-copied map with common numeric fields
// coerced to the expected number types (e.g., max_tokens -> int).
// This helps when params originate from JSONB/UI fields where numbers may
// be encoded as strings.
func NormalizeExtraParams(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	return normalizeExtraMap(in)
}

// PopIntExtraParam removes the first matching key and returns its integer value.
// It returns (value, found, ok) where ok indicates successful conversion.
func PopIntExtraParam(in map[string]any, keys ...string) (int64, bool, bool) {
	if len(in) == 0 || len(keys) == 0 {
		return 0, false, false
	}
	keyset := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		if nk := normalizeExtraKey(k); nk != "" {
			keyset[nk] = struct{}{}
		}
	}
	for k, v := range in {
		if _, ok := keyset[normalizeExtraKey(k)]; !ok {
			continue
		}
		delete(in, k)
		if n, ok := toInt64(v); ok {
			return n, true, true
		}
		return 0, true, false
	}
	return 0, false, false
}

func normalizeExtraMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = normalizeExtraValue(k, v)
	}
	return out
}

func normalizeExtraValue(key string, v any) any {
	switch tv := v.(type) {
	case map[string]any:
		return normalizeExtraMap(tv)
	case []any:
		out := make([]any, len(tv))
		for i, elem := range tv {
			out[i] = normalizeExtraValue("", elem)
		}
		return out
	case string:
		if decoded, ok := decodeStructuredJSON(tv); ok {
			return normalizeExtraValue(key, decoded)
		}
		return coerceExtraValue(key, tv)
	default:
		return coerceExtraValue(key, v)
	}
}

func decodeStructuredJSON(raw string) (any, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil, false
	}
	if !strings.HasPrefix(s, "{") && !strings.HasPrefix(s, "[") {
		return nil, false
	}

	var decoded any
	if err := json.Unmarshal([]byte(s), &decoded); err != nil {
		return nil, false
	}
	switch decoded.(type) {
	case map[string]any, []any:
		return decoded, true
	default:
		return nil, false
	}
}

func coerceExtraValue(key string, v any) any {
	norm := normalizeExtraKey(key)
	if norm == "" {
		return v
	}
	if _, ok := extraParamIntKeys[norm]; ok {
		if n, ok := toInt64(v); ok {
			return n
		}
	}
	if _, ok := extraParamFloatKeys[norm]; ok {
		if f, ok := toFloat64(v); ok {
			return f
		}
	}
	return v
}

func normalizeExtraKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	key = strings.ReplaceAll(key, "_", "")
	key = strings.ReplaceAll(key, "-", "")
	return strings.ToLower(key)
}

func toInt64(v any) (int64, bool) {
	switch tv := v.(type) {
	case int:
		return int64(tv), true
	case int64:
		return tv, true
	case float64:
		return int64(tv), true
	case float32:
		return int64(tv), true
	case jsonNumber:
		if i, err := tv.Int64(); err == nil {
			return i, true
		}
		if f, err := tv.Float64(); err == nil {
			return int64(f), true
		}
		return 0, false
	case string:
		if s := strings.TrimSpace(tv); s != "" {
			if i, err := strconv.ParseInt(s, 10, 64); err == nil {
				return i, true
			}
			if f, err := strconv.ParseFloat(s, 64); err == nil {
				return int64(f), true
			}
		}
	}
	return 0, false
}

func toFloat64(v any) (float64, bool) {
	switch tv := v.(type) {
	case float64:
		return tv, true
	case float32:
		return float64(tv), true
	case int:
		return float64(tv), true
	case int64:
		return float64(tv), true
	case jsonNumber:
		if f, err := tv.Float64(); err == nil {
			return f, true
		}
		if i, err := tv.Int64(); err == nil {
			return float64(i), true
		}
		return 0, false
	case string:
		if s := strings.TrimSpace(tv); s != "" {
			if f, err := strconv.ParseFloat(s, 64); err == nil {
				return f, true
			}
		}
	}
	return 0, false
}

// jsonNumber is a tiny shim to avoid importing encoding/json in this file.
type jsonNumber interface {
	Float64() (float64, error)
	Int64() (int64, error)
}
