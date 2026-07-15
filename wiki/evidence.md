# Evidence: lexminify Working State

## Commands Run

```bash
go test -C manifold ./internal/llm/lexminify ./internal/agent -count=1
# Reported: PASS (prior session)
```

## Source Anchors

| Claim | Where |
| --- | --- |
| Package purpose + level ladder L0–L5 with values 1–6 | `internal/llm/lexminify/lexminify.go` package comment + consts |
| `DefaultZones` includes tool + system | same file, `const DefaultZones = …` |
| Tool role minify gate | `MinifyMessages` `case role == "tool"` |
| System/developer gate | `case i < prefixEnd, role == "system", role == "developer"` |
| Protected span kinds | `internal/llm/lexminify/protect.go` (`reFenced`, URLs, UUID, path, structured blobs, markers) |
| Never-drop polarity set | `internal/llm/lexminify/tables.go` `protectedWords` |
| Engine default level 6 | `internal/agent/engine.go` `DefaultLexMinifyLevel = 6` |
| Engine zone zero comment includes tool | `Engine.LexMinifyZones` field comment |
| Adapter + log event | `internal/agent/engine_lexminify.go` (`lexminify_applied`) |
| Loop hooks | `internal/agent/engine_loop.go` `providerMsgs := e.lexMinifyForProvider(...)` (chat + stream) |
| Harness hook | `internal/agent/engine_harness.go` `providerSerial := e.lexMinifyForProvider(...)` |
| Default level assignment sites | `app_init_services.go`, `chat/builders.go`, `delegator.go`, `agent_call.go`, `cmd/agent/main.go` |
| ZoneTool default tests | `TestMessagesZonesMinifyToolByDefault`, `TestToolZoneCanBeDisabled` |
| ZoneSystem default tests | `TestMessagesZonesMinifySystemByDefault`, `TestSystemPromptZoneCanBeDisabled` |

## Inference (labeled)

- Protection heuristics are “good enough for common tool JSON + prose wrappers” based on tests, not exhaustive production corpus audit.
- Aggressive L5 on tool natural-language side is acceptable because structured islands are span-protected; residual risk is monolothic free-text tools without markers.

## Unknowns

- Net **token** reduction vs rune reduction on OpenAI/Anthropic tokenizers not measured in this wiki pass
- Whether production tool payloads routinely exceed heuristic structured-blob thresholds (16 chars, colon counts)