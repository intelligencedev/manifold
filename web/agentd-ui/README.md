# agentd-ui routes

This UI now includes multiple views:

- Overview (existing dashboard) at `/`
- Chat (LLM chat placeholder) at `/chat`
- Flow (VueFlow canvas) at `/flow`
- Runs at `/runs`
- Settings at `/settings`

A top navigation bar provides links to these routes. The Flow view uses VueFlow core with background, controls, and minimap.

Dev server:

```bash
pnpm -C web/agentd-ui dev
```

Build:

```bash
pnpm -C web/agentd-ui build
```
