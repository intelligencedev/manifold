# Manifold code review — plan for cmd/agentd/main.go

This document outlines a targeted, actionable plan to raise code quality for ./cmd/agentd/main.go. It follows the guidance in CODE_REVIEW.md and prioritizes safety, testability, and maintainability.

## Scope
- Target: manifold/cmd/agentd/main.go
- Goals: break up monolith, eliminate races, improve testability, add linting/tests/CI gates, harden security and observability.

## Summary of key findings (high-level)
- Monolithic main() (~2000+ LOC): many responsibilities (config, telemetry, DB, tools, HTTP handlers, orchestration, worker/warpp setup). Hard to unit test.
- Global mutable state and reassignment (eng, llm, toolRegistry, wfreg, warppRunner.Tools) causing potential data races and surprising behavior when updated at runtime.
- Engine callbacks (eng.OnDelta, eng.OnToolStart, eng.OnTool) are set globally per-request and thus can interleave across concurrent requests — critical concurrency bug.
- Repeated/duplicated code patterns (CORS headers, SSE wiring, JSON error handling, body-size limits) — extract helpers/middleware.
- Unsafe usage in /stt WAV parsing: unsafe.Pointer used to reinterpret bits -> use math.Float32frombits instead of unsafe conversion.
- Manual WAV parsing is fragile and duplicates functionality; no validation of chunk boundaries; susceptible to panics on malformed uploads.
- Serving audio with http.ServeFile using unvalidated filename from URL: possible path traversal and arbitrary file exposure.
- Numerous anonymous handlers capture environment; very hard to test in isolation. Lack of handler-level dependency injection.
- Several places log or return error strings that could leak secrets (e.g., API keys in persisted orchestrator entries) — ensure redaction.
- Missing centralized error response format and inconsistent status messages.

## Risks (priority)
- P0 (must fix before merge): global engine callback race causing cross-request data leakage and races. Path traversal exposure in /audio/ handler. Unsafe bit-casting in STT.
- P1 (high): Reassigning shared registries without synchronization leads to races. Monolithic main is untestable and hinders future changes.
- P2 (medium): Duplicate SSE/HTTP handling code increases maintenance cost. Manual WAV parsing and naive MIME checks.

## High-level remediation strategy
1. Introduce modular initialization and dependency injection
   - Extract initialization code into small packages/functions: config.Load, observability.Init, db.NewManager, makeEngines(...), makeToolRegistry(...), makeHTTPHandlers(...).
   - Each HTTP handler becomes a method on a typed handler struct with explicit dependencies (stores, engine, cfg). This enables unit testing.

2. Fix concurrency / data-race issues
   - Remove global mutable engine callbacks. eng.RunStream and eng.Run must accept per-request callback arguments or return a stream interface. If engine cannot be changed, wrap calls with a per-request proxy that marshals events to the specific request only.
   - Protect runtime updates to registries/engines with a RWMutex or use atomic.Value to publish immutable snapshots (preferred). Replace ad-hoc variable swaps with snapshot publish semantics.

3. Centralize common HTTP concerns
   - Implement middleware for: CORS, JSON responses, error handling, auth enforcement, body size limits, SSE encoding/flush logic.
   - Create helpers for reading JSON bodies with size limits and uniform error responses.

4. Remove unsafe and harden audio parsing
   - Replace unsafe float conversion with math.Float32frombits.
   - Prefer a tested audio parsing library or add robust bounds checks and unit tests for WAV parsing.
   - Add strict limits and validation for sample rate, channels, and bit depth.

5. Sanitize filesystem access
   - Serve audio files only from a configured directory. Resolve and validate the path (use filepath.Clean, ensure it is within configured audio dir). Avoid serving arbitrary paths from URL.

6. Improve observability & secrets handling
   - Ensure logs redact sensitive fields (API keys, tokens). Add an audit log for specialist/orchestrator changes but redact secrets.
   - Expose build info/version via /status or /meta.

7. Tests, linters, and CI
   - Add unit tests for handlers (table-driven), helper functions, WAV parsing, previewSnippet, storeChatTurn, withMaybeTimeout, resolveChatAccess logic.
   - Add integration tests using httptest.Server and in-memory/mocked stores.
   - Add golangci-lint config and run staticcheck, govet, gofmt checks in CI. Gate PRs with: go vet, go test -race -cover, golangci-lint, govulncheck.

## Concrete refactor plan (phased)

Phase A — Safety & small surface fixes (1–3 days)
- A1: Stop global engine callback races (hotfix)
  - Implement a per-request event broadcaster: do not assign eng.OnDelta/OnTool* globally. Instead, wrap engine calls so the engine uses callbacks passed in as params or use a small goroutine that filters events for the current request.
  - Tests: concurrent RunStream requests should not interleave deltas; run with -race.
- A2: Fix unsafe cast
  - Replace unsafe pointer conversion with math.Float32frombits. Add tests for WAV parsing edge cases.
- A3: Harden /audio/ handler
  - Serve files only from configured audio directory and validate filepath. Add tests for traversal attempts.

Phase B — Modularization & testability (3–7 days)
- B1: Split main into packages/functions
  - create package app or internal/server with Init* functions returning handler structs and a Run(serverOpts) entry. Keep main.go thin.
  - Move large anonymous handlers into methods (e.g., ProjectsHandler, ChatHandler, AgentHandler).
- B2: Centralize middleware and SSE helpers
  - Add middleware stack (auth, CORS, logging, size-limit). Implement SSE helper to reduce duplication.
- B3: Publish runtime configuration safely
  - Use atomic.Value for cfg/registries snapshots and ensure swaps are done atomically.

Phase C — Quality gates, tests, docs (5–10 days)
- C1: Add unit and integration tests covering endpoints, warpp personalization/execute paths (mocked), and security edge cases.
- C2: Add golangci-lint config, run in CI. Fix vet/staticcheck issues.
- C3: Add benchmarks for hot paths if perf concerns exist (LLM loop, RunStream) and run perf regression checks in CI.

## Example PR breakdown (suggested)
- PR 1 (hotfix): remove global engine callback assignment → add per-request callbacks; tests demonstrating concurrency fix. (P0)
- PR 2: Replace unsafe.Float conversion; add WAV parsing unit tests. (P0)
- PR 3: Sanitize /audio/ and centralize SSE formatting; add middleware for CORS. (P0/P1)
- PR 4: Extract handler types and DI for a subset of endpoints (projects, chat). Add tests. (P1)
- PR 5: Publish snapshots with atomic.Value, protect registry updates, and document runtime config. (P1)
- PR 6: Add CI gates (golangci-lint, vet, govulncheck, tests -race). (P2)

## Recommended short-term acceptance criteria
- All unit tests pass with `go test ./... -race` (no races). Basic integration tests for main web endpoints succeed.
- No global assignment to eng.OnDelta/OnTool*; RunStream uses per-request callbacks only.
- ServeFile path traversal test fails safely (blocked) and accepted audio file served correctly.
- Unsafe pointer usage removed.
- CI runs linters and tests on PRs.

## Longer-term improvements (optional)
- Consider replacing custom workflow persistence bootstrap with a clear migration path and idempotent seeding.
- Introduce feature flags/config-driven toggles for dev-mode behaviors (mock responses) so they are explicit.
- Add structured error types and an errors package to standardize responses and enable better client behavior handling.

## Estimated effort
- Phase A: 1–3 days
- Phase B: 3–7 days
- Phase C: 5–10 days

Total: ~2–3 weeks of focused engineering (can be split across several PRs). Prioritize P0 items immediately.

----
Notes: this plan was prepared after reviewing ./manifold/CODE_REVIEW.md and a static read of ./manifold/cmd/agentd/main.go. It is advisory; implementation may reveal additional smaller issues to triage.

