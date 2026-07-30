# Manifold Software Wiki Config

Created: `2026-07-14`
Repository root: `manifold/`
Wiki root: `manifold/wiki/`

## Purpose

Source-grounded contributor wiki for the Manifold Go agent platform. Emphasize runtime agent/LLM paths that affect provider-visible context quality, cost, and correctness.

## Audience

- New contributors
- Maintainers
- LLM coding agents
- Reviewers orienting before changing agent context pipelines

## Scope

In scope:

- Agent engine loop, provider marshalling, and context compression (`lexminify`)
- Package boundaries under `internal/`
- Runtime flows from composed messages to provider calls
- Setup, build, test commands found in the repository
- Mermaid diagrams for Request -> Minify -> Provider sequences
- Explicit evidence paths and unknowns

Out of scope:

- Generic LLM tutorials
- Unverified savings claims
- Changes to application code unless required to document accurately

## Repository Crawl Policy

- Primary evidence: path-linked source, tests, Makefile, package comments
- Start from `internal/agent`, `internal/llm`, `cmd/agent`, `internal/agentd`
- Prefer pure Go package behavior over runtime config unless config is the control surface

## Maintenance Rules

- Update `index.md`, `evidence.md`, and `logs/maintenance-log.md` when documenting important working changes
- Label inference separately from direct observation
- Prefer short pages that answer “where is it magically applied?” and “how do I turn one zone off?”