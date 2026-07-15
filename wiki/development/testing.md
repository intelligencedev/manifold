# Testing: lexminify And Agent Engines

## Focused Validation

```bash
go test -C manifold ./internal/llm/lexminify ./internal/agent -count=1
```

Optional wider surface (construction sites / tool agents):

```bash
go test -C manifold ./internal/tools/agents ./internal/agentd/chat -count=1
```

## What To Re-Run When Changing What

| Change | Minimum revalidation |
| --- | --- |
| Transform tables / vowel / stopwords | `./internal/llm/lexminify` |
| Protect scanners | protections tests + tool default test |
| Zone defaults / bitmask meaning | zone enable/disable tests |
| Engine adapter logging | `./internal/agent` |
| Hook sites | agent package + harness path tests if present |

## Guardrails Encoded In Tests

- Fenced JSON / inline code / URL / UUID survive minification
- Tool default-on removes filler but keeps JSON fragment
- Tool zone can be disabled while history still minifies
- System zone can be disabled while history still minifies
- Current request retains meaning words like `without` under default zones
- Same string twice ⇒ identical minified output