# Manifold Cockpit — New Frontend Plan

> **Scope:** Build a second, parallel Vue 3 frontend at `frontend/` that implements the
> "Agent Fleet Cockpit" concept on top of the existing Manifold backend (`agentd`).
> The current `web/agentd-ui` is preserved untouched; a new make target produces an
> `agentd` binary that embeds the cockpit instead of (or alongside) the classic UI.
> Refactoring of `agentd` is permitted where the cockpit surfaces require new
> capabilities, but no existing wire contracts may break.

---

## 1. What we are building, in one paragraph

A Vue 3 + TypeScript "operator cockpit" for Manifold — the seven-surface Fleet Cockpit
described in the design memo (Fleet Map, Intent Console, Interrupt Inbox, Trust &
Telemetry, Memory Garden, Replay & Counterfactuals, Constitution Editor). It is a
sibling of `web/agentd-ui`, lives in `frontend/`, embeds into `agentd` via a new
`internal/cockpitui` package, and ships through a `make build-cockpit` target. It
**reuses every existing backend API** wherever possible and adds a small, additive
set of new endpoints for cockpit-only concerns (live fleet topology, trust ledger,
constitution, replay log). No existing route, event name, or schema is removed or
renamed.

---

## 2. Inventory of what already exists in Manifold

The cockpit is mostly a re-presentation of capabilities that are already in the
backend. Inventory of useful primitives discovered in `repos/manifold`:

| Cockpit surface | Already-present backing | Source of truth |
|---|---|---|
| **Fleet Map** (agent nodes, delegation edges) | Specialists registry, teams, agent engine delegation with `call_id`/`parent_call_id`/`depth` events, `/agent/run` SSE | `internal/specialists/*`, `internal/agent/delegate.go`, `internal/agentd/handlers_specialists.go`, `internal/agentd/handlers_teams.go`, `web/agentd-ui/src/api/chat.ts` |
| **Intent Console** (objectives, decomposition) | Chat sessions + targets (orchestrator / specialist / team), Flow v2 workflows as runnable plans | `handlers_chat.go`, `handlers_flow_v2.go`, `internal/flow/*`, `flow_v2_runtime.go` |
| **Interrupt Inbox** | `input_request` / `input_request_cancelled` SSE events + `/api/chat/input-requests/{id}/answer` | `chat_input_request.go`, `internal/tools/inputrequest/*` |
| **Trust & Telemetry** | `/api/metrics/{tokens,memory,traces,logs}` (ClickHouse + local fallback), `/api/runs`, OTLP traces | `metrics_clickhouse.go`, `traces_clickhouse.go`, `logs_clickhouse.go`, `process_metrics.go` |
| **Memory Garden** | Transit, evolving memory, beliefs, RAG | `internal/transit/*`, `handlers_transit.go`, `handlers_memory.go`, `handlers_belief_debug.go`, `internal/rag/*` |
| **Replay & Counterfactuals** | Chat session messages + specialist activities (already persisted) + agent thread records | `internal/persistence/databases/*chat*`, `/api/chat/sessions/{id}/activities` |
| **Constitution Editor** | Specialists CRUD + agentd config + policy package | `handlers_specialists.go`, `handlers_config.go`, `internal/policy/*` |
| **MCP / tool surface** | MCP server registry + tool discovery | `handlers_mcp.go`, `internal/tools/discovery/*`, `internal/mcpclient/*` |
| **Pulse / Scheduling** | Pulse runtime + room-task scheduler | `internal/pulse/*`, `matrix_pulse.go` |
| **Project sandboxing** | Projects service + workspaces + sandbox path policy | `internal/projects/service.go`, `internal/workspaces/manager.go`, `internal/sandbox/*` |
| **Auth** | OIDC/OAuth2 + Postgres sessions | `internal/auth/*`, `auth_init.go` |
| **OpenAPI** | Runtime `/openapi.json` + `cmd/openapi` generator (~77 routes) | `internal/apidocs/spec.go`, `handlers_openapi.go` |

**Takeaway:** ~85 % of the cockpit is a UI on top of APIs that already exist.
The interesting backend work is concentrated in two new areas: a **live fleet
event bus** (a normalized SSE stream of cross-session agent activity) and a
**trust/policy/replay** layer.

---

## 3. Cockpit ↔ Manifold mapping

### 3.1 Direct reuse (no backend change)

| Cockpit primitive | Manifold endpoint(s) |
|---|---|
| Topology nodes (specialists, teams) | `GET /api/specialists`, `GET /api/teams`, `GET /api/specialists/defaults` |
| Topology edges (live delegation) | SSE on `/agent/run` and `/api/prompt`: `agent_start`, `agent_tool_start`, `agent_final`, with `call_id` / `parent_call_id` / `depth` |
| Active runs | `GET /api/runs`, `GET /api/chat/sessions`, `GET /api/chat/sessions/{id}/activities` |
| Interrupt queue | SSE `input_request` events + `POST /api/chat/input-requests/{id}/answer` |
| Telemetry tiles | `GET /api/metrics/tokens`, `/api/metrics/memory`, `/api/metrics/traces`, `/api/metrics/logs` |
| Memory Garden | `GET/POST /api/transit/memories`, `/api/transit/search`, `/api/debug/memory`, `/api/debug/beliefs` |
| Flow DAG editor | `GET /api/flows/v2/tools`, `/workflows`, `POST /api/flows/v2/run`, SSE `/api/flows/v2/runs/{id}/events` |
| Constitution editor (specialists/orchestrator) | `GET/PUT /api/specialists/{name}`, `GET/PUT /api/teams/{name}`, `GET/PATCH /api/config/agentd` |
| Project workspace context | `GET /api/projects`, `GET /api/projects/{id}/...`, `/api/me/preferences/project` |
| Auth gate | `/auth/login`, `/auth/callback`, `/auth/logout`, `/api/me` |

### 3.2 Additive backend work (new but small)

| Cockpit need | Why current APIs don't suffice | Proposed addition |
|---|---|---|
| Fleet-wide live event stream across **all** sessions for the current user | `/agent/run` is per-request; the cockpit needs to *observe* all currently-running agents | `GET /api/fleet/events` SSE: multiplexed feed of normalized `FleetEvent` (run started/finished, tool call, delegation edge, input_request, error). Implemented as a fan-out subscriber on the existing chat/agent runtime event bus. |
| Snapshot of "what is running right now" | `/api/runs` lists historical runs, not live state | `GET /api/fleet/state` returns `{runs[], specialists[], teams[], openInputRequests[], activeDelegationEdges[]}` derived from in-memory `runStore` + active engines. |
| Trust budget ledger (per-specialist autonomy quota) | Doesn't exist | `GET /api/trust/budgets`, `POST /api/trust/budgets/{specialist}/spend`, `POST /api/trust/budgets/{specialist}/refill`. Persisted in `trust_budgets` table; enforced as a policy hook in `internal/policy`. |
| Constitution (versioned policy doc) | `policy` package and specialist CRUD exist but no versioned document | `GET /api/constitution/versions`, `POST /api/constitution/versions` (immutable), `POST /api/constitution/versions/{id}/activate`. Backed by `constitutions` table; activation is a hot-swap of the policy enforcer's snapshot. |
| Replay cursor for a specific run | Activities and chat messages exist but no single time-ordered stream | `GET /api/runs/{runId}/timeline` returns an ordered event log (already implied by the existing event bus; this just persists or replays it). |
| Counterfactual "fork" of a run | Not currently possible | `POST /api/runs/{runId}/fork` clones session state + history at a given event offset and resumes a new run in a sandbox. (Stretch goal.) |
| OpenAPI-driven typegen | Frontend types are hand-written | Use the existing `/openapi.json` + `openapi-typescript` to generate `frontend/src/api/schema.d.ts` at build time. |

> **Backend additions are strictly additive.** No existing route, handler, or
> SSE event name is changed. All new endpoints follow the same conventions
> (`/api/*`, JSON, optional SSE) and are registered in `internal/agentd/router.go`
> + listed in `internal/apidocs/spec.go`.

---

## 4. Frontend technology choices

Selected to maximize reuse of the team's Vue experience and to align with the
existing Manifold stack so engineers can move between the two UIs without context
loss.

| Layer | Pick | Rationale |
|---|---|---|
| Framework | Vue 3 (Composition API, `<script setup lang="ts">`) | Team fluency; matches existing stack |
| Bundler | Vite 6 | Same as `agentd-ui`; Tauri-ready |
| State | Pinia + TanStack Vue Query | Same as existing UI; minimum cognitive load |
| Realtime collab (stretch) | Yjs + `y-webrtc` / WS provider | For multi-operator triage on the same fleet |
| Canvas / Fleet Map | **PixiJS (WebGL)** with a thin Vue overlay | DOM/SVG breaks past ~2k nodes; PixiJS handles 10k+ |
| Planner DAG | **Vue Flow** (already used by classic UI) | Direct reuse of existing flow-editor mental model; matches Flow v2 |
| Charts | **uPlot** for hi-freq time-series + Observable Plot + custom D3 for halos/heatmaps | uPlot is ~40kb and 60fps on 100k pts |
| Design system | **Reka UI** (ex–Radix Vue) + Tailwind + custom tokens | Accessible primitives without dictating look; matches Tailwind already in repo |
| Code editor | **CodeMirror 6** | Smaller and more composable than Monaco; fits Constitution + prompt editing |
| Markdown / code highlighting | `markdown-it` + `highlight.js` + `dompurify` | Mirror existing UI to keep render-fidelity parity |
| Mermaid | `mermaid` | Already in classic UI; cockpit replay diagrams use it |
| Routing | Vue Router | Same as existing |
| Tests | Vitest + Playwright | Same as existing |
| Lint/format | ESLint + Prettier with the existing project config copied | Consistency |
| Desktop wrapper (later) | Tauri | 30 MB bundle, OS notifications, global hotkeys |
| Type generation | `openapi-typescript` against `/openapi.json` | Single source of truth |

**Explicitly rejected:** Vuetify / Element Plus / PrimeVue (density/aesthetics),
Electron, SSR/Nuxt (operator console behind auth), heavyweight BI embeds.

---

## 5. The seven surfaces, mapped to routes & components

```
/cockpit                        (or '/', depending on build mode)
├── /fleet              FleetMapView         ← Fleet Map (Pixi canvas)
├── /intent             IntentConsoleView    ← decompose goals → run plans
├── /inbox              InterruptInboxView   ← live input_request triage
├── /telemetry          TelemetryView        ← trust + drift + cost dashboards
├── /memory             MemoryGardenView     ← Transit + RAG + beliefs curator
├── /replay             ReplayView           ← scrub a run / counterfactual fork
├── /constitution       ConstitutionView     ← policy + specialists + diff/deploy
└── /settings           SettingsView         ← user prefs, project picker, theme
```

Reuse the **already-present** `/projects`, `/specialists`, `/flow`, `/playground`,
`/codeqa`, `/pulse` data sources, but rendered through the cockpit aesthetic.
We are not duplicating those view files — we render their data through new
components designed for fleet-scale operation.

---

## 6. Directory layout

```
repos/manifold/
├── frontend/                          ← NEW
│   ├── package.json                   (Vue 3 + Vite 6, pnpm)
│   ├── pnpm-lock.yaml
│   ├── tsconfig.json
│   ├── vite.config.ts                 (proxy /api, /agent, /auth, /openapi.json → :32180)
│   ├── tailwind.config.ts
│   ├── postcss.config.cjs
│   ├── index.html
│   ├── playwright.config.ts
│   ├── vitest.config.ts
│   ├── public/
│   │   └── manifold-cockpit.svg
│   ├── src/
│   │   ├── main.ts
│   │   ├── App.vue
│   │   ├── router/index.ts
│   │   ├── api/
│   │   │   ├── client.ts              (axios; baseURL '/api', credentials:'include')
│   │   │   ├── sse.ts                 (typed EventSource helper)
│   │   │   ├── schema.d.ts            (auto-generated from /openapi.json)
│   │   │   ├── fleet.ts
│   │   │   ├── chat.ts                (re-uses event types of classic UI)
│   │   │   ├── specialists.ts
│   │   │   ├── transit.ts
│   │   │   ├── metrics.ts
│   │   │   ├── flow.ts
│   │   │   ├── constitution.ts
│   │   │   └── trust.ts
│   │   ├── stores/                    (Pinia)
│   │   │   ├── fleet.ts               (live topology + edges)
│   │   │   ├── inbox.ts               (open input_requests)
│   │   │   ├── trust.ts               (budgets + drift)
│   │   │   ├── replay.ts              (scrub cursor + branch state)
│   │   │   ├── memory.ts              (Transit/RAG/beliefs cache)
│   │   │   ├── constitution.ts        (versions + diff state)
│   │   │   ├── projects.ts
│   │   │   └── theme.ts
│   │   ├── composables/
│   │   │   ├── useFleetEvents.ts      (SSE → fleet store)
│   │   │   ├── useReplayCursor.ts
│   │   │   ├── useConfidenceHalo.ts
│   │   │   ├── useTrustBudget.ts
│   │   │   ├── useHotkeys.ts          (keyboard-first triage)
│   │   │   ├── useSelection.ts        (lasso-to-task)
│   │   │   └── useStreamingFetch.ts   (port of classic UI's SSE parser)
│   │   ├── views/
│   │   │   ├── FleetMapView.vue
│   │   │   ├── IntentConsoleView.vue
│   │   │   ├── InterruptInboxView.vue
│   │   │   ├── TelemetryView.vue
│   │   │   ├── MemoryGardenView.vue
│   │   │   ├── ReplayView.vue
│   │   │   ├── ConstitutionView.vue
│   │   │   └── SettingsView.vue
│   │   ├── components/
│   │   │   ├── fleet/
│   │   │   │   ├── FleetCanvas.vue            (PixiJS host)
│   │   │   │   ├── fleetRenderer.ts           (imperative WebGL scene)
│   │   │   │   ├── layoutEngine.ts            (force + hierarchy hybrid)
│   │   │   │   ├── AgentNodeOverlay.vue
│   │   │   │   └── EdgeAnimator.ts
│   │   │   ├── inbox/
│   │   │   │   ├── InterruptCard.vue
│   │   │   │   └── InboxRanker.ts             (ranking model, product IP)
│   │   │   ├── replay/
│   │   │   │   ├── TimelineScrubber.vue
│   │   │   │   └── EventDeltaView.vue
│   │   │   ├── halos/
│   │   │   │   └── ConfidenceHalo.vue
│   │   │   ├── memory/
│   │   │   │   ├── MemoryNode.vue
│   │   │   │   └── MemoryGardenCanvas.vue
│   │   │   ├── ui/                            (Reka UI wrappers)
│   │   │   │   ├── Button.vue
│   │   │   │   ├── Dialog.vue
│   │   │   │   ├── DropdownMenu.vue
│   │   │   │   ├── Tabs.vue
│   │   │   │   └── Topbar.vue
│   │   │   └── charts/
│   │   │       ├── UPlotChart.vue
│   │   │       ├── DriftHeatmap.vue
│   │   │       └── BurnRateTile.vue
│   │   ├── lib/
│   │   │   ├── sse.ts
│   │   │   ├── format.ts
│   │   │   └── markdown.ts                    (markdown-it + DOMPurify)
│   │   ├── theme/tokens.css
│   │   └── types/
│   │       ├── fleet.ts
│   │       └── stream.ts                      (copy of ChatStreamEventType + extensions)
│   ├── tests/                                 (Vitest)
│   └── e2e/                                   (Playwright)
│
├── internal/cockpitui/                 ← NEW Go package, mirrors internal/webui
│   ├── assets_embed.go                 (//go:build !dev_cockpit, embeds dist/*)
│   ├── assets_dev.go                   (//go:build dev_cockpit)
│   ├── fs.go
│   ├── handler.go                      (copy of webui handler.go, package renamed)
│   ├── dist/                           (populated by `make frontend-cockpit`)
│   │   └── .gitkeep
│   └── cockpitui_test.go
│
├── internal/agentd/server_lifecycle.go ← MODIFIED (frontend selector)
├── internal/agentd/router.go           ← MODIFIED (registers new /api/fleet/*, /api/trust/*, /api/constitution/*)
│
├── internal/fleet/                     ← NEW domain package
│   ├── bus.go                          (pub/sub for live agent events)
│   ├── snapshot.go                     (state aggregator)
│   ├── service.go
│   └── service_test.go
│
├── internal/trust/                     ← NEW domain package
│   ├── ledger.go
│   ├── store.go                        (Postgres + memory backends)
│   └── service.go
│
├── internal/constitution/              ← NEW domain package
│   ├── document.go
│   ├── store.go
│   ├── service.go
│   └── activator.go                    (hot-swap of policy snapshot)
│
├── internal/agentd/
│   ├── handlers_fleet.go               ← NEW
│   ├── handlers_trust.go               ← NEW
│   └── handlers_constitution.go        ← NEW
│
├── Makefile                            ← MODIFIED (new targets, see §8)
└── docs/cockpit-frontend-plan.md       ← THIS FILE
```

> The classic `web/agentd-ui` and `internal/webui` are **untouched**. The new
> Go package `internal/cockpitui` is a sibling so the two UIs are independently
> embeddable and cannot accidentally collide on `//go:embed` paths.

---

## 7. Backend changes (minimal & strictly additive)

### 7.1 New package: `internal/fleet`

Purpose: a single in-process pub/sub that normalizes existing agent/chat/flow
events into a `FleetEvent` and exposes them as a multiplexed SSE feed. Built on
top of the existing engine callbacks (`OnDelta`, `OnTool`, etc. — already used in
`internal/agentd/chat_execution.go`).

```go
package fleet

type EventKind string

const (
    EvtRunStarted    EventKind = "run_started"
    EvtRunFinished   EventKind = "run_finished"
    EvtToolStart     EventKind = "tool_start"
    EvtToolResult    EventKind = "tool_result"
    EvtDelegation    EventKind = "delegation"
    EvtInputRequest  EventKind = "input_request"
    EvtError         EventKind = "error"
)

type Event struct {
    Kind         EventKind         `json:"kind"`
    RunID        string            `json:"run_id"`
    SessionID    string            `json:"session_id,omitempty"`
    Specialist   string            `json:"specialist,omitempty"`
    CallID       string            `json:"call_id,omitempty"`
    ParentCallID string            `json:"parent_call_id,omitempty"`
    Depth        int               `json:"depth,omitempty"`
    UserID       int64             `json:"-"`
    At           time.Time         `json:"at"`
    Data         map[string]any    `json:"data,omitempty"`
}

type Bus interface {
    Publish(Event)
    Subscribe(ctx context.Context, userID int64) <-chan Event
}
```

**Where it plugs in:** `chat_execution.go::executeAgent...` and
`flow_v2_runtime.go::Run...` already construct callbacks; we wrap those to also
`bus.Publish(...)`. Zero impact on existing consumers because publishing is
side-effecting.

### 7.2 New handlers

| Route | Behavior |
|---|---|
| `GET /api/fleet/state` | Snapshot of active runs, open input requests, current delegation edges; aggregated from `runStore`, `inputRequests`, `fleet.Bus` recent buffer. |
| `GET /api/fleet/events` (SSE) | Live multiplexed `FleetEvent` stream scoped to the current user. |
| `GET /api/trust/budgets` / `POST /api/trust/budgets/{name}/...` | Trust ledger CRUD (see §7.3). |
| `GET /api/constitution/versions` / `POST .../activate` | Versioned policy doc (see §7.4). |
| `GET /api/runs/{id}/timeline` | Ordered, replayable event log for one run, built from persisted activities + (optional) replay log. |

All registered in `internal/agentd/router.go` and listed in
`internal/apidocs/spec.go` so they show up in `/openapi.json`.

### 7.3 Trust ledger (`internal/trust`)

* `trust_budgets(name TEXT PRIMARY KEY, quota INTEGER, spent INTEGER, refilled_at TIMESTAMPTZ)` Postgres table; memory fallback.
* Wired as a **policy hook** through the existing `internal/policy` package: before tool dispatch, the engine asks the trust ledger whether the specialist may spend an action. If quota is exhausted, the call becomes an `input_request` rather than a refusal — the operator can top up or approve.
* This is the one piece that **changes the agent loop**, but only via the existing policy hook surface; no new injection point is needed.

### 7.4 Constitution (`internal/constitution`)

* `constitutions(id UUID PRIMARY KEY, version INTEGER, body TEXT, created_at TIMESTAMPTZ, created_by INTEGER, active BOOLEAN)`.
* On `activate`, the service publishes the new policy snapshot to the policy enforcer (single-writer, atomic pointer swap). Specialists/teams that pin a version stay pinned; un-pinned ones use `active`.
* Diff/deploy semantics in the UI are powered by client-side diff against the JSON body returned by `GET /api/constitution/versions/{id}`.

### 7.5 What we are NOT changing

* No SSE event names or fields renamed.
* No removal of any current route.
* No persistence-schema changes to existing tables.
* No change to `web/agentd-ui` source code.
* No change to default behavior of `make build-manifold` (still embeds classic UI).
* No change to the agent engine's run loop other than wiring through the additive policy hook (which already exists).

---

## 8. Build & embedding

### 8.1 Selector strategy

The current `internal/webui` package owns the SPA mount at `/`. We add a sibling
package `internal/cockpitui` so we have two independently embeddable SPAs. The
mount logic lives in `internal/agentd/server_lifecycle.go::registerFrontend`,
which becomes a thin selector driven by **Go build tags** plus an environment
variable for dev:

```go
// internal/agentd/server_lifecycle.go (modified)
func (a *app) registerFrontend(mux *http.ServeMux) error {
    devProxy := os.Getenv("FRONTEND_DEV_PROXY")
    opts := frontend.Options{DevProxy: devProxy, AuthGate: a.authGate(), UnauthedRedirect: "/auth/login"}
    return frontend.Register(mux, opts) // resolved by build tags
}
```

```go
// internal/agentd/frontend_classic.go
//go:build !cockpit
package agentd
import "manifold/internal/webui"
type frontendImpl = webui.Options
var frontendRegister = webui.RegisterFrontend
```

```go
// internal/agentd/frontend_cockpit.go
//go:build cockpit
package agentd
import "manifold/internal/cockpitui"
type frontendImpl = cockpitui.Options
var frontendRegister = cockpitui.RegisterFrontend
```

Why build tags instead of a runtime flag: avoids embedding 2× the SPA in every
binary, keeps `agentd` size sane, and matches the existing `dev` build-tag
pattern in `internal/webui/assets_dev.go`.

> **Alternative considered:** mount classic at `/` and cockpit at `/cockpit` in
> the same binary. Rejected for v1 because the cockpit needs to own the auth
> redirect target and we don't want two SPAs racing for the not-found fallback.
> A future flag `MANIFOLD_UI=both` can lift this once both UIs are stable.

### 8.2 New Make targets

Added to `Makefile`:

```makefile
COCKPIT_DIR        := frontend
COCKPIT_SRC_DIST   := $(COCKPIT_DIR)/dist
COCKPIT_EMBED_DIR  := internal/cockpitui/dist
COCKPIT_FEATURE_GATE ?= stable

.PHONY: frontend-cockpit
frontend-cockpit:
	@command -v $(PNPM) >/dev/null 2>&1 || { echo "pnpm not found"; exit 1; }
	@echo "Installing cockpit deps in $(COCKPIT_DIR)"
	cd $(COCKPIT_DIR) && $(PNPM) install --frozen-lockfile
	@echo "Generating cockpit API types from /openapi.json"
	cd $(COCKPIT_DIR) && $(PNPM) run codegen
	@echo "Building cockpit (feature gate: $(COCKPIT_FEATURE_GATE))"
	cd $(COCKPIT_DIR) && VITE_MANIFOLD_FEATURE_GATE=$(COCKPIT_FEATURE_GATE) $(PNPM) run build
	@[ -d "$(COCKPIT_SRC_DIST)" ] || { echo "cockpit build missing"; exit 1; }
	rm -rf $(COCKPIT_EMBED_DIR) && mkdir -p $(COCKPIT_EMBED_DIR)
	cp -R $(COCKPIT_SRC_DIST)/. $(COCKPIT_EMBED_DIR)/

.PHONY: build-cockpit
build-cockpit: frontend-cockpit | $(DIST)
	@echo "Building agentd with embedded cockpit UI -> $(DIST)/agentd-cockpit"
	go build -tags cockpit -o $(DIST)/agentd-cockpit ./cmd/agentd

.PHONY: build-cockpit-beta
build-cockpit-beta: COCKPIT_FEATURE_GATE := beta
build-cockpit-beta: build-cockpit

.PHONY: dev-cockpit
dev-cockpit:
	@echo "Run two terminals:"
	@echo "  1) cd $(COCKPIT_DIR) && pnpm dev   (serves on :32181)"
	@echo "  2) FRONTEND_DEV_PROXY=http://localhost:32181 go run -tags cockpit ./cmd/agentd"
```

* `make build-cockpit` produces `dist/agentd-cockpit`, a binary that serves the cockpit at `/`.
* `make build-manifold` continues to produce `dist/agentd` with the classic UI unchanged.
* `make dev-cockpit` documents the dev-proxy flow (re-using existing `FRONTEND_DEV_PROXY` plumbing).

### 8.3 OpenAPI codegen step

A `frontend/scripts/codegen.ts` script runs `openapi-typescript` against
`http://localhost:32180/openapi.json` (or the static `docs/openapi/openapi.json`
in CI) and writes `src/api/schema.d.ts`. Keeps wire types in sync without
hand-maintaining duplicates.

---

## 9. Streaming & realtime architecture

### 9.1 Wire choices

| Surface | Transport | Why |
|---|---|---|
| Fleet event multiplex | **SSE** (`/api/fleet/events`) | One-way, auto-reconnect, cheapest; matches Manifold's existing SSE style |
| Run-scoped chat / agent run | Existing SSE on `/agent/run` | Unchanged — cockpit consumes the same events |
| Flow run | Existing SSE on `/api/flows/v2/runs/{id}/events` | Unchanged |
| Input-request resolution | `POST` (existing) | Already correct |
| Bidirectional collaboration (stretch) | **WebSocket** + Yjs awareness | Only needed for multi-operator |
| Telemetry tiles | Polling (TanStack Query) + SSE for "hot" tiles | Polling is fine for 1-second tiles; SSE for fleet burn rate |

### 9.2 Client-side event model

* Cockpit normalizes every incoming event into the **same** internal `FleetEvent`
  shape (mirroring the new server type). Three SSE consumers:
  1. global `/api/fleet/events` → `fleetStore`
  2. per-run `/agent/run` (when the user opens a single run) → `replayStore`
  3. per-workflow `/api/flows/v2/runs/{id}/events` → `replayStore`
* Yjs (optional, behind `cockpit.collab` feature gate) syncs only operator state
  (selection, cursor, comments) — never server-truth state.

---

## 10. Visual / UX system

* Dark-first; "Cockpit", "Garden", and "Theatre" themes selectable in settings (UI roles, not separate apps).
* Density profile is the inverse of the classic UI: smaller padding, more affordances visible at once, keyboard-driven.
* Hotkey layer (via `useHotkeys`) for triage: `A` approve, `R` reject, `?` help, `g f` go fleet, `g i` inbox, `[` `]` scrub timeline, etc.
* Accessibility: all interactive primitives via Reka UI, color tokens are WCAG-AA at default density.
* Two render modes for Fleet Map: **WebGL** (default) and **DOM fallback** (for ≤200 nodes / a11y).

---

## 11. Testing strategy

| Layer | Tool | What it covers |
|---|---|---|
| Pure logic (composables, ranking, layout) | Vitest | Inbox ranker, replay cursor math, layout engine determinism |
| Components | Vitest + @testing-library/vue | Reka UI wrappers, halo primitive, timeline scrubber |
| WebGL renderer | Vitest + headless Pixi mock | Scene diff on event ingestion |
| API client | MSW or recorded fixtures captured from a live agentd | Type-safe stubs from `schema.d.ts` |
| E2E | Playwright | Login → start a chat-as-run → cockpit shows topology → approve an input_request |
| Backend (Go) | `go test ./internal/fleet/... ./internal/trust/... ./internal/constitution/... ./internal/agentd/...` | Unit + handler tests with `httptest` |
| Contract | An integration test that asserts every cockpit SSE consumer recognizes every server event name | Locks the stable wire format |

CI: extends `.github/workflows` to also `make frontend-cockpit` and run
`pnpm test:unit` + `pnpm test:e2e` against a binary built with `-tags cockpit`.

---

## 12. Migration & coexistence

* **v0:** Cockpit binary builds and runs locally. Classic UI is the default
  production binary. No code in `web/agentd-ui` or `internal/webui` is changed.
* **v1:** Cockpit reaches parity for "monitor + triage" workflows. We document
  it in `README.md` and expose `make build-cockpit` as an officially-supported
  build. Users who opt in run `agentd-cockpit`.
* **v2:** Add optional dual-mount mode (`-tags cockpit,classic` with cockpit at
  `/cockpit`) for organizations that want both UIs in one process. Requires
  resolving the auth-redirect ambiguity (probably via a config setting).
* **v3 (long horizon):** If cockpit fully supersedes the classic UI, the classic
  UI is moved behind a feature flag and eventually retired. This plan does **not**
  schedule retirement; it preserves optionality.

---

## 13. Risks & mitigations

| Risk | Mitigation |
|---|---|
| Embedding two SPAs balloons binary size | Build tags select exactly one at a time. |
| SSE fan-out (`/api/fleet/events`) becomes a hot spot | Buffered channel per subscriber, drop-oldest policy, cap subscribers per user; identical pattern to `chat_activity_collector.go`. |
| Trust ledger introduces a regression in the agent loop | The ledger is a no-op when quota is unlimited (default). Feature gate `MANIFOLD_TRUST_ENABLED`. Tests assert byte-for-byte parity on existing run flows when disabled. |
| Constitution hot-swap races with in-flight runs | Activation captures a snapshot; in-flight runs keep their pinned snapshot until they complete. |
| WebGL renderer breaks on older GPUs | DOM fallback renderer flips on automatically when `WebGLRenderingContext` is unavailable, capped at 200 nodes. |
| Type drift between Go and TS | `make codegen` runs on every cockpit build; CI fails if `schema.d.ts` is stale. |
| Auth flow differs between two SPAs | Both packages share `internal/webui`-style `AuthGate` semantics; the cockpit's `Options.UnauthedRedirect` defaults to `/auth/login`, identical to classic. |
| Path-containment regressions when adding new endpoints | New handlers use the same `internal/sandbox` and `internal/projects` helpers; covered by handler tests. |

---

## 14. Phased delivery plan

### Phase 0 — Foundations (1 sprint)

* Scaffold `frontend/` (Vue 3 + Vite + TS + Tailwind + Pinia + Vue Router + Reka UI).
* Add `internal/cockpitui` (copy of `internal/webui` with package rename).
* Add `make frontend-cockpit` / `make build-cockpit` / `make dev-cockpit`.
* Wire build tags `cockpit` in `internal/agentd/server_lifecycle.go` (no other backend changes yet).
* CI green for `build-cockpit` on a placeholder `App.vue`.
* `make build-manifold` is bit-for-bit identical to before (asserted in CI).

### Phase 1 — Read-only cockpit (2 sprints)

* `frontend/src/api/` clients for: specialists, teams, runs, chat sessions, projects, metrics, transit, beliefs, flow.
* Telemetry view (uPlot tiles for tokens, burn, drift) — pure read.
* Memory Garden view (Transit + RAG + beliefs browser).
* Settings view (theme, project picker via `/api/me/preferences`).
* No backend changes yet.

### Phase 2 — Live fleet (2 sprints)

* `internal/fleet` package: `Bus`, `Snapshot`, handlers `GET /api/fleet/state`, `GET /api/fleet/events`.
* Wire `fleet.Bus.Publish` into the existing chat/agent/flow callback sites.
* Fleet Map view (PixiJS renderer + layout engine + Vue overlay).
* Interrupt Inbox view consuming `input_request` events.
* Approve/reject hotkeys land here.

### Phase 3 — Replay (1 sprint)

* `GET /api/runs/{id}/timeline` (built from persisted chat activities + recent fleet buffer).
* Replay view with `TimelineScrubber.vue` and `EventDeltaView.vue`.
* Vitest coverage for cursor logic.

### Phase 4 — Trust + Constitution (2 sprints)

* `internal/trust` ledger + handlers.
* Wire trust check into policy enforcer; default quota = unlimited.
* `internal/constitution` versioned doc + handlers + activator.
* Constitution view (CodeMirror 6 diff + activate).
* Trust budgets visible in Fleet Map (halo color encodes spent/quota).

### Phase 5 — Intent + Counterfactuals (stretch)

* Intent Console: drag-on-canvas plan editor that compiles to Flow v2 workflows.
* `POST /api/runs/{id}/fork` for counterfactual branches.
* Yjs-based multi-operator awareness (selection + cursor + comments).

### Phase 6 — Polish & adoption

* Tauri desktop wrapper.
* Production hardening (rate limits, subscriber caps, OTel spans on new handlers).
* README & docs updated; cockpit promoted from beta to stable build target.

Total: roughly **8 sprints (~16 weeks) for a beta-quality cockpit**, of which
~6 are frontend-heavy and ~2 are concentrated Go work in the additive packages.

---

## 15. Decisions that need user confirmation

Before Phase 0 starts we should confirm:

1. **Binary naming:** `dist/agentd-cockpit` vs reusing `dist/agentd` with a build tag. Recommendation: keep both binaries so the classic build is never accidentally clobbered.
2. **Default mount path:** cockpit at `/` (replaces classic in that binary) vs at `/cockpit` (cohabits with classic). Recommendation: `/` for v1, single-SPA-per-binary; coexistence in v2 if demanded.
3. **Trust ledger default:** off (`MANIFOLD_TRUST_ENABLED=false`) until policy semantics are agreed.
4. **Type generation:** generate from `/openapi.json` at build time, or check generated `schema.d.ts` into the repo? Recommendation: check it in and verify in CI.
5. **Where to retire the legacy UI:** out of scope for this plan; only ensure optionality.

---

## 16. One-line summary

We add a sibling Vue 3 cockpit under `frontend/`, a sibling Go embed package
`internal/cockpitui/`, three additive backend packages (`fleet`, `trust`,
`constitution`), and a `make build-cockpit` target. The classic UI and every
existing wire contract remain unchanged.
