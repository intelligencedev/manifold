package gym

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WriteResults persists a suite result as indented JSON so Phase 2 scoring
// (and the Transit scorecard) can consume gym runs without re-parsing test
// output.
func WriteResults(path string, suite SuiteResult) error {
	payload, err := json.MarshalIndent(suite, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal suite result: %w", err)
	}
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create results dir: %w", err)
		}
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return fmt.Errorf("write suite result: %w", err)
	}
	return nil
}
