# Memory Gym (Tier 1)

Offline, deterministic ground-truth scenarios for the unified memory runtime.
This is Tier 1 of the memory test-and-tune loop: everything runs in-process
against in-memory stores (`databases.NewMemoryBeliefStore`,
`databases.NewMemoryDecisionStore`, `databases.NewMemoryGraph/Vector`) and a
deterministic 3-gram embedder. **No LLM, no network, no daemon.**

## Why

`memory.*` and `archaeology.*` are startup-only configuration on a live
`agentd` (not runtime-mutable via `/api/config/agentd`), and the daemon must
not be restarted during the tuning loop. The gym therefore injects config
permutations directly via the `Knobs` struct, whose fields map 1:1 onto
`config.yaml` paths (see the `Knob*` tag constants).

## Layout

- `knobs.go` — injectable subset of `memory.*` / `archaeology.*` config;
  `DefaultKnobs()` mirrors `config.yaml.example`; `Knobs.Values()` reports the
  exact config-path → value map embedded in every result.
- `scenario.go` — scenario schema: seeded state, probe steps, ground-truth
  expectations, and the knob tags each scenario exercises.
- `suite.go` — the scenario suite, covering every subsystem:
  - `decision_lane`: deterministic scope-walk ranking, lifecycle exclusion,
    prompt budget caps.
  - `belief`: confidence floor, contradiction visibility, per-prompt caps,
    scope proximity.
  - `evolving`: topK, similarity threshold + keyword fallback, smart prune,
    FIFO max size, recent window.
  - `magma`: structured vs plain context, causal extraction, max-node caps.
  - `archaeology`: simple distiller extraction, deterministic auto-activation
    (floor / duplicate / conflict), belief reactor lifecycle (retraction →
    stale, confidence drop → needs_review, **no automated stale→active**),
    reconstruction verdicts.
  - `runtime_fusion`: lane ordering (policy → decision → belief → magma →
    evolving → recent) and global token-budget truncation.
- `harness.go` — builds one wired `memory.Runtime` per scenario+knobs and
  evaluates expectations into deterministic `Check` results.
- `results.go` — JSON suite-result writer for Phase 2 scoring.
- `restartparity.go` — the Phase 5 **restart-parity gate**: disk-vs-pin and
  disk-vs-live invariant checks that must pass before any daemon restart
  (see "Restart-parity gate" below).

## Running

```sh
CGO_ENABLED=0 go test ./internal/agent/memory/gym/...
```

(`CGO_ENABLED=0` is required in sandboxed environments where `xcrun` is
blocked; cgo failures there are environmental, not code signal.)

To emit the machine-readable scorecard input:

```sh
CGO_ENABLED=0 MEMORY_GYM_RESULTS=baseline-results.json \
  go test ./internal/agent/memory/gym/ -run TestSuiteGroundTruth -count=1
```

Phase 2/3 sweeps call `RunSuite(ctx, name, Suite(), knobs)` with permuted
`Knobs` and compare `SuiteResult` payloads across knob values; scenario
`Knobs` tags attribute each check to the configuration dimensions it
exercises. Note: scenarios pin knob overrides via `Mutate` when their ground
truth depends on a non-default config point — those overrides apply on top of
the swept base knobs.

## Invariants encoded as ground truth

- Decision lane retrieval is plain store reads (no LLM/tool calls).
- Stale/superseded/revoked decisions never enter the prompt lane.
- Auto-activation is deterministic and bounded by confidence floor,
  duplicate hash, and conflict similarity; conflicts flag the existing
  decision `needs_review` instead of mutating it.
- Stale→active transitions are never automated; reinstating a belief does
  not reactivate a stale decision.

## Restart-parity gate (Tier 2, permanent)

`memory.*` and `archaeology.*` are startup-only: whatever is in the on-disk
`config.yaml` (gitignored — disk is authoritative) silently becomes the live
behavior at the next restart. Phase 4 finding F2 showed this drift can
disable memory lanes without any runtime signal. `restartparity.go` makes
the drift a deterministic, blocking check:

- **Pin parity (disk vs invariants).** `DefaultRestartInvariants()` pins
  `memory.enabled=true`, `archaeology.auto_activate.enabled=false`,
  `archaeology.auto_activate.min_confidence=0.85`, and rejects an explicit
  `memory.belief.retrieval.includeContradictions: false` (the loader
  invariant forces it true; an explicit false on disk is inert drift).
- **Disk-vs-live parity (optional).** Pass a `LiveSnapshot` built from the
  `config` block of `GET /api/observability/memory/overview` (never the
  unredacted `/api/config/agentd`) and the gate also fails when on-disk
  `memory.enabled` disagrees with the live daemon.

A restart is allowed only when `RestartSafe(checks)` is true. Run the gate
**before every daemon restart**:

```sh
CGO_ENABLED=0 go test ./internal/agent/memory/gym -run TestRestartParityRepoConfig -count=1
```

`TestRestartParityRepoConfig` validates the deployment's real `config.yaml`
at the repo root and fails loudly (`RESTART BLOCKED — …`) on any violated
invariant; it skips only when no deployment config exists (e.g. CI).
