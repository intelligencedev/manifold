# Interface: Frontend Routes

The Vue UI route table is defined in `web/agentd-ui/src/router/index.ts`.

## Route Table

| Route | View | Notes |
| --- | --- | --- |
| `/` | `OverviewView.vue` | Dashboard/overview. |
| `/projects` | `ProjectsView.vue` | Project management and files. |
| `/specialists` | `SpecialistsView.vue` | Specialists and teams configuration. |
| `/chat` | `ChatView.vue` | Main chat interface. |
| `/pulse` | `PulseView.vue` | Matrix/Pulse scheduled tasks. |
| `/matrix` | redirect to `/pulse` | Compatibility redirect. |
| `/flow` | `FlowView.vue` | Flow v2 editor/runtime. |
| `/codeqa/:runId?` | `CodeQaView.vue` | Beta nav item. |
| `/beliefs` | `BeliefsView.vue` | Beta nav item. |
| `/settings` | `SettingsView.vue` | Runtime settings. |
| `/playground` | `PlaygroundLayoutView.vue` | Nested Playground area. |
| `/playground/prompts` | `PlaygroundPromptsView.vue` | Prompt registry. |
| `/playground/prompts/:promptId` | `PlaygroundPromptDetailView.vue` | Prompt detail/versioning. |
| `/playground/datasets` | `PlaygroundDatasetsView.vue` | Dataset management. |
| `/playground/experiments` | `PlaygroundExperimentsView.vue` | Experiment list. |
| `/playground/experiments/:experimentId` | `PlaygroundExperimentDetailView.vue` | Experiment detail/runs. |
| catch-all | `NotFoundView.vue` | Fallback. |

## Feature Gates

`App.vue` includes beta nav items when `VITE_MANIFOLD_FEATURE_GATE` is `beta`. `make build-manifold-beta` passes this through the build.

## Evidence

- `web/agentd-ui/src/router/index.ts` route definitions.
- `web/agentd-ui/src/App.vue` navigation and beta gate.
- `README.md` and `Makefile` describe stable/beta build targets.
