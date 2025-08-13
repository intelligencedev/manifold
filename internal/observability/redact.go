package observability

import (
    "encoding/json"
    "strings"
)

var sensitiveKeys = []string{
    "api_key", "apikey", "apiKey", "x-api-key", "authorization", "auth", "token", "access_token", "refresh_token", "password", "secret", "bearer",
}

// RedactJSON takes a JSON payload and redacts sensitive values based on common key names.
func RedactJSON(raw json.RawMessage) json.RawMessage {
    if len(raw) == 0 {
        return raw
    }
    var v any
    if err := json.Unmarshal(raw, &v); err != nil {
        return raw
    }
    redacted := redactValue(v)
    b, err := json.Marshal(redacted)
    if err != nil {
        return raw
    }
    return b
}

func redactValue(v any) any {
    switch val := v.(type) {
    case map[string]any:
        for k, vv := range val {
            if isSensitiveKey(k) {
                val[k] = "[REDACTED]"
            } else {
                val[k] = redactValue(vv)
            }
        }
        return val
    case []any:
        for i := range val {
            val[i] = redactValue(val[i])
        }
        return val
    default:
        return v
    }
}

func isSensitiveKey(k string) bool {
    low := strings.ToLower(k)
    for _, s := range sensitiveKeys {
        if low == s {
            return true
        }
        // contains common header forms
        if strings.Contains(low, s) {
            return true
        }
    }
    return false
}

