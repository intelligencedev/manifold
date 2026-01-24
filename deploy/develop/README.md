# Developer Tooling

This folder contains tooling and configuration intended for contributors developing Manifold locally.

Guidelines:

- Keep repo root clean: developer tools live under `develop/`.
- Never hard-code credentials in scripts or configs. Use `.env` (already ignored by git).
- Prefer additive, opt-in tooling that does not affect production builds.

## Tools

- `sonarqube/`: Local SonarQube stack + scanner configuration.
