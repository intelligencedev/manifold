package gym

import (
	"os"
	"path/filepath"
	"testing"
)

const restartSafeYAML = `
memory:
    enabled: true
    belief:
        retrieval:
            maxTokensPerPrompt: 700
            minConfidence: 0.35
archaeology:
  enabled: true
  auto_activate:
    enabled: false
    min_confidence: 0.85
    conflict_similarity_floor: 0.50
`

func mustChecks(t *testing.T, yaml string, live *LiveSnapshot) []Check {
	t.Helper()
	checks, err := CheckRestartParity([]byte(yaml), live)
	if err != nil {
		t.Fatalf("CheckRestartParity: %v", err)
	}
	return checks
}

func failedNames(checks []Check) []string {
	var out []string
	for _, c := range checks {
		if !c.Pass {
			out = append(out, c.Name)
		}
	}
	return out
}

func TestRestartParitySafeConfig(t *testing.T) {
	checks := mustChecks(t, restartSafeYAML, nil)
	if !RestartSafe(checks) {
		t.Fatalf("expected restart-safe config, failed checks: %v", failedNames(checks))
	}
}

func TestRestartParityBlocksAutoActivateDrift(t *testing.T) {
	drifted := `
memory:
    enabled: true
archaeology:
  auto_activate:
    enabled: true
    min_confidence: 0.85
`
	checks := mustChecks(t, drifted, nil)
	if RestartSafe(checks) {
		t.Fatal("expected gate to block: archaeology.auto_activate.enabled=true violates Phase 3 pin (false)")
	}
}

func TestRestartParityBlocksMemoryDisabled(t *testing.T) {
	drifted := `
memory:
    enabled: false
archaeology:
  auto_activate:
    enabled: false
    min_confidence: 0.85
`
	checks := mustChecks(t, drifted, nil)
	if RestartSafe(checks) {
		t.Fatal("expected gate to block: memory.enabled=false would disable memory lanes on restart (F2)")
	}
}

func TestRestartParityBlocksMinConfidenceDrift(t *testing.T) {
	drifted := `
memory:
    enabled: true
archaeology:
  auto_activate:
    enabled: false
    min_confidence: 0.5
`
	checks := mustChecks(t, drifted, nil)
	if RestartSafe(checks) {
		t.Fatal("expected gate to block: min_confidence=0.5 violates Phase 3 pin (0.85)")
	}
}

func TestRestartParityBlocksInertIncludeContradictionsFalse(t *testing.T) {
	drifted := `
memory:
    enabled: true
    belief:
        retrieval:
            includeContradictions: false
archaeology:
  auto_activate:
    enabled: false
    min_confidence: 0.85
`
	checks := mustChecks(t, drifted, nil)
	if RestartSafe(checks) {
		t.Fatal("expected gate to block: explicit includeContradictions=false is inert drift (loader forces true)")
	}
}

func TestRestartParityAllowsAbsentIncludeContradictions(t *testing.T) {
	checks := mustChecks(t, restartSafeYAML, nil)
	for _, c := range checks {
		if c.Name == "restart-parity/pin["+KnobBeliefIncludeContradictions+"]" && !c.Pass {
			t.Fatal("absent includeContradictions must pass (loader default is true)")
		}
	}
}

func TestRestartParityBlocksMissingPin(t *testing.T) {
	drifted := `
memory:
    enabled: true
archaeology:
  enabled: true
`
	checks := mustChecks(t, drifted, nil)
	if RestartSafe(checks) {
		t.Fatal("expected gate to block: archaeology.auto_activate pins missing on disk")
	}
}

func TestRestartParityDiskVsLiveDrift(t *testing.T) {
	driftedDiskOff := `
memory:
    enabled: false
archaeology:
  auto_activate:
    enabled: false
    min_confidence: 0.85
`
	live := &LiveSnapshot{MemoryEnabled: true, EvolvingEnabled: true, BeliefEnabled: true, MagmaEnabled: true}
	checks := mustChecks(t, driftedDiskOff, live)
	found := false
	for _, c := range checks {
		if c.Name == "restart-parity/disk-vs-live[memory.enabled]" {
			found = true
			if c.Pass {
				t.Fatal("expected disk-vs-live drift check to fail: disk=false live=true (F2)")
			}
		}
	}
	if !found {
		t.Fatal("disk-vs-live check missing when LiveSnapshot supplied")
	}
	if RestartSafe(checks) {
		t.Fatal("expected gate to block restart on disk-vs-live drift")
	}
}

func TestRestartParityDiskVsLiveParity(t *testing.T) {
	live := &LiveSnapshot{MemoryEnabled: true, EvolvingEnabled: true, BeliefEnabled: true, MagmaEnabled: true}
	checks := mustChecks(t, restartSafeYAML, live)
	if !RestartSafe(checks) {
		t.Fatalf("expected parity with live daemon, failed checks: %v", failedNames(checks))
	}
}

func TestRestartSafeEmptyIsUnsafe(t *testing.T) {
	if RestartSafe(nil) {
		t.Fatal("no checks run must mean unsafe restart")
	}
}

func TestRestartParityRejectsInvalidYAML(t *testing.T) {
	if _, err := CheckRestartParity([]byte("memory: [unclosed"), nil); err == nil {
		t.Fatal("expected parse error for invalid yaml")
	}
}

// TestRestartParityRepoConfig is the operational gate: when the deployment's
// real config.yaml exists at the repo root (it is gitignored, so disk is
// authoritative), it must satisfy every restart invariant. Run this test
// BEFORE any daemon restart:
//
//	go test ./internal/agent/memory/gym -run RestartParityRepoConfig
func TestRestartParityRepoConfig(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "config.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("no deployment config.yaml at repo root (%v); pin gate not applicable", err)
	}
	checks, err := CheckRestartParityFile(path, nil)
	if err != nil {
		t.Fatalf("CheckRestartParityFile: %v", err)
	}
	for _, c := range checks {
		if !c.Pass {
			t.Errorf("RESTART BLOCKED — %s: %s", c.Name, c.Detail)
		}
	}
	if !RestartSafe(checks) {
		t.Fatal("config.yaml violates restart invariants; do NOT restart the daemon until reconciled")
	}
}
