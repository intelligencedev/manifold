package warpp

import (
	"fmt"
	"strings"
)

// EvalGuard evaluates a minimal guard expression against attributes.
// Supported forms:
//   - "" or "true" → true
//   - "A.key" → presence and truthiness of attribute
//   - "not A.key" → negation
//   - "A.key == 'value'" / "A.key != 'value'"
func EvalGuard(guard string, A Attrs) bool {
	g := strings.TrimSpace(guard)
	if g == "" || g == "true" {
		return true
	}
	if strings.HasPrefix(g, "not ") {
		return !EvalGuard(strings.TrimSpace(strings.TrimPrefix(g, "not ")), A)
	}
	// presence check: A.key
	if strings.HasPrefix(g, "A.") && !strings.Contains(g, "==") && !strings.Contains(g, "!=") {
		key := strings.TrimPrefix(g, "A.")
		v, ok := A[key]
		if !ok {
			return false
		}
		switch t := v.(type) {
		case bool:
			return t
		case string:
			return t != ""
		default:
			return v != nil
		}
	}
	// equality/inequality
	var op string
	if strings.Contains(g, "!=") {
		op = "!="
	} else if strings.Contains(g, "==") {
		op = "=="
	}
	if op != "" {
		parts := strings.SplitN(g, op, 2)
		if len(parts) == 2 {
			left := strings.TrimSpace(parts[0])
			right := strings.TrimSpace(parts[1])
			if strings.HasPrefix(left, "A.") {
				key := strings.TrimPrefix(left, "A.")
				val := fmt.Sprintf("%v", A[key])
				right = strings.Trim(right, "'\"")
				if op == "==" {
					return val == right
				}
				return val != right
			}
		}
	}
	// default: conservative allow
	return true
}
