package kafka

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockWriter is a mock Kafka writer for testing.
type MockWriter struct {
	lastMessage kafka.Message
	shouldError bool
}

// Ensure MockWriter implements Writer interface
var _ Writer = (*MockWriter)(nil)

func (mw *MockWriter) WriteMessages(ctx context.Context, msgs ...kafka.Message) error {
	if mw.shouldError {
		return nil // In real scenario, would return an error
	}
	if len(msgs) > 0 {
		mw.lastMessage = msgs[0]
	}
	return nil
}

func TestNewSendMessageTool(t *testing.T) {
	t.Parallel()
	mockWriter := &MockWriter{}
	tool := NewSendMessageTool(mockWriter)
	assert.NotNil(t, tool)
	assert.Equal(t, "kafka_send_message", tool.Name())
}

func TestSendMessageToolName(t *testing.T) {
	t.Parallel()
	tool := NewSendMessageTool(&MockWriter{})
	assert.Equal(t, "kafka_send_message", tool.Name())
}

func TestSendMessageToolJSONSchema(t *testing.T) {
	t.Parallel()
	tool := NewSendMessageTool(&MockWriter{})
	schema := tool.JSONSchema()

	assert.NotNil(t, schema)
	assert.Contains(t, schema, "description")
	assert.Contains(t, schema, "parameters")

	params := schema["parameters"].(map[string]any)
	assert.Contains(t, params, "type")
	assert.Contains(t, params, "properties")
	assert.Contains(t, params, "required")

	required := params["required"].([]string)
	assert.Contains(t, required, "topic")
	assert.Contains(t, required, "message")
}

func TestSendMessageToolCallSuccess(t *testing.T) {
	t.Parallel()
	mockWriter := &MockWriter{}
	tool := NewSendMessageTool(mockWriter)

	req := SendMessageRequest{
		Topic:   "test-topic",
		Message: "test message",
		Key:     "test-key",
	}

	raw, err := json.Marshal(req)
	require.NoError(t, err)

	ctx := context.Background()
	result, err := tool.Call(ctx, raw)
	require.NoError(t, err)

	response := result.(SendMessageResponse)
	assert.True(t, response.OK)
	assert.Empty(t, response.Error)
}

func TestSendMessageToolCallMissingTopic(t *testing.T) {
	t.Parallel()
	tool := NewSendMessageTool(&MockWriter{})

	req := SendMessageRequest{
		Message: "test message",
	}

	raw, err := json.Marshal(req)
	require.NoError(t, err)

	ctx := context.Background()
	result, err := tool.Call(ctx, raw)
	require.NoError(t, err)

	response := result.(SendMessageResponse)
	assert.False(t, response.OK)
	assert.Contains(t, response.Error, "topic is required")
}

func TestSendMessageToolCallMissingMessage(t *testing.T) {
	t.Parallel()
	tool := NewSendMessageTool(&MockWriter{})

	req := SendMessageRequest{
		Topic: "test-topic",
	}

	raw, err := json.Marshal(req)
	require.NoError(t, err)

	ctx := context.Background()
	result, err := tool.Call(ctx, raw)
	require.NoError(t, err)

	response := result.(SendMessageResponse)
	assert.False(t, response.OK)
	assert.Contains(t, response.Error, "message is required")
}

func TestSendMessageToolCallInvalidJSON(t *testing.T) {
	t.Parallel()
	tool := NewSendMessageTool(&MockWriter{})

	ctx := context.Background()
	result, err := tool.Call(ctx, []byte("invalid json"))
	require.NoError(t, err)

	response := result.(SendMessageResponse)
	assert.False(t, response.OK)
	assert.Contains(t, response.Error, "invalid request")
}
