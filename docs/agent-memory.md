# Agent Memory

The agent now keeps long–running conversations in durable storage while
maintaining a compact context window for every orchestrator run. This document
explains how the memory pipeline works, the data that is persisted, and how to
tune the summarisation behaviour.

## Storage Layout

Chat transcripts live in the configured `persistence.ChatStore` backend. The
store records only the user prompt and the final assistant response for each
turn. Per session we persist:

- `chat_sessions` table
  - `summary`: running synopsis used as prior context
  - `summarized_count`: number of messages covered by the summary
- `chat_messages` table: individual user/assistant messages (no tool chatter,
  deltas, or partial responses)

The Postgres implementation applies the schema automatically at start-up and
adds the new columns via `ALTER TABLE IF NOT EXISTS`; the in-memory store keeps
matching fields.

## Memory Manager

`internal/agent/memory.Manager` is responsible for assembling the conversation
history that the orchestrator sees and for keeping the summary fresh.

1. Load all persisted turns for a session.
2. If summarisation is enabled and the number of messages exceeds the
   threshold, call the LLM once to update the stored summary with the oldest
   turns (everything except the most recent `keepLast` messages).
3. Persist the new summary and the count covered.
4. Return a working context composed of:
   - A system message containing the summary (when present)
   - The most recent unsummarised user/assistant turns

The manager is invoked for every `/agent/run` and `/agent/vision` request in
`cmd/agentd`. The TUI and CLI paths can reuse the same component once they
instantiate it with the shared `ChatStore`.

## Configuration

Memory behaviour is controlled by the existing summarisation knobs in
`config.Config`:

| Setting              | Description                                             |
|----------------------|---------------------------------------------------------|
| `summaryEnabled` | Turn memory summarisation on/off. Defaults to `false`. |
| `summaryThreshold` | Minimum number of messages before we summarise (default 40). |
| `summaryKeepLast` | How many of the newest messages stay verbatim (default 12). |
| `openai.summaryModel` / `OPENAI_SUMMARY_MODEL` | Optional model override used only for summaries. Defaults to the primary `openai.model`. |
| `openai.summaryBaseURL` / `OPENAI_SUMMARY_URL` | Optional base URL override for summary requests. Defaults to the primary `openai.baseURL`. |

The CLI flags and YAML/env values described in `config.yaml.example` still
apply. When summarisation is disabled the manager simply returns all stored
messages verbatim.

## Operational Notes

- Summaries are generated with the dedicated summary provider, defaulting to
  the main `openai.model`/`openai.baseURL` unless the summary overrides are
  set.
- Summarisation failures (e.g. LLM error) leave the existing summary untouched
  and fall back to the full transcript.
- Because only user prompts and final responses are stored, session replays are
  deterministic and do not include tool traces or partial delta events.
- The `/api/chat/sessions` endpoints expose the `summary` and
  `summarizedCount` fields for debugging; the web UI can decide whether to show
  them.

With this pipeline the agent keeps memory across restarts while staying inside
model context limits and avoiding redundant tool chatter in long conversations.
