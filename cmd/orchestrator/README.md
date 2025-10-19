Orchestrator helper tools

This folder contains small helper programs used during local development and integration testing of the orchestrator Kafka integration.

Tools

- create_topics: small admin tool that will ensure the commands and responses topics exist on the configured Kafka broker.
  - Build/run:
    go run ./cmd/orchestrator/tools/create_topics
  - Environment variables (defaults shown):
    - KAFKA_BROKERS: localhost:9092
    - KAFKA_COMMANDS_TOPIC: dev.manifold.orchestrator.commands
    - KAFKA_RESPONSES_TOPIC: dev.manifold.orchestrator.responses

- consume_responses: short-lived consumer that prints any ResponseEnvelope messages from the responses topic for 10s.
  - Build/run:
    go run ./cmd/orchestrator/tools/consume_responses
  - Useful to verify the orchestrator is publishing step_result or final success/error messages.

Integration test

The repository includes an integration test program at `cmd/orchestrator/integration_test/main.go` which publishes a `CommandEnvelope` to the commands topic and waits for a matching `correlation_id` response on the responses topic.

Typical workflow to exercise Kafka integration locally:

1. Ensure Kafka and Redis are running (docker-compose or local services).
2. Ensure config/topcis are correct in `.env` or `config.yaml`.
3. Ensure the topics exist (create if needed):

   go run ./cmd/orchestrator/tools/create_topics

4. Start the orchestrator (loads config.yaml / .env):

   go run ./cmd/orchestrator/main.go

5. Run the integration test in a separate terminal (it will publish a command and wait for the response):

   go run ./cmd/orchestrator/integration_test -brokers localhost:9092 -commands-topic dev.manifold.orchestrator.commands -responses-topic dev.manifold.orchestrator.responses

Notes

- The orchestrator's producer writes per-message Topic values, so the response topic is not a fixed writer property. The DLQ naming scheme is `<reply_topic>.dlq`.
- If your Kafka cluster enforces ACLs and topic-auto-creation is disabled, ensure the user has permissions to create topics or pre-create topics using the create_topics tool as above.
- The create_topics tool uses kafka-go's controller-based CreateTopics API and sets 1 partition / RF=1 by default. Adjust if your environment requires a different partitioning/replication setup.
