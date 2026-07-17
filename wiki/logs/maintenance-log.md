# Wiki Maintenance Log

| Date | Change | Evidence |
| --- | --- | --- |
| 2026-07-14 | Scaffolded Manifold `wiki/` and documented completed lexminify work (level 6 defaults, zone model, system + **tool** zones on by default, provider-only copy hooks, disable patterns, tests). | `wiki/index.md`, `wiki/components/lexminify.md`, `wiki/flows/lexminify-provider-path.md`, `wiki/architecture/decisions.md`, `wiki/evidence.md`, package sources under `internal/llm/lexminify` and `internal/agent` |
| 2026-07-15 | Documented unminified `[LEXMINIFY NOTICE]` advisory prepended to first system/developer message when ZoneSystemPrompt minifies (D8). | `internal/llm/lexminify/lexminify.go` (`LexMinifyAdvisory`), tests, `wiki/components/lexminify.md`, `wiki/flows/lexminify-provider-path.md`, `wiki/architecture/decisions.md` |
