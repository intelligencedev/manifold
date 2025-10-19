# Kafka Tool

The Kafka tool enables the agent to send messages to Apache Kafka topics. This tool integrates with the manifold agent's tool registry and can be used by the LLM to publish events, logs, or data to configured Kafka brokers.

## Features

- **Message Publishing**: Send messages to any Kafka topic
- **Message Keys**: Optional key-based partitioning for ordering guarantees
- **Headers**: Optional key-value headers for message metadata
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
| `message` | string | Yes | The message content to send |
| `key` | string | No | Optional message key for partition routing |
| `headers` | object | No | Optional key-value pairs for message headers |

### Response

The tool returns a JSON object with the following structure:

```json
{
  "ok": boolean,
  "error": "string (only present if ok is false)",
  "offset": integer (optional),
  "segment": integer (optional)
}
```

## Usage Example

### Basic Message Publishing
```json
{
  "topic": "user-events",
  "message": "{\"event\": \"user_login\", \"user_id\": 123}"
}
```

### Message with Key and Headers
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
2. **`producer.go`**: Producer factory function `NewProducerFromBrokers` for creating Kafka writers
3. **`kafka_test.go`**: Comprehensive unit tests with mock writer implementation

### Writer Interface

The tool uses a `Writer` interface to abstract Kafka implementation, making it easy to test with mocks:

```go
type Writer interface {
	WriteMessages(ctx context.Context, msgs ...kafka.Message) error
}
```

## Integration

The tool is automatically registered in `cmd/orchestrator/main.go` when Kafka brokers are configured:

```go
if cfg.Kafka.Brokers != "" {
	if producer, err := kafkatools.NewProducerFromBrokers(cfg.Kafka.Brokers); err == nil {
		registry.Register(kafkatools.NewSendMessageTool(producer))
	}
}
```

## Testing

Unit tests are provided with 100% coverage of success and error paths:

```bash
go test ./internal/tools/kafka/... -v
```

Test cases cover:
- Successful message publication
- Missing required fields (topic, message)
- Invalid JSON in request
- Tool schema validation
