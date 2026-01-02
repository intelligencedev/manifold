# Prompt Cards — Engineering Proposal

> Status: **Draft (incremental)**

## 1) Summary

We want a new **Prompt Cards** feature in `manifold/web/agentd-ui` that lets users:

- Create reusable **prompt cards** (small prompt building blocks).
- **Tag** and **categorize** prompt cards for fast retrieval.
- Drag cards from a **palette** into a **canvas**.
- Arrange/“connect” cards into an ordered **sequence** that composes into a larger prompt.
- Save the composed sequence as:
  1) a **new prompt card** (appears in the palette), and/or
  2) an **exported versioned prompt**, using the same versioning model as the Playground prompt system.

This proposal is written after initial codebase research (backend + frontend). It will be expanded iteratively.

---

## 2) Codebase Research (What Exists Today)

### 2.1 Frontend: Vue app structure + styling conventions

- Frontend lives in `manifold/web/agentd-ui` (Vue 3 + TypeScript + Tailwind).
- Styling convention is a Tailwind-first approach with theme tokens like:
  - `bg-surface`, `bg-surface-muted/60`, `border-border/70`, `text-subtle-foreground`, etc.
  - Utility “components” via classes like `ap-panel`, `ap-input`, `ap-chip`, `ap-hairline-b`.
- Theme definitions live in `src/assets/aperture.css`.

### 2.2 Frontend: Existing “versioned prompt” UX (Playground)

Relevant files:

- `src/views/playground/PlaygroundPromptsView.vue`
  - Create prompt (name/description/tags).
  - List prompts + filter.
- `src/views/playground/PlaygroundPromptDetailView.vue`
  - Shows prompt summary + tags.
  - Lists versions (newest first).
  - Create a new version (semver, template, variables JSON, guardrails JSON).

This is the reference UX + API usage for “Export as versioned prompt”.

### 2.3 Frontend: Existing drag/drop canvas + palette pattern (FlowView)

Relevant file:

- `src/views/FlowView.vue`

Key takeaways:

- Uses **VueFlow** (`@vue-flow/core`, `@vue-flow/background`, `@vue-flow/minimap`).
- Implements a robust **palette → canvas** DnD mechanism using HTML5 drag data:
  - `DRAG_DATA_TYPE = 'application/warpp-tool'`
  - `onPaletteDragStart` sets data transfer payload.
  - `onDrop` projects screen coordinates into VueFlow coordinates (`useVueFlow().project`).
- Provides a strong, app-consistent layout:
  - Left panel switches between palette and inspector.
  - Center is the canvas.
  - Top toolbar for primary actions.

This pattern is directly reusable for Prompt Cards.

### 2.4 Backend: Existing prompt + prompt version model

Relevant packages/files:

- `internal/playground/registry/registry.go`
  - `registry.Prompt` includes:
    - `Name`, `Description`, `Tags []string`, `Metadata map[string]string`
  - `registry.PromptVersion` includes:
    - `Template string`, `Variables map[string]VariableSchema`, `Guardrails`, `ContentHash`
- `internal/httpapi/server.go` and `internal/httpapi/handlers.go`
  - Endpoints exist for prompts + versions:
    - `GET /api/v1/playground/prompts`
    - `POST /api/v1/playground/prompts`
    - `GET /api/v1/playground/prompts/{promptID}`
    - `DELETE /api/v1/playground/prompts/{promptID}`
    - `GET /api/v1/playground/prompts/{promptID}/versions`
    - `POST /api/v1/playground/prompts/{promptID}/versions`
- Notably **missing today**:
  - No `PUT/PATCH /api/v1/playground/prompts/{promptID}` (cannot rename/change tags/metadata after creation).
    - This matters for prompt cards because category/composition metadata likely needs to evolve.
- Persistence:
  - `internal/persistence/databases/playground_store.go` persists prompts + versions in Postgres JSONB tables.

Important: **Prompts already support tags and metadata**, and prompt versions support templates and variables.
This is a strong foundation for implementing prompt cards with minimal new backend surface area.

Implementation caveat discovered:

- Backend template validation (`registry.ValidateTemplate`) treats `{{ name }}` and `{{name}}` equivalently (it trims whitespace).
- Runtime rendering (`worker.renderTemplate`) only replaces the exact token `{{name}}` (no whitespace).
  - If a template uses whitespace inside braces, it may **pass validation but fail at runtime**.
  - Prompt Cards should either enforce a no-whitespace placeholder style in the UI, or we should harden backend rendering.

---

## 3) Product/UX Goals

### 3.1 Primary goals

1. **Fast reuse**: Users can build a library of prompt cards and recompose them quickly.
2. **Obvious UX**: The palette/canvas interaction should be discoverable without documentation.
3. **Composable prompt output**: The system produces a deterministic combined prompt (and variables) from an ordered sequence.
4. **Save paths**:
   - Save composition as a new prompt card.
   - Export composition as a versioned Playground prompt.

### 3.2 Non-goals (initially)

- Full graph semantics (branching, conditional edges, cycles).
- Runtime execution of the composed prompt (beyond exporting into the existing Playground execution flows).
- Collaborative editing / multi-user real-time.

---

## 4) Key Design Decision: Custom Canvas vs VueFlow

### Recommendation

Use **VueFlow** for the Prompt Cards canvas, but with a **purpose-built Prompt Cards UI**.

Rationale:

- VueFlow is already in the project (`FlowView.vue`) and provides panning/zooming, node selection, hit-testing, and DnD coordinate projection.
- We can constrain the interaction model to a **linear sequence** (left-to-right) while still using VueFlow’s rendering + interactions.
- We reuse the existing app’s proven palette drag/drop patterns and styling.

We will *not* reuse the Warpp node types/UI; we will create dedicated PromptCard nodes and inspectors.

### Alternative: no VueFlow (not recommended)

A custom canvas could be built with native drag/drop and absolute positioning, but would require re-implementing:

- panning/zooming
- selection + hit-testing
- keyboard interactions
- edge rendering
- minimap (optional)

Given VueFlow is already a dependency and FlowView demonstrates an app-consistent implementation, VueFlow is the pragmatic high-quality choice.

---

## 5) Proposed Data Model (Draft)

### 5.1 Prompt Card representation

We can represent a prompt card as a **Playground prompt** with a marker in `metadata`:

- `Prompt.metadata.kind = "prompt_card"`
- `Prompt.metadata.category = "…"` (free-form string, optionally hierarchical like `"Sales/Outbound"`)

Note on shape:

- In both backend and frontend today, `metadata` is a **string map** (`map[string]string` / `Record<string,string>`).
- This works well for `kind` and `category`.
- It is **not** a great place for deeply structured composition graphs unless we JSON-encode into a string.

Prompt card content lives in the **latest prompt version template**.

Pros:

- Reuses existing persistence + API + ownership model.
- Prompt cards can be versioned later “for free” (same prompt version model).

Cons / caveats:

- `GET /playground/prompts` does not server-filter by metadata kind today; client filtering is possible but may not scale.
- Palette previews may require fetching latest versions for cards on demand.

### 5.2 Composition document

The canvas composition should be serializable so it can be:

- saved as part of a prompt card (metadata or as a special prompt version payload), and/or
- re-opened later.

Draft composition schema (frontend-level):

```ts
type PromptCardRef = {
  promptId: string           // points to a Prompt (kind=prompt_card)
  versionId?: string         // optional: pin to specific version
}

type PromptCardInstance = {
  id: string                 // canvas node id
  ref: PromptCardRef
  position: { x: number; y: number }
}

type PromptCardComposition = {
  id: string
  name: string
  description?: string
  tags: string[]
  category?: string
  createdAt: string
  nodes: PromptCardInstance[]
  edges: { from: string; to: string }[] // expected to form a simple chain
}
```

We can keep edges implicit (a strict order array) in v1, and only store edges if we decide to allow user-created links.

Persistence options (impacted by metadata being string-only):

- MVP: do not persist compositions (flatten to template when saving as a new card).
- If we persist in prompt metadata: store a JSON string under a key like `metadata.composition_json`.
- Preferred: new backend table/entity for compositions (JSONB) similar to `warpp_workflows.doc`.

---

## 6) Proposed UI: Prompt Cards Studio View (Draft)

**Layout (consistent with FlowView):**

- Top toolbar: primary actions
- Left panel: Prompt Card Palette + filters
- Center: canvas
- Right panel (or left panel toggle): inspector + live composed output

### 6.1 Prompt Card Palette

- Search by name/description.
- Filter chips:
  - Category dropdown (grouped list derived from prompt card metadata).
  - Tag filter (multi-select, derived from tags).
- “New Card” button opens a small modal/editor.
- Card rows are draggable; each row shows:
  - name
  - category (subtle)
  - tags (optional, compact)
  - description preview

### 6.2 Canvas (sequence-focused)

- Drag cards into canvas to create nodes.
- Nodes snap into a left-to-right sequence lane.
- Visual edges indicate the current order.
- Reorder by dragging nodes left/right.
- Selecting a node shows details in the inspector.

Sequence modeling choice (MVP):

- Each canvas node stores an explicit `order` in `node.data.order`.
- Visual edges are derived from `order` (connect i → i+1).
- Reordering updates `order` and then auto-repositions nodes to a tidy lane.

This mirrors `FlowView.vue` where steps maintain an `order` field and are sorted via `getOrderedStepNodes()`.

### 6.3 Inspector + Output

- Selected node shows:
  - card metadata (name/category/tags)
  - template preview (read-only by default)
  - “Edit Card” action
- Output pane shows:
  - composed prompt template
  - merged variable schema summary
  - warnings for variable conflicts

### 6.4 Save / Export

- **Save as prompt card**:
  - Creates a new Prompt with `metadata.kind=prompt_card` and the chosen category.
  - Creates a PromptVersion with template = composed template.
  - Adds tags.
- **Export as versioned prompt**:
  - Creates a new Prompt (normal kind) OR uses an existing prompt (optional future enhancement).
  - Creates a PromptVersion with template = composed template.
  - Navigates user to `PlaygroundPromptDetailView` for the created prompt.

---

## 11) VueFlow Sequence Enforcement (Design)

We want a canvas that *feels* like arranging cards in a line, not building an arbitrary graph.

### 11.1 Data model

- Canvas nodes use VueFlow nodes.
- Each node’s `data` includes:
  - `order: number`
  - `card: { promptId: string; versionId?: string }`

Edges are **derived**, not user-authored:

- `edges = [ {source: node(order=i).id, target: node(order=i+1).id} ]`

### 11.2 Reordering algorithm

Primary interactions:

- Drag a card from palette → append to end of sequence.
- Drag an existing node left/right → reorder.

Recommended implementation:

1. On node drag end, sort nodes by `position.x` (tie-break by `position.y`).
2. Reassign `data.order = index`.
3. Snap nodes onto a lane:
   - fixed `y` (with slight offsets allowed)
   - `x = startX + index * gap`
4. Recompute edges from order.

### 11.3 UX constraints

- Disable manual edge creation entirely (canvas is sequence-only).
- Provide explicit “Add before/after” affordances on node hover (optional nice-to-have).
- Provide keyboard shortcuts:
  - Delete removes selected node and closes the gap.

---

## 12) Variable Merge + Template Safety (Design)

### 12.1 Variable merge

Source of truth:

- Each card references a `PromptVersion` that may define `variables`.

Composition merge:

- `mergedVariables = union(card.variables...)`.
- Conflicts produce warnings:
  - same variable name but different `type`/`required`/`description`.

MVP conflict policy:

- `required`: OR across definitions.
- `type`: if mismatch → set `"string"` and warn.
- `description`: prefer the first non-empty.

### 12.2 Placeholder style enforcement

Because backend validation and runtime rendering disagree on whitespace-inside-braces, enforce a strict style at authoring time:

- `{{name}}` only (no internal whitespace).
- Linter runs on save/export; blocks action if any placeholder contains whitespace.

### 12.3 Server-side validation guarantees

- Saving/exporting will call the existing prompt version endpoint.
- Backend will compute `ContentHash` and validate template placeholders against variables.
- If the backend rejects the version, surface the server error in the UI.

---

## 13) Frontend Architecture (Proposed)

### 13.1 New route + view

- Add a new route `/prompt-cards` rendering:
  - `src/views/PromptCardsView.vue` (new)
- Add a top-nav item in `src/App.vue` navigation list (similar to Flow).

### 13.2 Recommended component/module layout

Create a focused feature directory:

```
src/
  views/
    PromptCardsView.vue
  components/
    promptCards/
      PromptCardPalette.vue
      PromptCardPaletteItem.vue
      PromptCardInspector.vue
      PromptCardOutputPane.vue
      PromptCardNode.vue
      PromptCardCreateModal.vue
  stores/
    promptCards.ts
  types/
    promptCards.ts
```

Notes:

- Keep Prompt Cards UI components separate from WARPP Flow components so the UX can be purpose-built.
- It’s acceptable to reuse low-level primitives/patterns (e.g., `DropdownSelect`) to stay consistent.

### 13.3 State management

The codebase uses both Pinia and Vue Query:

- Playground uses Pinia (`usePlaygroundStore`).
- Other areas (Specialists) use Vue Query.

Recommendation:

- Use **Pinia** for Prompt Cards (`usePromptCardsStore`) to align with Playground prompt/version interactions and to simplify caching versions by promptId.

Store responsibilities:

- Load prompt cards list (via `listPrompts`, client filter).
- Cache prompt versions per promptId.
- Maintain current canvas state (nodes/order).
- Provide computed “composed output” (template + variables + warnings).
- Handle save/export flows.

---

## 14) Backend Enhancements (Recommended)

### 14.1 PATCH prompt metadata (high leverage)

Add:

- `PATCH /api/v1/playground/prompts/{promptID}`

Why it matters:

- Prompt Cards need editable `metadata.category` and (eventually) `metadata.composition_json`.
- It improves existing Playground UX as well (rename prompts, edit tags, etc.).

### 14.2 GET prompt version by ID (nice-to-have)

Today, the backend has `Service.GetPromptVersion`, but the HTTP API does not expose it.

Add:

- `GET /api/v1/playground/prompt-versions/{versionID}`

This enables:

- pinning a composition node to an explicit version without fetching all versions for the prompt.

### 14.3 Fix template whitespace mismatch (bugfix)

Backend currently:

- Validates placeholders with whitespace tolerated.
- Renders placeholders with whitespace **not** tolerated.

Fix options:

1. Harden `worker.renderTemplate` to replace `{{ name }}` as well as `{{name}}`.
2. Introduce a shared placeholder parser used by both registry validation and worker rendering.

Prompt Cards UI can mitigate, but backend correctness is preferred.

---

## 15) Testing Strategy

The repo already tests complex VueFlow UI via `tests/views/flowview.spec.ts` (Testing Library + Vitest).

Recommended Prompt Cards tests:

1. **Palette rendering + filtering**
   - Mock `listPrompts()` response with a mix of prompts and prompt_cards.
   - Assert category/tag filters narrow the list.
2. **Drag/drop creates nodes**
   - Simulate dragstart with a promptId payload.
   - Simulate drop onto VueFlow wrapper.
   - Assert node count increments.
3. **Reorder updates composed output**
   - Add two cards with distinct templates.
   - Reorder nodes and assert composed template order flips.
4. **Save/export integration**
   - Mock `createPrompt` + `createPromptVersion`.
   - Assert correct payloads:
     - metadata.kind/category on save-as-card
     - normal prompt export

---

## 16) Rollout + Migration Notes

- Prompt cards are implemented as normal prompts with metadata markers.
- No migration is required.
- If we later add server-side filtering or dedicated endpoints, we can keep the same underlying data.


---

## 7) Open Questions / To Research Next

1. Do we want prompt cards to be **versioned** from day 1 in the UI, or treat them as single-template artifacts (still backed by PromptVersion under the hood)?
2. Where should the Prompt Cards view live in navigation?
   - top-level route (e.g., `/prompt-cards`), or
   - under Playground (e.g., `/playground/prompt-cards`).
3. Should composition edges be user-editable, or strictly an ordered list (simpler + clearer)?
4. Do we need backend support for:
   - filtering prompt cards by kind/category/tag server-side,
   - returning latest version preview for palette,
   - storing/re-opening composition graphs.

---

## 8) Implementation Plan (Draft, phased)

### Phase 0 — “No new backend endpoints” MVP

Goal: deliver Prompt Cards Studio with minimal backend changes by reusing the Playground prompt APIs.

Frontend-only approach:

1. **Prompt cards are prompts**
   - When creating a prompt card, we create a `Prompt` via `POST /v1/playground/prompts` and set:
     - `tags` (existing field)
     - `metadata.kind = "prompt_card"`
     - `metadata.category = "..."`
2. **Prompt card content is a prompt version**
   - Use `POST /v1/playground/prompts/:id/versions` to store the card’s template.
3. **Prompt cards palette loading**
   - Use `listPrompts()` and client-filter to `metadata.kind === 'prompt_card'`.
   - On-demand load latest versions when:
     - user selects a card, or
     - user drags it onto canvas, or
     - user opens the output pane.
4. **Composition export**
   - Export uses the same `createPrompt` + `createPromptVersion` path as Playground.
   - After export, route to `/playground/prompts/:promptId`.

Implementation detail (frontend API base):

- The UI calls Playground endpoints via Axios `apiClient` with baseURL `VITE_AGENT_API_BASE_URL || '/api'`.
- Playground routes are under `/api/v1/...` on the Go server.
- Therefore frontend calls `/v1/playground/...` and relies on the `/api` prefix from the client baseURL.

Tradeoffs (acceptable for MVP):

- Palette filtering is client-side.
- Category values are free-form and derived from metadata.
- No durable composition storage beyond saving as a new prompt card.

### Missing capability to account for

The Playground backend currently lacks an update endpoint for prompts (no `PUT/PATCH /api/v1/playground/prompts/{id}`), which affects prompt cards:

- We **cannot** change a card’s category, tags, or composition metadata after creation.

Therefore, Phase 0 should treat “Edit card metadata” as:

- either “create a new card” (soft-duplicate), or
- a Phase 1 backend enhancement.

### Phase 1 — Dedicated Prompt Cards API (optional)

If scale/performance warrants it, add backend endpoints:

- `GET /api/v1/prompt-cards` (server-side filter by metadata.kind)
- `GET /api/v1/prompt-cards/:id` (include latest version)
- `POST /api/v1/prompt-cards` (create prompt + initial version in one request)

Also recommended in Phase 1 (even if we do not add a separate prompt-cards API):

- `PATCH /api/v1/playground/prompts/{promptID}` to update:
  - `name`, `description`, `tags`, `metadata`
  - This would immediately improve UX for both prompts and prompt cards.

This is largely a convenience/optimization layer over existing persistence.

### Phase 2 — Persist compositions as compositions

If we want users to reopen and keep editing compositions as compositions (not just as flattened prompt templates), we should store the composition graph explicitly:

- Option A: store serialized JSON in `Prompt.metadata.composition` (quick but may be size-limited / awkward).
- Option B: introduce a new backend entity/table `prompt_card_compositions` keyed by user_id.

Refinement:

- Because prompt metadata is a `map[string]string`, Option A should be understood as:
  - store a JSON string in `Prompt.metadata["composition_json"]`.

The recommended long-term approach is **Option B**.

---

## 9) Detailed Design: Composing Templates + Variables (Draft)

### 9.1 Composed prompt template

Given an ordered list of cards `C1..Cn`, each card has a prompt template `Ti`.

We define the composed template as:

- `T = join(T1, T2, ..., Tn, separator)`

Separator rules (MVP):

- Use a deterministic separator that preserves readability:

```
\n\n---\n\n
```

We can optionally include headings:

```
\n\n### <card name>\n\n<template>
```

But headings may be undesirable in some prompts; the separator strategy should be configurable later.

### 9.2 Variable schema merge

Prompt versions support variables: `variables: Record<string, VariableSchema>`.

When composing, we merge variables across cards:

- Union by variable name.
- Conflict detection:
  - If the same variable name appears with different `type`/`required`/`description`, mark a warning.
  - MVP behaviour: allow merge, prefer the “stricter” settings:
    - `required = true` if any card requires it.
    - `type`: if conflicting, set to `"string"` and warn (or leave empty and warn).

We should surface conflicts in the UI output pane.

### 9.3 Template validation

Backend validation exists in `internal/playground/registry/hash.go`:

- `ValidateTemplate(template, vars)` checks that all `{{placeholder}}` variables referenced exist in `vars`.

Implication:

- When exporting/saving a composed prompt, we must ensure the composed `variables` map includes *all* placeholders referenced across the concatenated template.

Known backend mismatch to handle (important):

- Backend validation trims whitespace inside placeholders (so `{{ name }}` is treated as `name`).
- Runtime render only replaces exact `{{name}}`.

Prompt Cards MVP mitigation:

- UI enforces/auto-formats placeholders to `{{name}}` (no internal whitespace) when editing card templates.
- UI linter warns if it detects `{{ something }}`.

Phase 1 mitigation (preferred long-term):

- Update backend template render to also replace whitespace variants, or implement a proper placeholder parser.

---

## 10) Proposed Route + Navigation Placement (Draft)

Two viable placements:

1. **Top-level**: `/prompt-cards`
   - Pros: feature stands alone; aligned with Flow.
   - Cons: adds another top nav item.
2. **Under Playground**: `/playground/prompt-cards`
   - Pros: conceptually adjacent to prompts and versioning.
   - Cons: the Prompt Cards UX is closer to Flow than to the current Playground forms.

Recommendation (MVP): **Top-level** `/prompt-cards` so it can have Flow-like layout and not be constrained by Playground’s tab semantics.


