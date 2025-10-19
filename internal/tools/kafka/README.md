# Kafka Tool

The Kafka tool enables the agent to send messages to Apache Kafka topics. This tool integrates with the manifold agent's tool registry and can be used by the LLM to publish events, logs, or data to configured Kafka brokers.

## Features

- **Message Publishing**: Send messages to any Kafka topic
- **Message Keys**: Optional key-based partitioning for ordering guarantees
- **Headers**: Optional key-value headers for message metadata
- **Orchestrator Integration**: Automatic intelligent formatting for orchestrator commands topics (auto-converts to `CommandEnvelope` format)
- **Error Handling**: Graceful error reporting with structured responses
- **Interface-Based Design**: Uses a `Writer` interface for easy testing and mocking

## Configuration

The Kafka tool is configured via environment variables:

- `KAFKA_BROKERS` or `KAFKA_BOOTSTRAP_SERVERS`: Comma-separated list of Kafka broker addresses (default: `localhost:9092`)

Example:
```bash
export KAFKA_BROKERS=broker1:9092,broker2:9092,broker3:9092
```

## Tool Schema

### Tool Name

`kafka_send_message`

### Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `topic` | string | Yes | The Kafka topic to send the message to |
| `message` | string | Yes | The message content to send. For orchestrator topics, becomes workflow input or attributes. |
| `key` | string | No | Optional message key for partition routing. For orchestrator topics, used as correlation_id if not explicitly provided. |
| `headers` | object | No | Optional key-value pairs for message headers |
| `workflow` | string | No | Workflow name (orchestrator topics only) |
| `reply_topic` | string | No | Reply topic for responses (orchestrator topics only) |
| `attrs` | object | No | Workflow attributes (orchestrator topics only) |

### Response

The tool returns a JSON object with the following structure:

```json
{
  "ok": boolean,
  "error": "string (only present if ok is false)",
  "correlation_id": "string (for orchestrator commands)",
  "offset": integer (optional),
  "segment": integer (optional)
}
```

## Orchestrator Integration

The `kafka_send_message` tool has built-in intelligence to detect when sending to orchestrator commands topics and automatically formats messages as `CommandEnvelope` structures.

### Topic Detection

The tool automatically detects orchestrator topics by pattern matching:
- Default pattern: topics ending with `.orchestrator.commands` (e.g., `dev.manifold.orchestrator.commands`, `prod.manifold.orchestrator.commands`)
- Simple match: `orchestrator.commands`
- Explicit configuration: Via `NewSendMessageToolWithOrchestratorTopic(producer, topicName)`

### Message Formatting

When sending to an orchestrator topic:

1. If no `key` is provided, a UUID is auto-generated as the `correlation_id`
2. The `message` field is intelligently handled:
   - If it's valid JSON, it's parsed and merged with `attrs`
   - If it's plain text, it's wrapped as `{"message": "..."}` in attrs
3. All fields are wrapped in a `CommandEnvelope`:
   ```json
   {
     "correlation_id": "searchop",
     "workflow": "kafka_op",
     "reply_topic": "responses",
     "attrs": {
       "message": "what is manifold?",
       "custom_field": "value"
     }
   }
   ```

### Usage Examples

#### Sending to Regular Kafka Topic

```json
{
  "topic": "user-events",
  "message": "{\"event\": \"user_login\", \"user_id\": 123}",
  "key": "user-789"
}
```

#### Sending to Orchestrator Commands Topic (Plain Text Message)

```json
{
  "topic": "dev.manifold.orchestrator.commands",
  "message": "what is the manifold project?",
  "key": "search-query-1",
  "workflow": "research",
  "reply_topic": "dev.manifold.orchestrator.responses"
}
```

This automatically becomes:

```json
{
  "correlation_id": "search-query-1",
  "workflow": "research",
  "reply_topic": "dev.manifold.orchestrator.responses",
  "attrs": {
    "message": "what is the manifold project?"
  }
}
```

#### Sending to Orchestrator with JSON Attributes

```json
{
  "topic": "orchestrator.commands",
  "message": "{\"query\": \"analyze this\", \"mode\": \"deep\"}",
  "key": "analysis-task-123",
  "workflow": "analysis",
  "attrs": {
    "priority": "high"
  }
}
```

This becomes:

```json
{
  "correlation_id": "analysis-task-123",
  "workflow": "analysis",
  "attrs": {
    "query": "analyze this",
    "mode": "deep",
    "priority": "high"
  }
}
```

## Usage Example

### Basic Message Publishing with Message Key and Headers

```json
{
  "topic": "order-events",
  "message": "{\"order_id\": 456, \"total\": 99.99}",
  "key": "user-789",
  "headers": {
    "source": "api",
    "version": "1.0"
  }
}
```

## Implementation Details

### Key Components

1. **`kafka.go`**: Main tool implementation with `sendMessageTool` struct implementing the `Tool` interface
   - Orchestrator topic detection with `isOrchestratorTopic()`
   - Automatic `CommandEnvelope` formatting
   - UUID-based correlation ID generation
2. **`producer.go`**: Producer factory function `NewProducerFromBrokers` for creating Kafka writers
3. **`kafka_test.go`**: Comprehensive unit tests (18 tests) with full coverage of orchestrator features

### Writer Interface

The tool uses a `Writer` interface to abstract Kafka implementation, making it easy to test with mocks:

```go
type Writer interface {
	WriteMessages(ctx context.Context, msgs ...kafka.Message) error
}
```

### Orchestrator Support

Two constructors are available:

```go
// Auto-detect orchestrator topics by pattern matching
NewSendMessageTool(producer Writer) *sendMessageTool

// Explicit orchestrator topic configuration
NewSendMessageToolWithOrchestratorTopic(producer Writer, orchestratorTopic string) *sendMessageTool
```

## Integration

The tool is automatically registered:

- **`cmd/orchestrator/main.go`**: Creates tool with explicit orchestrator topic for intelligent formatting
- **`internal/agentd/run.go`**: Creates tool with pattern-based orchestrator detection

Example from orchestrator:

```go
if cfg.Kafka.Brokers != "" {
	if producer, err := kafkatools.NewProducerFromBrokers(cfg.Kafka.Brokers); err == nil {
		registry.Register(kafkatools.NewSendMessageToolWithOrchestratorTopic(producer, commandsTopic))
	}
}
```

## Testing

Comprehensive unit tests (18 total) with 100% coverage:

```bash
go test ./internal/tools/kafka/... -v
```

Test cases cover:

- Successful message publication
- Missing required fields (topic, message)
- Invalid JSON in request
- Orchestrator topic detection (pattern matching, explicit config)
- CommandEnvelope formatting with plain text messages
- CommandEnvelope formatting with JSON messages
- Auto-generated correlation IDs
- Attribute merging
- Regular topic passthrough
