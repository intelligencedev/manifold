# agentd-ui

This directory contains the frontend bundled into the `manifold` container and embedded into `agentd` for production-style runs.

For a normal Docker deployment from the repository root, you do not need to build this directory manually. The Docker build handles the frontend bundle.

## Local Frontend Development

Requirements:

- Node 22
- `pnpm`

If you use `nvm`, run `nvm use` from the repository root before working in this directory.

Install dependencies:

```bash
pnpm -C web/agentd-ui install
```

Run the dev server:

```bash
pnpm -C web/agentd-ui dev
```

Build the production bundle:

```bash
pnpm -C web/agentd-ui build
```

Or copy the built assets into the Go embed directory through the repository Makefile:

```bash
make frontend
```

## Notes

- The embedded production UI is served by `agentd` on port `32180`.
- The Vite dev server uses its own port during frontend development.
- API routes still target the running `agentd` instance.
