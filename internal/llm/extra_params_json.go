package llm

import (
	"encoding/json"
	"strings"
)

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
