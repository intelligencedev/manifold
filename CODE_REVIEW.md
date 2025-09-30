# ✅ Expert Go (1.24+) Codebase Review Checklist — For Automated Agents

> Inputs the agent needs
> - Path(s) to repo(s)
> - Supported Go versions/targets (GOOS/GOARCH), CI provider
> - Org policies: min Go version, crypto/TLS/FIPS/security rules, license policy

──────────────────────────────────────────────────────────────────────────────

## 0) Setup & discovery
- [ ] Detect toolchain: `go version`; `go env -json`; `go env GOTOOLCHAIN` (note if set).
- [ ] Parse module roots: find all `go.mod` (monorepo?) and their module paths.
- [ ] Record CI runners / Dockerfiles / build scripts that install Go. Match toolchain expectations.

Exit if:
- [ ] `go` not found OR conflicting versions across CI/dev/container; report locations & versions.

──────────────────────────────────────────────────────────────────────────────

## 1) Governance: go.mod + toolchain baselines
- [ ] For each module:
  - [ ] `go` directive is the minimum language version actually required (prefer `1.24`+ if 1.24 features are used). Flag artificially high values.
  - [ ] If using a preferred toolchain: `toolchain` directive present and sensible; verify CI honors it. Add note if `GOTOOLCHAIN` will auto-upgrade.
  - [ ] Zero unexpected `replace` directives (allow only documented local/dev overrides); fail if any stray replaces remain.
  - [ ] Hygiene: `go mod tidy` produces no diff; `go.sum` tracked.
  - [ ] Private deps: `GOPRIVATE`/`GONOPROXY` respected in CI; if private fetch needed, check presence or plan for `GOAUTH`.
- [ ] Version selection sanity:
  - [ ] Explain MVS to output: do **not** force latest everywhere; ensure `go` (main) ≥ every dep’s stated minimum.
  - [ ] Emit a report of direct deps needing upgrade (security/critical fixes) vs “optional” bumps.

Artifacts:
- [ ] Attach `go mod graph`+`go list -m -json all` digests, and a tidy diff summary if any.

──────────────────────────────────────────────────────────────────────────────

## 2) Reproducible build metadata
- [ ] Confirm `go build` embeds VCS version (+`+dirty` if applicable). If hermetic builds are required, ensure `-buildvcs=false` used and documented.
- [ ] Ensure release builds record version via `runtime/debug.ReadBuildInfo` (surface in `--version` or logs).

──────────────────────────────────────────────────────────────────────────────

## 3) Language & stdlib features (Go 1.24 focus)
- Generics/type system
  - [ ] If type aliases with type params make APIs clearer, confirm use of **generic type aliases**; reject gratuitous complexity.
- Finalizers / cleanup
  - [ ] Prefer `runtime.AddCleanup` over `runtime.SetFinalizer` in new code. Check for cycles/leaks and multiple cleanups.
- Weak references
  - [ ] If weak refs are present, ensure justified and implemented via `weak` package; document ownership semantics.
- Filesystem safety
  - [ ] For dir-scoped operations on untrusted input, prefer `os.OpenRoot` / `os.Root` (or `os.OpenInRoot`) to avoid path escape; never initialize the root from user input.
- Allocation-friendly iteration
  - [ ] Where splitting/iterating large strings/bytes, consider `bytes.Lines` / `SplitSeq` / `FieldsSeq` and `strings.*Seq` helpers; keep code clear.
- JSON
  - [ ] Use `omitzero` where zero-value omission is intended (esp. `time.Time`); keep `omitempty` only for “empty” semantics.
- Crypto/TLS
  - [ ] Prefer AEAD; use `crypto/cipher.NewGCMWithRandomNonce` for sealed outputs that carry a random nonce.
  - [ ] Ensure no SHA-1 verification in `x509` chains; RSA < 2048 forbidden, <1024 never allowed.
  - [ ] PQ TLS: confirm default **X25519MLKEM768** enabled (unless explicitly disabled). Flag servers/clients that break with larger records; suggest `GODEBUG=tlsmlkem=0` only as compatibility fallback.
  - [ ] For constant-time behavior, use `crypto/subtle.WithDataIndependentTiming` where applicable.
- net/http
  - [ ] Check `Server.Protocols`/`Transport.Protocols` configuration; justify any **unencrypted HTTP/2 (h2c)** enablement.
  - [ ] Bound headers via `Transport.MaxResponseHeaderBytes`; validate 1xx handling; use `httptrace` where needed.
- Testing ergonomics
  - [ ] Benchmarks prefer `testing.B.Loop`; verify proper `B.Context`, `T/B.Chdir`, and `t.Cleanup`.
  - [ ] Concurrency tests: if complex, consider `testing/synctest` with `GOEXPERIMENT=synctest`.

──────────────────────────────────────────────────────────────────────────────

## 4) Project layout & API design
- [ ] Commands in `cmd/<app>`; libraries in cohesive packages (`internal/<pkg>` when appropriate); no import cycles.
- [ ] Package naming: short, evocative; avoid `util/common/types/...`.
- [ ] Public API:
  - [ ] Small **consumer** interfaces; return concrete types.
  - [ ] Receivers: value vs pointer by mutability/size/containment of mutex; do not mix on same type.
  - [ ] Context: accept `context.Context` as the first param; never store on structs.
  - [ ] Errors: multi-return idiom; `errors.Is/As/Join`; wrap with `%w`; lower-cased messages, no trailing punctuation. Avoid in-band error sentinels; prefer `(T, bool)` where appropriate.
  - [ ] Control flow: guard-clause errors; avoid naked returns in non-trivial funcs.
- [ ] Documentation:
  - [ ] Package doc comments present; exported identifiers documented and start with the name; examples compile (`go test ./...` runs `Example*`).

──────────────────────────────────────────────────────────────────────────────

## 5) Formatting, lint, vet
- [ ] `gofmt -s -l .` returns empty; `goimports` with proper import grouping (stdlib / external / internal).
- [ ] `go vet ./...` clean, including Go 1.24 analyzers:
  - [ ] `printf`: flag `fmt.Printf(s)` with non-const `s` and no args (use `fmt.Print`).
  - [ ] `tests`: malformed/misnamed tests/benchmarks/examples fixed.
  - [ ] `buildtag`: invalid `go1.X` point-release constraints fixed (use `go1.23`, not `go1.23.1`).
  - [ ] `copylock`: no copying `sync.Locker` in 3-clause for loops.
- [ ] `golangci-lint run` (curated set): gofmt, goimports, revive/staticcheck (and agreed org linters). Keep noise low; document exceptions.

──────────────────────────────────────────────────────────────────────────────

## 6) Build, test, fuzz, bench, coverage
- [ ] Build all: `go build ./...` on supported platforms; CI variant with `-race` when feasible.
- [ ] Tests: `go test -race -count=1 ./...` passes; target strong coverage on business logic (e.g., ≥80% or org policy). Produce `coverage.out`; compute package and total.
- [ ] Fuzz where valuable (`-fuzz=Fuzz` seeds for parsers/decoders); fuzz tests are isolated from unit tests.
- [ ] Benchmarks: use `testing.B.Loop`; avoid per-iteration setup; capture `-benchmem`. Store historical runs for regressions.
- [ ] Deterministic concurrent tests: adopt `testing/synctest` where races/flakes exist.

Artifacts:
- [ ] Attach coverage summary & top N low-coverage packages.
- [ ] Attach perf delta vs baseline if benches exist.

──────────────────────────────────────────────────────────────────────────────

## 7) Concurrency & memory model
- [ ] CI always runs `-race`; zero data races before merge.
- [ ] Goroutine lifetimes explicit: no leaks on send/recv; document lifetimes when non-trivial; cancel on errors and context timeouts.
- [ ] Synchronization correctness:
  - [ ] No busy-wait flags for cross-goroutine visibility; use channels/`sync` primitives.
  - [ ] No double-checked locking without proper sync.
  - [ ] Channels/locks: avoid deadlocks; buffer only with rationale; minimize goroutine proliferation.
  - [ ] `sync/atomic` only when truly required; ensure happens-before edges.
- [ ] Containers & maps:
  - [ ] Be aware of 1.24 **Swiss-table** maps and changed `sync.Map` perf; remove stale micro-opts; never copy structs with embedded mutexes.

──────────────────────────────────────────────────────────────────────────────

## 8) Networking & HTTP
- [ ] Set sane server/client timeouts (Read/Write/Header/Idle/Handshake).
- [ ] Always `Close()` response bodies; check `defer resp.Body.Close()` patterns.
- [ ] Respect idempotency/backoff rules on retries; propagate `context.Context`.
- [ ] Validate `Server.Protocols` / `Transport.Protocols` (HTTP/1.1, HTTP/2, optional h2c) per threat model and interoperability.

──────────────────────────────────────────────────────────────────────────────

## 9) Logging, errors, and observability
- [ ] Structured logging via `log/slog` (levels/handlers configurable); ensure PII/secret scrubbing; redact error values as needed.
- [ ] Expose version/build info from `debug.BuildInfo`; log at startup and in diagnostics.
- [ ] Profiling & tracing hooks present where needed (`pprof` endpoints or on-demand `go tool pprof` support).

──────────────────────────────────────────────────────────────────────────────

## 10) Performance & resources
- [ ] Allocation hygiene: prefer zero-values usable; avoid needless pointers; preallocate slices/maps when size is known.
- [ ] Run representative benches; inspect `allocs/op` and `B/op`.
- [ ] CPU/mem profiles for hot paths; verify improvements before merging micro-opts.

──────────────────────────────────────────────────────────────────────────────

## 11) Security, crypto, compliance
- [ ] **govulncheck** automated in CI for source and produced binaries; reachable vulns triaged first; attach report.
- [ ] Crypto posture:
  - [ ] No SHA-1 verification; RSA ≥2048; use AEAD (GCM/ChaCha20-Poly1305).
  - [ ] PQ TLS defaults (X25519MLKEM768) acceptable for your environments; otherwise document explicit `CurvePreferences`/GODEBUG workaround.
  - [ ] Use `crypto/rand` for secrets; **never** `math/rand`.
  - [ ] If FIPS 140-3 is required: confirm `GOFIPS140`/`GODEBUG=fips140=1` as per deployment guide; verify approved algorithms only.
- [ ] Secrets hygiene: no secrets in repo; env/secret manager used; logs crash reports scrubbed.
- [ ] Filesystem safety: prefer `os.Root` where user-controlled path components exist; forbid roots from user input.

──────────────────────────────────────────────────────────────────────────────

## 12) Modules, tools, and supply chain (Go 1.24 tools)
- [ ] Tools tracked via `tool` directives (not blank-import `tools.go`):
  - [ ] Add/update with: `go get -tool <module@version>`; run with `go tool <name>`.
- [ ] Verify CI caches tool downloads and uses pinned tool versions.
- [ ] Module proxies and private sources configured; document `GOAUTH` if needed.

──────────────────────────────────────────────────────────────────────────────

## 13) Platforms & ports
- [ ] Supported OS/arch list is current; validate deprecations (e.g., Linux ≥3.2; macOS 11 is **last** in 1.24).
- [ ] WebAssembly (if used): validate `go:wasmexport/wasmimport` types; initial memory expectations; build modes.

──────────────────────────────────────────────────────────────────────────────

## 14) CI/CD gates (suggested)
- [ ] Format/imports: `gofmt -s -l .` empty; `goimports` (or via linter).
- [ ] Build: `go build ./...` (+`-race` on CI where feasible).
- [ ] Vet: `go vet ./...`.
- [ ] Lint: `golangci-lint run`.
- [ ] Test: `go test -race -cover ./...`.
- [ ] Bench (optional): `go test -run=^$ -bench=. -benchmem ./...`.
- [ ] Vuln: `govulncheck ./...` (source + binaries).
- [ ] Module hygiene: `go mod tidy` & assert no diff in `go.mod`/`go.sum`.
- [ ] Tools (1.24+): `go get -tool <tool>`; `go tool <tool>`.

──────────────────────────────────────────────────────────────────────────────

## 15) Quick anti-pattern sweep (auto-flag → link to occurrences)
- [ ] Busy-waiting on booleans for cross-goroutine visibility.
- [ ] Double-checked locking without proper sync.
- [ ] `fmt.Printf(nonConstString)` with no args.
- [ ] `context.Context` stored on structs.
- [ ] Leaking goroutines / missing `Close` on `http.Response.Body`.
- [ ] `tools.go` blank-import pattern (should be tool directives).
- [ ] `encoding/json` `omitempty` used when `omitzero` intended (esp. `time.Time`).
- [ ] Crypto: SHA-1 in x509 verify, RSA<2048, OFB/CFB modes, or `math/rand` for secrets.
- [ ] Paths built from user input without `os.Root` constraints.

──────────────────────────────────────────────────────────────────────────────

# Review completion criteria (agent must prove):
- [ ] All gates in §14 pass on CI for every module.
- [ ] No high/critical reachable vulns remain (attach govulncheck proof).
- [ ] Coverage target achieved or justified with risk notes.
- [ ] Performance deltas acceptable or better on key benches.
- [ ] Security/TLS/FIPS posture explicitly documented.
- [ ] All TODOs from this checklist resolved or tracked with owners/dates.

