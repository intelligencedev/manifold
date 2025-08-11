WARPP for this project

Overview

This repository now includes a production-ready implementation of WARPP (Workflow Adherence via Runtime Parallel Personalization). WARPP is a modular, multi-agent execution pattern that identifies intent, personalizes a workflow at runtime, and executes the personalized path after authentication, with strong guardrails on tool usage.

What we’ve implemented

- A portable WARPP runner that uses a typed workflow model and your configured tools
  - Workflows are natural-language steps with optional guards and tool references
  - Minimal attribute inference and guard evaluation at runtime
  - Simple template substitution in tool arguments via ${A.key}
  - Tool allow-listing derived from the trimmed workflow, enforced at execution
- An optional on-disk workflow registry
  - JSON files under configs/workflows/*.json (optional)
  - If none are found, sensible defaults ship with the binary (web_research and cli_echo)
- CLI and TUI integration
  - cmd/agent: add -warpp flag to execute WARPP instead of the LLM agent
  - cmd/agent-tui: add -warpp flag to run WARPP within the TUI; tool outputs appear on the right pane, final summary on the left pane

Key files

- internal/warpp/
  - types.go — workflow, step, and tool reference types
  - loader.go — file loader and default workflow registry
  - guards.go — minimal guard language (A.key, A.key == 'v', not A.key)
  - runner.go — DetectIntent, Personalize (TRIM), Execute
- internal/agent/warpp.go — generic orchestration and tests for the WARPP protocol (parallel auth + personalization + gating)
- cmd/agent/main.go — -warpp switch for CLI
- cmd/agent-tui/main.go and internal/tui/model.go — -warpp switch and TUI integration

Bundled workflows (examples)

- deep_web_report — researches a topic and writes a deep research style report to report.md using tools: web_search → web_fetch → write_file
  - Now includes multiple fetch steps (first and second search results). The runner synthesizes a report combining multiple sources before writing, and finally echoes report.md to the console using run_cli (cat or type on Windows).
- web_research — searches the web and fetches a promising URL (no file write)
- cli_echo — echoes input via run_cli

Usage

CLI (non-interactive)

- Run standard LLM tool-calling agent (default):
  go run ./cmd/agent -q "Search for latest Go features"

- Run WARPP instead:
  go run ./cmd/agent -warpp -q "Search for latest Go features"

TUI (interactive)

- Default (LLM with streaming):
  go run ./cmd/agent-tui

- WARPP mode:
  go run ./cmd/agent-tui -warpp

Workflow model

- Workflow
  - intent: string
  - description: string
  - keywords: []string (for simple intent detection)
  - steps: []Step
- Step
  - id: string
  - text: string
  - guard?: string — over A (attribute map)
  - tool?: ToolRef
- ToolRef
  - name: string
  - args?: map[string]any — string values can include ${A.key} substitutions

Personalization and TRIM

- Attributes A are collected/inferred during personalization
  - In this implementation: basic inference from the utterance
    - A.utter is always set to the user input
    - A.query defaults to A.utter
- TRIM performs:
  - Static pruning: drop steps whose guard is false
  - Tool allow-listing: collect only tools referenced by the trimmed steps
  - Fidelity preservation for outcomes is not necessary for the shipped example flows; the structure accommodates it if you extend steps to include outcomes

Guard language (minimal)

- "" or "true" — always true
- "A.key" — true if present and truthy (non-empty string, true boolean, non-nil)
- "not A.key" — negation
- "A.key == 'value'" and "A.key != 'value'"

Argument templating

- Any string value in step.tool.args of the form ${A.key} is substituted at runtime
- Nested objects and arrays are supported (recursively processed)

Tool enforcement

- Tools are executed only if they appear in the allowlist derived from the trimmed workflow
- The executor never uses tools beyond those referenced by the personalized plan (W*, D*)

Workflows on disk

- Place JSON workflows under configs/workflows/*.json:
  {
    "intent": "updateAddress",
    "description": "Example flow",
    "keywords": ["address", "update"],
    "steps": [
      {"id": "s1", "text": "Search", "tool": {"name": "web_search", "args": {"query": "${A.query}"}}},
      {"id": "s2", "text": "Fetch",  "tool": {"name": "web_fetch", "args": {"url": "${A.first_url}", "prefer_readable": true}}}
    ]
  }
- If none are provided, two defaults are embedded:
  - web_research — web_search → web_fetch
  - cli_echo — simple echo using run_cli

What to expect (examples)

- CLI
  - go run ./cmd/agent -warpp -q "web search: jellyfish lifespan"
    - Detects the web_research workflow
    - Personalization sets A.query="web search: jellyfish lifespan"
    - Execute:
      - web_search: returns top URLs; captures first URL into A.first_url
      - web_fetch: fetches readable content from A.first_url
    - Outputs a concise, deterministic summary of executed steps

  - go run ./cmd/agent -warpp -q "Write a deep research report on the history of containerization"
    - Selects deep_web_report workflow
    - A.query set from the utterance
    - Executes: web_search → web_fetch (top result) → web_fetch (second result, if present) → write_file
    - Creates report.md under WORKDIR with a deep-research style markdown report
    - Console output includes a summary of steps and a final confirmation line:
      "Objective complete: report written to report.md (steps=N)."

- TUI
  - Run with -warpp, input a research prompt
  - Right pane shows web_search payload then web_fetch payload (title, markdown)
  - Left pane shows the summary produced by the runner

Interoperability with existing agent

- The WARPP implementation does not alter the existing agent loop
- The -warpp flag selects a completely different path and still uses the same configured tools (run_cli, web_search, web_fetch)
- No code paths are shared in a way that could introduce regressions; both modes can coexist

Extending WARPP

- Add info tools and extend personalization to call them and enrich A
- Introduce outcomes on steps and update Execute to honor outcome control flow
- Extend guards to more complex expressions or use a small embedded expression engine
- Add LLM-based DetectIntent to choose among many workflows by name or metadata

Testing and quality

- internal/agent/warpp_test.go verifies orchestration gating: no fulfillment before personalization and authentication
- The warpp runner is intentionally simple and deterministic for reliability; it uses the project’s existing tools and safety constraints

If you want richer example workflows, add them under configs/workflows, and the runner will pick them up automatically at runtime.
