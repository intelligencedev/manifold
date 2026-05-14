# Diagrams Index

This folder indexes diagrams embedded throughout the wiki.

| Diagram | Type | Page |
| --- | --- | --- |
| Manifold system map | `flowchart` | `../index.md`, `../architecture/overview.md` |
| Component map | `flowchart` | `../architecture/diagrams.md` |
| Chat request | `sequenceDiagram` | `../architecture/diagrams.md`, `../flows/chat-request.md` |
| Workflow lifecycle | `stateDiagram-v2` | `../architecture/diagrams.md`, `../components/flow-v2.md` |
| Persistence overview | `erDiagram` | `../architecture/diagrams.md`, `../data/postgres-schema.md` |
| Tool model | `classDiagram` | `../architecture/diagrams.md`, `../components/tools-and-mcp.md` |
| MCP lifecycle | `sequenceDiagram`, `stateDiagram-v2` | `../flows/mcp-lifecycle.md` |
| Matrix/Pulse | `sequenceDiagram`, `stateDiagram-v2` | `../components/matrix-pulse.md`, `../flows/matrix-pulse-runtime.md` |
| Memory/RAG | `sequenceDiagram`, `flowchart`, `erDiagram` | `../components/memory-rag-transit-beliefs.md`, `../flows/memory-ingest-retrieve.md`, `../data/memory-systems.md` |

## Mermaid Selection Rule

- Structure = `flowchart`.
- Time-ordered interaction = `sequenceDiagram`.
- Database/entity relationships = `erDiagram`.
- Lifecycle = `stateDiagram-v2`.
- Interfaces/classes = `classDiagram`.
