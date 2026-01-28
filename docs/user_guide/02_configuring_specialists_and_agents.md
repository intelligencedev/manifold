---
title: Configuring Specialists (Agents)
description: Create and manage named specialists bound to model providers, system prompts, and tool access.
---

# Configuring Specialists/Agents

Specialists are named assistants you define for the platform. Each specialist binds to a provider/model, a system prompt, and optional tool access. Chats can target a specific specialist; the orchestrator remains available as a default.

Placeholder for screenshots: [Specialists grid + editor]

## Where to find it

- App header: Specialists in the main navigation.

## List view

The left card grid shows each specialist with:
- Name and active/paused status.
- Model label and short description (derived from Description or the System Prompt snippet).
- Badges for tool access: Tools enabled/disabled and whether an allow list is set.

Actions per card
- Edit: open the editor.
- Clone: duplicate into a paused copy; name ends with “copy”.
- Pause/Resume: toggle availability; paused specialists are excluded from runtime registries.
- Delete: remove the specialist configuration.

## Create or edit a specialist

The right-side editor has two columns: core settings on the left, system prompt and tools on the right.

Core settings
- Name: unique identifier (immutable when editing an existing specialist).
- Description: optional; appears in the card grid.
- Provider: openai, anthropic, google, or local. Changing this applies sensible defaults for model/base URL/headers.
- Model: model identifier for the provider (e.g., gpt-4o, claude-3-5-sonnet, gemini-1.5-pro).
- Base URL: custom endpoint base if you proxy or self-host.
- API Key: secret for the provider (stored server-side).
- Enable Tools: allow tool calling for this specialist; if disabled, no tool schema is sent to the model.
- Paused: hide from registries without deleting.
- Extra Headers (JSON): static headers injected into all requests.
- Extra Params (JSON): provider-specific parameters (e.g., reasoning_effort for OpenAI JSON format).

System Prompt
- A freeform prompt that is prepended to the chat history.
- “Apply saved prompt version” lets you pull a template from Playground Prompts (see Prompts docs). Choose a Prompt, then a Version; the System Prompt textarea is replaced with the version’s template.

Tool Access
- Modes: Disable tools; Allow any tool; Use an allow list.
- Manage tools opens a modal with search and multi-select. Selecting any tools switches mode to “allow list”.
- When a specialist with tools runs, only the allowed schemas are advertised to the LLM.

Saving
- Clicking Save validates JSON fields and upserts the specialist via the API. Errors are surfaced inline.

## Teams

Teams group specialists under a dedicated orchestrator configuration. A team has its own provider/model, tool policy, and system prompt, and can include any subset of specialists. Specialists can belong to multiple teams.

Use the Teams section to:
- Create or edit a team (name, description, orchestrator settings).
- Configure tool access for the team orchestrator.
- Add or remove team members.

In chat, selecting a team runs the request using the team orchestrator and limits participants to the team’s members.

Backend behavior
- The agent registry builds providers from the specialist config and the base LLM settings. When tools are enabled, the registry creates a filtered tools view from the global registry using the allow list.
- Reasoning effort and extra params are merged and passed when supported by the provider implementation.

Placeholder for screenshots: [Editor core fields; Tools modal]

