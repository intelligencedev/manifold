# Unified Manifold UI Redesign Plan (obs-dash)

A single cohesive plan that merges the proposal, implementation playbook, and style guide into one shippable redesign for **Manifold’s Vue 3 + TypeScript + Tailwind UI** (repo: `manifold/web/agentd-ui`). The goal is an **observability-dashboard aesthetic**—dark glass, grid+glow canvas, crisp strokes, Inter typography—while **preserving the existing theme-store + Tailwind token pipeline** and enabling **incremental migration** behind a feature flag.

---

## 1) Goals & Non‑Goals

### Goals
- Introduce an **“Observability (Dark)”** look: glassy surfaces, minimal shadows, hairline borders, grid+glow background, subtle grain, dual accents.
- Keep compatibility with current Tailwind usage by **reusing existing `--color-*` token names** (no churn across components).
- Provide a small “surface kit” of reusable UI primitives to migrate views **incrementally**.
- Maintain accessibility (contrast + focus) and performance (blur/glow discipline).

### Non‑Goals
- No removal of existing themes (`aperture-dark/light` stay).
- No “big bang rewrite”; migrate view-by-view.

---

## 2) Pre‑Flight (repo + tooling + assets)

### Tooling
- Node **20+**
- pnpm **9.x** (example):  
  `corepack enable && corepack prepare pnpm@9.15.9 --activate`
- Install:  
  `cd manifold/web/agentd-ui && pnpm install`

### Assets
- Add subtle grain texture: `public/assets/noise.png` (transparent PNG, ~512–1024px square).
- Ensure Tailwind utilities reference it as `url(/assets/noise.png)`.

### Hygiene
- Keep changes additive.
- Run `pnpm lint` and/or `pnpm vitest --runInBand` for touched logic.

---

## 3) Design System: Theme + Tokens (no Tailwind breakage)

### Add theme: `obsdash-dark`
- Implement in `src/theme/themes.ts` as a new `ThemeId` union value and add to exported theme list.
- **All values flow through the same CSS variables** Tailwind already consumes (`--color-background`, etc.).

**Token set (RGB components):**
- `background: 6 8 12`
- `surface: 14 18 26`
- `surface-muted: 18 22 32`
- `border: 52 60 76`
- `input: 28 32 44`
- `ring: 118 182 255`
- `foreground: 232 238 247`
- `muted-foreground: 166 176 196`
- `subtle-foreground: 128 138 158`
- `faint-foreground: 94 104 124`
- `muted: 14 18 26`
- `accent: 108 127 255`
- `accent-foreground: 14 16 22`
- `info: 118 182 255`
- `success: 72 214 172`
- `warning: 244 188 110`
- `danger/destructive: 235 104 96` (+ matching `*-foreground`)

### Theme switching + gating
- Register `obsdash-dark` in the same theme store mechanism as aperture themes.
- Add optional global gating via body class: `body.theme-obsdash` when active.
- Roll out behind feature flag: **`uiTheme=obsdash`** (or equivalent), so both old and new can ship together.

---

## 4) Global Base Styling (Inter + radii + consistent surfaces)

### Typography
- Import Inter once in `src/assets/main.css`:
  ```css
  @import url('https://rsms.me/inter/inter.css');

  :root {
    font-family: 'Inter', system-ui, -apple-system, sans-serif;
    --radius: 18px;
    --radius-lg: 26px;
  }

  code, pre {
    font-family: 'Inter', 'Inter var', ui-monospace, SFMono-Regular, monospace;
  }
  ```

### Radius + borders
- Cards: `--radius` (18px)
- Panels/top shells: `--radius-lg` (26px)
- Pills/chips: `rounded-full`
- Borders are **1px hairlines** at ~8–12% opacity on dark glass (avoid pure black strokes).

---

## 5) Tailwind Utilities (background + glass + glow)

Add a small utility set in `tailwind.config.ts` (plugins array). This prevents bespoke per-view CSS and keeps migration fast:

- `.bg-grid-glow` (dual radial glow + grid lines)
- `.bg-grain` (noise overlay)
- `.glass-surface` (gradient + blur + hairline border)
- `.pill-glow` (accent glow ring)
- Keep any existing utilities (e.g., etched).

---

## 6) Core “Surface Kit” Components (reusable primitives)

Create under `src/components/ui/`:

- **GlassCard**: primary card container; optional `interactive` and `padded`.
- **Panel**: page section shell with header/footer slots; larger radius.
- **Pill**: status/filter labels; tones: `accent | neutral | success | danger`.
- **Chip**: compact meta tag; typically neutral.
- **MetricCard**: metrics display (value + label + delta) built on GlassCard.
- **Topbar**: sticky, glassy header with status pill and action slot.
- (Optional) **StatusBadge/Badge**: semantic state badges (can be Pill/Chip variants).

**Component rules**
- Prefer **strokes + subtle gradients** over heavy drop shadows.
- Keep blur limited to top-level surfaces (Topbar, GlassCard), not nested elements.

---

## 7) App Shell + Background System

In `App.vue`, wrap the app in layered backgrounds (grid+glow + grain), then keep content on a higher z-layer:

- Root: `min-h-screen bg-background text-foreground relative overflow-hidden`
- Background layers (pointer-events-none): `.bg-grid-glow`, `.bg-grain`
- Add `Topbar`
- Page gutters: `px-4 md:px-8 pb-10`

Gate the background layers behind `theme-obsdash` if needed to avoid affecting aperture themes.

---

## 8) Style Guide (rules for consistent UI during migration)

### Color usage rules
- Strokes: `border-white/8` to `border-white/12` on glass; hover up to ~20%.
- Semantic tones: use “10–15% fill + semantic text” (e.g., `bg-success/10 text-success`).
- Focus: always show `focus-visible:ring-2 focus-visible:ring-ring`.

### Typography scale (practical defaults)
- Key metric: `text-3xl font-semibold` (or bolder for hero)
- Section header: `text-sm text-muted-foreground` in Panel header slot
- Meta: `text-xs text-subtle-foreground`

### Spacing & layout
- Base unit 4px.
- Default grid gap: `gap-4` (16px), `gap-6` on md+ where needed.
- Page gutters: `px-4` mobile, `px-8` md+.
- Card padding: `p-4` mobile, `p-6` md+.

### Anti-patterns to avoid
- Heavy drop shadows as hierarchy.
- Too many accent colors (stick to `accent` + `info`).
- Blur stacked inside blur.
- Low-contrast muted text on muted surfaces.

---

## 9) View‑by‑View Migration Order (incremental, high impact first)

1) **Overview**
- Convert the main container to **Panel**.
- Add a metrics grid (e.g., 2 cols at md, 4 cols at lg) using **MetricCard**.
- Use GlassCard for secondary sections (activity/history).

2) **Chat**
- Messages become GlassCards with role Pill + timestamp meta line.
- Input area aligns to the glass aesthetic (surface-muted fill + ring focus).
- Ensure scroll areas remain readable against the grid/glow (tone down opacity if needed).

3) **Projects**
- Lists become interactive GlassCard rows (hover border accent).
- Filters use Pill/Chip patterns.

4) **Specialists**
- Cards use GlassCard; status becomes Pill/Badge with semantic tones.
- Optional: add MetricCard-style summaries (utilization/latency).

5) **Settings + remaining views (Resources, Logs, etc.)**
- Panel shells + standardized form styles.
- Inputs follow the same focus/contrast rules.

6) **Nav/Sidebar (if present)**
- Glassy rail with clear active indicator and consistent spacing.

---

## 10) Testing, QA, and Performance

### Testing
- Unit: `pnpm test:unit` / Vitest for new computed logic (e.g., MetricCard deltas).
- E2E/visual: `pnpm test:e2e` / Playwright with screenshot coverage for Overview + Chat.

### Accessibility
- Text contrast ≥ 4.5:1 (especially muted text).
- Keyboard focus always visible (use ring token).
- Hit targets ~44px for buttons/inputs.

### Performance guardrails
- Limit `backdrop-filter` to key surfaces (Topbar + primary cards).
- Keep grain opacity ≤ 0.4.
- Consider gating grid/glow for low-end devices or reduced motion:
  - If `prefers-reduced-motion: reduce`, reduce glow intensity / disable transitions.

---

## 11) Phased Implementation Timeline (practical sequencing)

**Phase 1 — Foundation (1–2 days)**
- Add `obsdash-dark` theme tokens.
- Add Inter + radii in CSS base.
- Add Tailwind utilities (grid/glow/grain/glass/pill glow).
- Add body class gating (`theme-obsdash`) if desired.

**Phase 2 — Shell & Topbar (0.5–1 day)**
- Implement Topbar.
- Update `App.vue` shell with background layers + gutters.

**Phase 3 — Surface Kit (1–2 days)**
- Add GlassCard, Panel, Pill, Chip, MetricCard (+ optional badges).
- (Optional) wire stories if Storybook exists.

**Phase 4 — View migrations (2–4 days)**
- Overview → Chat → Projects → Specialists → Settings.
- Validate each view against style + accessibility checklist.

**Phase 5 — Polish & QA (0.5–1 day)**
- Contrast + focus audits, responsive checks, perf tuning.

**Phase 6 — Rollout**
- Ship behind `uiTheme=obsdash`.
- Gather feedback.
- Optionally make `obsdash-dark` default for dark users; keep `aperture-light` as light fallback.

---

This unified plan keeps Manifold’s theming architecture intact, provides a tight component/tooling “kit” for consistent glass-dashboard UI, and lays out a low-risk, incremental migration path with testing and accessibility baked in.