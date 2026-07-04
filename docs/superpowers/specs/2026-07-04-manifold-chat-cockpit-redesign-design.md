# Manifold Chat Cockpit Redesign

Date: 2026-07-04

## Objective

Redesign the Manifold frontend's `/chat` route to resemble the attached multi-agent cockpit image while preserving the existing working chat behavior. This is a Chat/orchestration-first pass, not a full redesign of every route.

## Scope

In scope:

- Reshape `/chat` into a dense orchestration workspace with a left conversation/workspace panel, central active run surface, and right participant/operations inspector.
- Make minimal shared shell/topbar/rail adjustments only where needed to support the chat cockpit.
- Preserve real application behavior for sessions, projects, memory settings, team routing, participants, messages, attachments, streaming, input requests, and modals.
- Derive visual summaries from existing chat state and agent thread data; when data is unavailable, show idle or empty states instead of synthetic runtime data.
- Add focused tests that protect visible landmarks and core interaction behavior.

Out of scope:

- Mock-only showcase screens.
- Redesigning every Manifold route.
- New backend endpoints.
- New frontend dependencies.
- Pixel-perfect replication of screenshot data labels that do not map to current product state.

## Design Direction

The chat route becomes a "mission control" surface. The attached image's core qualities should carry through: dark instrument-panel atmosphere, compact typography, bordered panes, colored agent status cues, and a central sense of orchestration progress.

The implementation should use Manifold's existing Vue 3, Tailwind, semantic theme tokens, Pinia, Vue Query, and local UI primitives. Styling should stay token-driven and avoid raw Tailwind palette colors in app code.

## Layout

Left panel:

- Keep real conversations and session actions.
- Increase resemblance to the reference with compact workspace/project/conversation grouping.
- Preserve create, select, rename, pin, export, delete, and bulk-delete flows.

Center panel:

- Keep the active conversation header with project, memory, and command policy controls.
- Present assistant activity as orchestration rather than plain chat only.
- Retain the real message stream, markdown rendering, attachments, input requests, timers, and composer.
- Introduce cockpit-style summary blocks for active participants, run activity, and tool activity using existing computed chat data.

Right panel:

- Keep real participant/team routing controls.
- Show active agents, participant state, and selected team context in a denser inspector.
- Add operational readouts that can be derived locally, such as message counts, context usage, memory state, run state, and recent activity.

Shared shell:

- Keep changes minimal and compatible with other routes.
- Use the existing app shell, topbar, and rail as support structure for `/chat`.

## Data Flow

Existing stores and API modules remain authoritative:

- `useChatStore` supplies sessions, messages, active runs, streaming state, participant activity, memory settings, and chat actions.
- `useProjectsStore` supplies projects and active project options.
- Existing specialist and team queries supply team routing and participant lists.

New visual summaries should be computed in `ChatView.vue` or a small local helper only if that reduces complexity. Do not introduce a parallel state model, and do not invent activity that is not present in the existing store/API data.

## Empty And Loading States

The redesign must remain useful when data is sparse:

- No sessions: show the existing empty conversation path with cockpit styling.
- No participants: show a compact unavailable state.
- No active run: show idle readouts rather than fake activity.
- Loading and error states should remain visible and actionable.

## Accessibility

- Preserve labels for project selection, memory toggle, team selection, composer, and modal dialogs.
- Icon-only buttons must keep aria labels.
- Focus rings must remain visible.
- The route should remain keyboard usable for common chat workflows.

## Testing

Add or update Vitest coverage for:

- The redesigned `/chat` landmarks and labels.
- Existing send-message flow.
- Project selection and memory toggle behavior.
- Team routing and participant display.

Run at least:

- `pnpm -C web/agentd-ui test:unit tests/views/chatview.spec.ts`
- `pnpm -C web/agentd-ui lint`
- `pnpm -C web/agentd-ui build`

## Implementation Notes

- Prefer incremental edits to `ChatView.vue` and scoped CSS.
- Keep any shared shell changes small and compatible with current route tests.
- Avoid introducing new dependencies or changing package manager files.
- Existing uncommitted UI changes in the workspace must be preserved and worked with, not reverted.
