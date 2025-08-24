# Kafka integration (orchestrator)

Overview

The orchestrator integrates with Kafka as a consumer for commands and a producer for responses and optional per-step messages. Topics and behavior are configurable via environment variables and application config.

Important topics

- Commands topic: default `dev.sio.orchestrator.commands` — the orchestrator reads instructions from this topic.
- Responses topic: default `dev.sio.orchestrator.responses` — used as the default reply topic for responses when a command does not override `reply_topic`.
- Per-command reply topic: the incoming command envelope can provide `reply_topic` to override where responses are written.
- DLQ topic: when a workflow fails permanently, responses are written to `<reply_topic>.dlq`.

Key environment/config values

- `KAFKA_BROKERS` (comma-separated) — e.g., `localhost:9092`.
- `KAFKA_GROUP_ID` — consumer group id.
- `KAFKA_COMMANDS_TOPIC` — commands topic (consumer).
- `KAFKA_RESPONSES_TOPIC` — default responses topic (producer default, though the orchestrator's producer does not set a default topic so messages can set per-message topics).

Message structures

CommandEnvelope (incoming command)

```json
{
  "correlation_id": "<uuid>",
  "workflow": "noop",
  "reply_topic": "dev.sio.orchestrator.responses", // optional
  "attrs": { "query": "How do I ..." }
}
```

- `correlation_id` (string, recommended): unique identifier used to correlate responses and DLQ messages with the original command.
- `workflow` (string): name/intent of the workflow to execute.
- `reply_topic` (string): optional override for where the orchestrator should publish responses for this command.
- `attrs` (object): arbitrary attributes passed to WARPP `Personalize` and `Execute` (templating uses `${A.key}`).

ResponseEnvelope (outgoing response and step_result)

```json
{
  "correlation_id": "<uuid>",
  "status": "success|error|step_result",
  "result": { /* result object */ },
  "error": "optional error string"
}
```

- Success messages use `status: "success"` and place the workflow result under `result`.
- Non-transient failures produce a message with `status: "error"` which is written to `<reply_topic>.dlq`.
- Per-step messages use `status: "step_result"` and include `result.step_id` and `result.payload` (the raw tool output serialized as string) and, optionally, `result.query` to echo the user's input.

Publishing rules

- The orchestrator's internal publisher uses per-message `Topic` when writing with the Kafka writer. The kafka writer itself is configured without a default Topic to allow specifying a Topic in each message. This avoids the kafka-go restriction that forbids setting a Topic on both the writer config and the message.
- DLQ messages are written to `<reply_topic>.dlq`.
- The orchestrator commits offsets after handling a message (either success or after publishing a DLQ entry when retries are exhausted).

Retries and transient errors

- The orchestrator worker loop will retry transient execution errors a small number of times (default 3 attempts with exponential backoff) before publishing to DLQ.
- Transient errors include network/timeouts, dedupe store failures, and producer write errors. The heuristics live in the orchestrator code (`isTransientError`).

Correlation and dedupe

- `correlation_id` is used as the dedupe key. The orchestrator stores successful responses in a dedupe store (Redis-based) to avoid re-processing repeated commands with the same correlation id.
- The dedupe TTL defaults to the workflow timeout and is configurable.

Integration test

The repository includes a simple integration test program `cmd/orchestrator/integration_test/main.go` that publishes a command and waits on the responses topic for a matching correlation id. Use it to exercise the orchestration and per-step publish behavior.

Sample flow

1. Test publishes command with `correlation_id=abc`, `workflow=noop`, `attrs.query="hello"`, `reply_topic=dev.sio.orchestrator.responses`.
2. Orchestrator consumes the command, runs the workflow.
3. If a step has `publish_result: true`, the orchestrator will publish a message to the `reply_topic` similar to:

```json
{
  "correlation_id": "abc",
  "status": "step_result",
  "result": {"step_id":"s1","payload":"{...}","query":"hello"}
}
```

4. At the end of successful execution, the orchestrator publishes the final `status: "success"` message to the `reply_topic` with a `result` object representing the summary.

Operational notes

- For high throughput scenarios consider using per-topic writer caching rather than a single writer with per-message Topic, or create dedicated writers for heavily-used topics.
- Ensure Kafka ACLs and network security allow the orchestrator to write to the configured reply topics and any DLQ topics.
- Add observability: the orchestrator initializes OpenTelemetry and logs; payload logging is configurable and may be turned off for privacy-sensitive deployments.

Troubleshooting

- "Topic must not be specified for both Writer and Message": ensure the kafka writer is created without a default `Topic` when you plan to set `Topic` on messages. The repository's orchestrator writer is configured accordingly.
- Missing query in dispatched tool: ensure the incoming `attrs` map contains `query` or `utter` (or set `echo` if you use older tests), and the workflow template uses `${A.query}` to consume it.


---

These docs are a starting point — if you want I can add an example YAML/JSON schema validator for workflow files, examples of guard expressions, or a troubleshooting checklist for common Kafka issues.
