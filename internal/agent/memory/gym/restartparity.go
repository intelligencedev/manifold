package gym

// Restart-parity gate (Phase 5, Gate 2).
//
// memory.* and archaeology.* are startup-only on a live daemon: they cannot
// be mutated through /api/config/agentd, so whatever is on disk in
// config.yaml at restart time silently becomes the live behavior. Phase 4
// finding F2 showed the on-disk config can drift from the validated live
// session (memory.enabled:false on disk vs enabled live), which means a
// restart can disable memory lanes or flip archaeology auto-activation
// against the Phase 3 safety pins without any runtime signal.
//
// This gate makes that drift a deterministic, blocking check:
//
//  1. Pin parity (disk vs invariants): the on-disk config.yaml must satisfy
//     every RestartInvariant (at minimum memory.enabled=true,
//     archaeology.auto_activate.enabled=false,
//     archaeology.auto_activate.min_confidence=0.85, and
//     memory.belief.retrieval.includeContradictions must never be an
//     explicit false — the loader invariant forces it to true, so an
//     explicit false on disk is inert drift that misleads readers).
//  2. Disk-vs-live parity (optional): when the caller supplies a
//     LiveSnapshot built from Tier 2 observability
//     (GET /api/observability/memory/overview — never the unredacted
//     /api/config/agentd), the on-disk lane enablement must match the live
//     daemon so a restart cannot silently change memory behavior.
//
// A restart is safe only if RestartSafe(checks) is true. Operators and CI
// must run this gate (go test ./internal/agent/memory/gym -run RestartParity
// or CheckRestartParityFile directly) BEFORE any daemon restart.

import (
	"fmt"
	"math"
	"os"
	"strings"

	yaml "gopkg.in/yaml.v3"
)

// RestartInvariant is one startup-only configuration value that must hold on
// disk before a daemon restart is allowed.
type RestartInvariant struct {
	// Path is the dotted config.yaml path, e.g. "archaeology.auto_activate.enabled".
	Path string
	// Want is the required value when the key is present on disk.
	Want any
	// AllowAbsent marks invariants whose key may be omitted from disk
	// because the config loader forces the safe default. When the key is
	// present it must still equal Want.
	AllowAbsent bool
	// Reason documents why the invariant exists so gate failures are
	// self-explanatory.
	Reason string
}

// DefaultRestartInvariants returns the Phase 3 / Phase 5 pinned invariants
// that every restart must preserve.
func DefaultRestartInvariants() []RestartInvariant {
	return []RestartInvariant{
		{
			Path:   "memory.enabled",
			Want:   true,
			Reason: "Phase 4 finding F2: disk drift to memory.enabled=false would disable all memory lanes on restart",
		},
		{
			Path:   KnobAutoActivateEnabled,
			Want:   false,
			Reason: "Phase 3 safety pin: decision auto-activation must stay off",
		},
		{
			Path:   KnobAutoActivateMinConfidence,
			Want:   0.85,
			Reason: "Phase 3 safety pin: auto-activate confidence floor",
		},
		{
			Path:        KnobBeliefIncludeContradictions,
			Want:        true,
			AllowAbsent: true,
			Reason:      "loader invariant forces includeContradictions=true; an explicit false on disk is inert drift",
		},
	}
}

// LiveSnapshot captures the lane enablement reported by the live daemon's
// Tier 2 observability endpoint (GET /api/observability/memory/overview,
// "config" block). It is supplied by the caller so this package stays
// offline and deterministic, and so the gate never touches the unredacted
// /api/config/agentd endpoint.
type LiveSnapshot struct {
	MemoryEnabled   bool `json:"memoryEnabled"`
	EvolvingEnabled bool `json:"evolvingEnabled"`
	BeliefEnabled   bool `json:"beliefEnabled"`
	MagmaEnabled    bool `json:"magmaEnabled"`
}

// CheckRestartParityFile reads the on-disk config file and runs the
// restart-parity gate against it. live may be nil when only pin parity is
// being validated (e.g. in CI where no daemon is running).
func CheckRestartParityFile(path string, live *LiveSnapshot) ([]Check, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("restart-parity: read config %s: %w", path, err)
	}
	return CheckRestartParity(data, live)
}

// CheckRestartParity runs the restart-parity gate against raw config.yaml
// bytes. It returns one Check per invariant (plus disk-vs-live checks when a
// LiveSnapshot is provided); any failing check means a restart is unsafe.
func CheckRestartParity(diskYAML []byte, live *LiveSnapshot) ([]Check, error) {
	var root map[string]any
	if err := yaml.Unmarshal(diskYAML, &root); err != nil {
		return nil, fmt.Errorf("restart-parity: parse config yaml: %w", err)
	}

	invariants := DefaultRestartInvariants()
	checks := make([]Check, 0, len(invariants)+1)
	for _, inv := range invariants {
		got, found := lookupConfigPath(root, inv.Path)
		switch {
		case !found && inv.AllowAbsent:
			checks = append(checks, check(
				"restart-parity/pin["+inv.Path+"]",
				true,
				"",
			))
		case !found:
			checks = append(checks, check(
				"restart-parity/pin["+inv.Path+"]",
				false,
				fmt.Sprintf("key missing on disk; want %v (%s)", inv.Want, inv.Reason),
			))
		default:
			checks = append(checks, check(
				"restart-parity/pin["+inv.Path+"]",
				configValueEquals(got, inv.Want),
				fmt.Sprintf("disk=%v want %v (%s)", got, inv.Want, inv.Reason),
			))
		}
	}

	if live != nil {
		got, found := lookupConfigPath(root, "memory.enabled")
		diskEnabled := found && configValueEquals(got, true)
		checks = append(checks, check(
			"restart-parity/disk-vs-live[memory.enabled]",
			diskEnabled == live.MemoryEnabled,
			fmt.Sprintf("disk=%v live=%v — restart would silently change memory lane enablement (Phase 4 finding F2)",
				diskEnabled, live.MemoryEnabled),
		))
	}
	return checks, nil
}

// RestartSafe reports whether every restart-parity check passed. A restart
// must be blocked unless this returns true.
func RestartSafe(checks []Check) bool {
	if len(checks) == 0 {
		return false
	}
	for _, c := range checks {
		if !c.Pass {
			return false
		}
	}
	return true
}

// lookupConfigPath walks a dotted path through nested YAML mappings.
func lookupConfigPath(root map[string]any, path string) (any, bool) {
	parts := strings.Split(path, ".")
	var cur any = root
	for _, part := range parts {
		node, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		next, ok := node[part]
		if !ok {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

// configValueEquals compares a YAML-decoded value against an expected value,
// tolerating int/float decoding differences.
func configValueEquals(got, want any) bool {
	if gf, gok := asFloat(got); gok {
		if wf, wok := asFloat(want); wok {
			return math.Abs(gf-wf) < 1e-9
		}
		return false
	}
	return got == want
}

func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	default:
		return 0, false
	}
}
