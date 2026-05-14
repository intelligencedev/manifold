# Contributor Playbook

Use this playbook when making a change.

## 1. Identify the Subsystem

Start with [Repository Map](../architecture/repository-map.md). Determine whether the change is in:

- HTTP/API integration (`internal/agentd`);
- agent loop (`internal/agent`);
- tools/MCP (`internal/tools`, `internal/mcpclient`);
- storage (`internal/persistence/databases`);
- project filesystem (`internal/projects`, `internal/workspaces`, `internal/sandbox`);
- frontend (`web/agentd-ui`);
- domain package (`flow`, `transit`, `rag`, `codeqa`, `playground`, etc.).

## 2. Trace Inputs and Outputs

For any feature, identify:

- API route or CLI entrypoint;
- request payload or tool JSON schema;
- auth/user/project scoping;
- storage writes;
- stream events or frontend types;
- tests.

## 3. Change the Lowest Responsible Layer

Prefer changing domain services and store interfaces before handlers. Keep `agentd` focused on orchestration, routing, and response mapping.

## 4. Update Contracts Together

If you change a public or model-visible contract, update all relevant locations:

- route handler;
- OpenAPI catalog;
- frontend API client/types;
- tool schema;
- docs/wiki;
- tests.

## 5. Validate Safety Boundaries

Explicitly check:

- path containment;
- auth/tenant scoping;
- tool allow lists and discovery;
- cancellation/timeouts;
- schema migrations;
- prompt/context separation for untrusted data.

## 6. Run Focused Tests First

Run the package-level tests nearest your change, then broader targets if the change touches shared code.

## Evidence

This playbook is derived from the package structure, route and tool contracts, store initialization patterns, and tests listed in `evidence.md`.
