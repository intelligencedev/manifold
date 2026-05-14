# AGENTS.md — agentd-ui (Vue 3 frontend)

> Scope: this file applies to everything under `web/agentd-ui/`. If a more
> deeply-nested `AGENTS.md` exists, the closest one to the file you are editing
> wins. For repository-wide rules, see the root `AGENTS.md`.

## 1. What this package is

`agentd-ui` is the Vue 3 single-page application that ships as the production
UI for `agentd`. The built assets are embedded into the Go binary via
`make frontend` (run from the repo root) and served by `agentd` on port
**32180**. During development, Vite serves the app on its own port and proxies
API traffic to a running `agentd` instance.

Treat this package as a **library that is bundled into a Go binary**, not a
standalone web app:

- No server-side rendering, no Node runtime in production.
- No environment variables are read at runtime in the browser — only `VITE_*`
  values inlined at build time.
- All network calls go to same-origin paths (`/api`, `/stt`, `/audio`,
  `/auth`). Never hardcode hostnames.

## 2. Tech stack (authoritative)

| Area            | Choice                                                   |
| --------------- | -------------------------------------------------------- |
| Framework       | Vue 3.4 with `<script setup lang="ts">` SFCs             |
| Language        | TypeScript (strict via `tsconfig.base.json`)             |
| Build           | Vite 6 (`@vitejs/plugin-vue`, `@vitejs/plugin-vue-jsx`)  |
| Package manager | pnpm `9.15.9` (see `packageManager` field — do not change) |
| Node            | `>=22.16.0 <23` (use `nvm use` from repo root)           |
| State           | Pinia                                                    |
| Routing         | vue-router 4                                             |
| Server state    | `@tanstack/vue-query`                                    |
| HTTP            | `axios` (a single shared instance in `src/api/`)         |
| Graph UI        | `@vue-flow/*` + `dagre` for layout                       |
| Markdown        | `markdown-it` + `dompurify` + `highlight.js`             |
| Grid            | `vue3-grid-layout-next`                                  |
| Styling         | Tailwind CSS 3 with CSS-variable theme tokens            |
| Unit tests      | Vitest + jsdom + `@vue/test-utils` + `@testing-library/vue` |
| E2E tests       | Playwright (Chromium project)                            |
| Lint / format   | ESLint (`eslint-plugin-vue`, `@vue/eslint-config-typescript`) + Prettier |

Do not introduce alternative libraries that overlap with the above
(e.g. Vuex, fetch wrappers, Chakra, styled-components, Jest, Cypress) without
an explicit ADR-style note in the PR description.

## 3. Setup

From the repo root:

```bash
nvm use                              # picks Node 22
pnpm -C web/agentd-ui install        # uses lockfile; do NOT run plain `npm install`
```

If you change dependencies, commit the updated `pnpm-lock.yaml`. CI uses
`pnpm install --frozen-lockfile`.

## 4. Commands

Run from `web/agentd-ui/` (or use `pnpm -C web/agentd-ui <script>` from root):

| Task                | Command                                          |
| ------------------- | ------------------------------------------------ |
| Dev server          | `pnpm dev`                                       |
| Dev server w/ proxy | `VITE_DEV_SERVER_PROXY=http://127.0.0.1:32180 pnpm dev` |
| Production build    | `pnpm build`                                     |
| Preview build       | `pnpm preview`                                   |
| Lint                | `pnpm lint`                                      |
| Format              | `pnpm format`                                    |
| Unit tests          | `pnpm test:unit`                                 |
| Unit tests (watch)  | `pnpm test:unit --watch`                         |
| Single unit file    | `pnpm test:unit tests/path/to/file.spec.ts`      |
| E2E (headless)      | `pnpm test:e2e`                                  |
| E2E (headed)        | `pnpm test:e2e:headed`                           |
| Build + embed       | `make frontend` (from repo root)                 |

**Before opening a PR, you must run, in this order, and all must pass:**

```bash
pnpm lint
pnpm test:unit
pnpm build
```

E2E (`pnpm test:e2e`) is required when changes touch routing, auth, or any
flow already covered under `e2e/`.

## 5. Project layout

```
web/agentd-ui/
├── e2e/                      # Playwright specs (baseURL http://127.0.0.1:4173)
├── public/                   # Static assets copied verbatim
├── src/
│   ├── api/                  # axios instance + typed API clients (one file per resource)
│   ├── assets/               # Imported assets (images, fonts, css)
│   ├── components/           # Reusable presentation components
│   │   └── ui/               # Design-system primitives (e.g. AppButton.vue)
│   ├── composables/          # `useXxx` Composition-API helpers (no DOM-only logic)
│   ├── constants/            # Pure constants and enums
│   ├── lib/                  # Framework-agnostic helpers (no Vue imports)
│   ├── mocks/                # Test/dev mocks (never imported from prod code paths)
│   ├── router/               # vue-router config and route guards
│   ├── stores/               # Pinia stores (`useXxxStore`)
│   ├── theme/                # CSS variable definitions and theme switching
│   ├── types/                # Shared TS types/interfaces
│   ├── utils/                # Small pure utilities
│   ├── views/                # Route-level components (lazy-loaded from router)
│   ├── App.vue
│   └── main.ts               # App bootstrap (Pinia, Router, Vue Query install)
├── tests/                    # Vitest specs (mirror `src/` structure)
├── env.d.ts                  # Ambient types for `import.meta.env`
├── index.html
├── package.json
├── playwright.config.ts
├── postcss.config.cjs
├── tailwind.config.ts
├── tsconfig.app.json         # App build config
├── tsconfig.base.json        # Shared base
├── tsconfig.vitest.json      # Test config
└── vite.config.ts
```

Path alias: **`@/` → `src/`**. Always prefer `@/foo/bar` over deep relative
imports (`../../../foo/bar`).

## 6. Coding conventions

### 6.1 Components

- Use **SFCs** with `<script setup lang="ts">`. No Options API in new code.
- Component filenames are **PascalCase** and match the default export name
  (e.g. `AgentCard.vue`, used as `<AgentCard />`).
- Define props and emits with the type-based form:

  ```ts
  const props = defineProps<{ agentId: string; compact?: boolean }>()
  const emit = defineEmits<{ (e: 'select', id: string): void }>()
  ```

- Prefer `withDefaults(defineProps<...>(), { compact: false })` for defaults.
- One component per file. Keep `<template>` first, then `<script setup>`,
  then `<style>` (scoped only when truly necessary — prefer Tailwind).
- Views (under `src/views/`) are lazy-loaded by the router; do not import them
  statically except from `src/router/`.

### 6.2 Composables

- File name and export start with `use` (`useAgents.ts` → `export function useAgents()`).
- Composables must be **pure with respect to component lifecycle**: any
  `onMounted` / `watchEffect` they register must be safe to call from a
  component’s `setup()`.
- Don’t access the DOM from a composable unless it accepts a template ref.

### 6.3 State management

- **Pinia** for client state. Stores live in `src/stores/` and are named
  `useXxxStore`. Use the **setup-store** form:

  ```ts
  export const useAgentsStore = defineStore('agents', () => {
    const items = ref<Agent[]>([])
    const byId = computed(() => new Map(items.value.map(a => [a.id, a])))
    function setAll(next: Agent[]) { items.value = next }
    return { items, byId, setAll }
  })
  ```

- **Vue Query** for server state (lists, fetch-by-id, mutations). Do not cache
  server responses inside Pinia — keep server state in Vue Query and derived
  UI state in Pinia.
- Avoid `provide`/`inject` for app-wide state; use a Pinia store instead.

### 6.4 HTTP / API layer

- All HTTP goes through the shared axios instance exported from `src/api/`.
  Do not call `axios.create` ad hoc and do not use `fetch` directly.
- One file per resource (`src/api/agents.ts`, `src/api/sessions.ts`, etc.)
  exporting typed functions: `listAgents()`, `getAgent(id)`, `createAgent(...)`.
- Use **same-origin paths** (`/api/...`). Never hardcode `http://localhost:...`.
- Wrap calls in Vue Query (`useQuery` / `useMutation`) inside composables or
  views, never inline in random components.

### 6.5 Routing

- Routes are defined in `src/router/`. Use **named routes** and refer to them
  by name (`router.push({ name: 'agent', params: { id } })`).
- Code-split route components via `() => import('@/views/AgentView.vue')`.
- Auth/role guards live in `src/router/` only — not inside views.

### 6.6 TypeScript

- `strict` is on. Don’t silence errors with `// @ts-ignore`; prefer
  `// @ts-expect-error` with a one-line reason, and only as a last resort.
- Avoid `any`. Use `unknown` plus narrowing, or proper generics.
- Shared cross-feature types go in `src/types/`. Local types stay next to
  their consumer.
- Vue ambient types come from `"types": ["vue"]` in `tsconfig.app.json`. Do
  not add `vue/dist/*` imports manually.

### 6.7 Styling

- **Tailwind first.** Use utility classes in templates. Reach for a
  `<style>` block only for keyframes, complex selectors, or third-party
  overrides.
- Colors and surfaces come from CSS variables defined in `src/theme/` and
  exposed in `tailwind.config.ts` as `rgb(var(--color-*) / <alpha-value>)`.
  **Use semantic tokens** (`bg-surface`, `text-foreground`, `border-border`,
  `text-destructive`, etc.). Do **not** use raw Tailwind palette colors
  (`bg-slate-800`, `text-red-500`) in app code — they break theming.
- Use the project’s spacing/radius/shadow scales (`rounded-3`, `shadow-2`,
  etc.) defined in `tailwind.config.ts`.
- Custom utilities like `.glass-surface`, `.pill-glow`, `.etched-light` are
  defined in the Tailwind plugin — prefer them over re-implementing.
- Dark mode is driven by CSS-variable themes, not Tailwind’s `dark:` variant.
  Do not introduce `dark:` utilities.

### 6.8 Markdown / HTML rendering

- All user- or model-generated markdown must go through `markdown-it` and be
  sanitized with `dompurify` before being injected. Never use `v-html` on
  unsanitized strings.
- Code highlighting uses `highlight.js`. Register only the languages you
  need; don’t import the full bundle.

### 6.9 Vue Flow

- Use the existing wrapper components/composables before adding new
  `@vue-flow/*` imports.
- Layout is computed with `dagre`; place layout helpers under
  `src/lib/` (framework-agnostic) and keep node/edge components in
  `src/components/`.

### 6.10 Imports & module hygiene

- Order: builtin/3rd-party → `@/` aliases → relative → styles.
- No circular imports between `stores/`, `api/`, and `composables/`.
- `src/lib/` must not import from `src/components/`, `src/views/`,
  `src/stores/`, or `vue`.

### 6.11 Naming

- Components: `PascalCase.vue`
- Composables: `useThing.ts`
- Stores: `useThingStore` in `things.ts` or `thingStore.ts`
- Types/interfaces: `PascalCase`
- Constants: `SCREAMING_SNAKE_CASE`
- Test files: `*.spec.ts` (unit) under `tests/`, `*.spec.ts` under `e2e/`

## 7. API & dev proxy

Set `VITE_DEV_SERVER_PROXY` to point Vite at a running `agentd`:

```bash
VITE_DEV_SERVER_PROXY=http://127.0.0.1:32180 pnpm dev
```

This proxies `/api`, `/stt`, `/audio`, `/auth` (see `vite.config.ts`). When
adding a new top-level API path, update the proxy block in `vite.config.ts`
in the **same PR**.

Runtime config is **build-time only**. New configurable values must:

1. Be exposed under the `VITE_` prefix.
2. Be declared in `env.d.ts` with a precise type.
3. Have a documented default; never throw at import time if missing.

## 8. Testing

### Unit (Vitest)

- Specs live in `tests/` and mirror `src/` (`tests/components/AgentCard.spec.ts`).
- Environment is `jsdom`, globals are enabled, and setup runs from
  `tests/setupTests.ts`. Add new global mocks/matchers there, not per-file.
- Use `@vue/test-utils` for component mounting and
  `@testing-library/vue` + `@testing-library/jest-dom` for behavior-style
  assertions.
- Mock the network at the **api module** boundary (`vi.mock('@/api/agents')`),
  not at axios. Do not hit the real network in unit tests.
- Components that use `vue3-grid-layout-next` must rely on the existing
  `tests/styleMock.css` alias — do not import the real CSS in tests.
- A change to `src/` requires either a new test or a clearly justified note
  in the PR explaining why no test is feasible.

### E2E (Playwright)

- Specs in `e2e/`. Default `baseURL` is `http://127.0.0.1:4173` (Vite preview).
- In CI, Playwright runs `pnpm preview` automatically; locally you must
  `pnpm build && pnpm preview` first, or pass `PLAYWRIGHT_BASE_URL`.
- Keep E2E tests deterministic: stub network at the route level
  (`page.route('**/api/...', ...)`) unless the test is explicitly an
  integration test against a real `agentd`.
- Do not add new browser projects (Firefox/WebKit) without discussion — the
  embedded UI only needs to support Chromium-class engines for now.

## 9. Build & embed pipeline

- `pnpm build` produces the bundle in `dist/`.
- `make frontend` (from repo root) runs `pnpm install --frozen-lockfile`,
  builds, and copies the output into the Go embed directory.
- Do **not** commit `dist/`, `node_modules/`, or `tsconfig.*.tsbuildinfo`.
- Keep the bundle lean:
  - Lazy-load route views.
  - Avoid importing entire icon packs; import individual icons.
  - Don’t add a dependency just for one helper — write it in `src/utils/`.

## 10. Accessibility & UX baseline

- All interactive elements must be reachable via keyboard and have a visible
  focus ring (use the `ring` token).
- Buttons use `<AppButton>` (in `src/components/ui/`) unless there’s a
  specific reason not to.
- Provide `aria-label` on icon-only controls. Provide `alt` on `<img>`.
- Respect `prefers-reduced-motion` for any non-trivial animation.

## 11. Security

- Never `v-html` untrusted strings; always sanitize with `dompurify`.
- Never log tokens or full request/response bodies for `/auth`.
- Don’t store secrets in `VITE_*` — those are public after build.
- Cookies and auth headers are managed by `agentd`; the SPA must not parse
  or persist auth tokens itself.

## 12. Commits & PRs

- Conventional Commits (`feat(ui): ...`, `fix(ui): ...`, `chore(ui): ...`,
  `test(ui): ...`, `refactor(ui): ...`).
- Keep PRs focused: a feature change and a dependency bump go in separate PRs.
- PR description must include:
  - **What** changed and **why**.
  - Commands run (`pnpm lint`, `pnpm test:unit`, `pnpm build`, optionally
    `pnpm test:e2e`).
  - Screenshots or a short clip for visible UI changes.
- Lockfile changes must be intentional. If `pnpm-lock.yaml` changes, mention
  it in the PR description.

## 13. Things agents must NOT do

- ❌ Switch package manager (no `npm install`, no `yarn`).
- ❌ Bump Node, Vue, Vite, Pinia, vue-router, or Tailwind major versions in a
  drive-by change.
- ❌ Add a second HTTP client, state library, or test runner.
- ❌ Introduce SSR, Nuxt, or a Node-only runtime dependency.
- ❌ Use Options API in new components.
- ❌ Use raw Tailwind palette colors instead of semantic theme tokens.
- ❌ Inline `<script>` or external `<iframe>` content into `index.html`.
- ❌ Hardcode hostnames, ports, or absolute URLs for backend calls.
- ❌ Commit `dist/`, `coverage/`, `playwright-report/`, or
  `*.tsbuildinfo`.
- ❌ Disable lint rules or TS checks broadly to make CI green; fix the cause.
- ❌ Add large assets (>200 KB) without justification; prefer SVG or
  optimized WebP.

## 14. When in doubt

1. Look for a similar pattern already in `src/` and follow it.
2. If no precedent exists, prefer the **smallest** change that is consistent
   with sections 2 and 6 of this file.
3. Document the decision in the PR description.
