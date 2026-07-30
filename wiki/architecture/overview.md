# Architecture Overview (lexminify slice)

Manifold’s agent engine assembles multi-role message lists, optionally runs rolling summarization / context budgeting, then issues provider calls. **lexminify** is a late, pure-function stage on the **provider-visible** copy:

```mermaid
flowchart TB
  subgraph permanent [Persistent agent state]
    Hist[Conversation / harness history]
    Tools[Tool dispatch I/O]
  end
  subgraph compose [Per-step compose]
    Msgs[llm.Message slice]
  end
  subgraph compress [Optional lexical compress]
    LM[lexMinifyForProvider]
    Pack[lexminify package]
  end
  subgraph provider [External]
    API[LLM API]
  end
  Hist --> Msgs
  Tools --> Hist
  Msgs --> LM
  LM --> Pack
  Pack --> API
  Hist -. never rewritten by Pack .-> Hist
```

Why this cut:

- Cheap deterministic density before tokens burn
- No second model in the compression path
- Zones let experiments toggle system vs tool vs history without forked pipelines

Deeper package detail: [components/lexminify.md](../components/lexminify.md). Runtime timing: [flows/lexminify-provider-path.md](../flows/lexminify-provider-path.md).