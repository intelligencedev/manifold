package registry

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ComputeContentHash creates a deterministic digest for a prompt template and its variables.
func ComputeContentHash(template string, vars map[string]VariableSchema) (string, error) {
	if err := ValidateTemplate(template, vars); err != nil {
		return "", err
	}
	normalised := map[string]any{
		"template":  template,
		"variables": normaliseVariables(vars),
	}
	payload, err := json.Marshal(normalised)
	if err != nil {
		return "", fmt.Errorf("marshal prompt for hashing: %w", err)
	}
	sum := sha256.Sum256(payload)
	return fmt.Sprintf("sha256:%x", sum[:]), nil
}

// ValidateTemplate performs lightweight checks to catch template mistakes early.
func ValidateTemplate(template string, vars map[string]VariableSchema) error {
	if strings.TrimSpace(template) == "" {
		return fmt.Errorf("template cannot be empty")
	}

	missing := findUnboundPlaceholders(template, vars)
	if len(missing) > 0 {
		return fmt.Errorf("template references undefined variables: %s", strings.Join(missing, ", "))
	}
	return nil
}

func findUnboundPlaceholders(template string, vars map[string]VariableSchema) []string {
	placeholders := make(map[string]struct{})
	for {
		start := strings.Index(template, "{{")
		if start == -1 {
			break
		}
		end := strings.Index(template[start+2:], "}}")
		if end == -1 {
			break
		}
		name := strings.TrimSpace(template[start+2 : start+2+end])
		placeholders[name] = struct{}{}
		template = template[start+2+end+2:]
	}
	var missing []string
	for name := range placeholders {
		if _, ok := vars[name]; !ok {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	return missing
}

func normaliseVariables(vars map[string]VariableSchema) []VariableSchema {
	if len(vars) == 0 {
		return nil
	}
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]VariableSchema, 0, len(vars))
	for _, key := range keys {
		v := vars[key]
		v.Name = key
		out = append(out, v)
	}
	return out
}
