package observability

import (
	"encoding/json"
	"reflect"
	"strings"
	"time"
)

var sensitiveKeys = []string{
	"api_key", "apikey", "apiKey", "x-api-key", "authorization", "auth", "token", "access_token", "refresh_token", "password", "secret", "bearer",
}

const RedactedValue = "[REDACTED]"

var (
	rawMessageType = reflect.TypeOf(json.RawMessage{})
	timeType       = reflect.TypeOf(time.Time{})
)

// RedactJSON takes a JSON payload and redacts sensitive values based on common key names.
func RedactJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return raw
	}
	redacted := RedactValue(v)
	b, err := json.Marshal(redacted)
	if err != nil {
		return raw
	}
	return b
}

// RedactValue returns a recursively redacted copy of v. Maps with sensitive key
// names have their values replaced; slices and nested maps are redacted in place
// in the returned copy.
func RedactValue(v any) any {
	return redactValue(v)
}

func redactValue(v any) any {
	if v == nil {
		return nil
	}
	return redactReflectValue(reflect.ValueOf(v))
}

func redactReflectValue(v reflect.Value) any {
	if !v.IsValid() {
		return nil
	}
	if v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil
		}
		return redactReflectValue(v.Elem())
	}

	if v.Type() == rawMessageType {
		raw := v.Interface().(json.RawMessage)
		if len(raw) == 0 {
			return raw
		}
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			return string(raw)
		}
		return redactValue(value)
	}

	if v.Type() == timeType {
		return v.Interface()
	}

	switch v.Kind() {
	case reflect.Map:
		if v.Type().Key().Kind() != reflect.String {
			return v.Interface()
		}
		out := make(map[string]any, v.Len())
		iter := v.MapRange()
		for iter.Next() {
			k := iter.Key().String()
			if isSensitiveKey(k) {
				out[k] = RedactedValue
				continue
			}
			out[k] = redactReflectValue(iter.Value())
		}
		return out
	case reflect.Slice, reflect.Array:
		out := make([]any, v.Len())
		for i := 0; i < v.Len(); i++ {
			out[i] = redactReflectValue(v.Index(i))
		}
		return out
	case reflect.Struct:
		out := make(map[string]any, v.NumField())
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			field := t.Field(i)
			if field.PkgPath != "" {
				continue
			}
			name, ok := jsonFieldName(field)
			if !ok {
				continue
			}
			if isSensitiveKey(name) {
				out[name] = RedactedValue
				continue
			}
			out[name] = redactReflectValue(v.Field(i))
		}
		return out
	default:
		return v.Interface()
	}
}

func jsonFieldName(field reflect.StructField) (string, bool) {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", false
	}
	if idx := strings.IndexByte(tag, ','); idx >= 0 {
		tag = tag[:idx]
	}
	if tag != "" {
		return tag, true
	}
	return field.Name, true
}

func isSensitiveKey(k string) bool {
	low := strings.ToLower(k)
	if strings.HasSuffix(low, "tokens") {
		return false
	}
	normalized := strings.NewReplacer("-", "_").Replace(low)
	switch normalized {
	case "auth", "token", "access_token", "refresh_token", "id_token", "session_token":
		return true
	}
	if strings.HasSuffix(normalized, "_auth") || strings.HasSuffix(normalized, "_token") || strings.HasSuffix(normalized, "token") {
		return true
	}
	for _, s := range sensitiveKeys {
		if s == "auth" || s == "token" || s == "access_token" || s == "refresh_token" {
			continue
		}
		needle := strings.NewReplacer("-", "_").Replace(strings.ToLower(s))
		if normalized == needle {
			return true
		}
		// contains common header forms
		if strings.Contains(normalized, needle) {
			return true
		}
	}
	return false
}
