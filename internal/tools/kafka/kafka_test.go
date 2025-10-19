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

// Tests for orchestrator-aware functionality

func TestOrchestratorTopicDetection(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name              string
		topic             string
		expectedMatch     bool
		orchestratorTopic string
	}{
		{
			name:          "exact match with pattern suffix",
			topic:         "dev.manifold.orchestrator.commands",
			expectedMatch: true,
		},
		{
			name:          "different environment with pattern suffix",
			topic:         "prod.manifold.orchestrator.commands",
			expectedMatch: true,
		},
		{
			name:          "simple orchestrator.commands",
			topic:         "orchestrator.commands",
			expectedMatch: true,
		},
		{
			name:          "non-orchestrator topic",
			topic:         "regular-topic",
			expectedMatch: false,
		},
		{
			name:              "explicit orchestrator topic match",
			topic:             "my-custom-topic",
			expectedMatch:     true,
			orchestratorTopic: "my-custom-topic",
		},
		{
			name:              "explicit orchestrator topic no match",
			topic:             "other-topic",
			expectedMatch:     false,
			orchestratorTopic: "my-custom-topic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var tool *sendMessageTool
			if tt.orchestratorTopic != "" {
				tool = NewSendMessageToolWithOrchestratorTopic(&MockWriter{}, tt.orchestratorTopic)
			} else {
				tool = NewSendMessageTool(&MockWriter{})
			}

			result := tool.isOrchestratorTopic(tt.topic)
			assert.Equal(t, tt.expectedMatch, result)
		})
	}
}

func TestOrchestratorCommandEnvelopeFormatting(t *testing.T) {
	t.Parallel()
	mockWriter := &MockWriter{}
	tool := NewSendMessageToolWithOrchestratorTopic(mockWriter, "dev.manifold.orchestrator.commands")

	req := SendMessageRequest{
		Topic:      "dev.manifold.orchestrator.commands",
		Message:    "test query",
		Key:        "corr-123",
		Workflow:   "test_workflow",
		ReplyTopic: "responses-topic",
		Attrs: map[string]any{
			"custom_field": "custom_value",
		},
	}

	raw, err := json.Marshal(req)
	require.NoError(t, err)

	ctx := context.Background()
	result, err := tool.Call(ctx, raw)
	require.NoError(t, err)

	response := result.(SendMessageResponse)
	assert.True(t, response.OK)
	assert.Equal(t, "corr-123", response.CorrelationID)

	// Verify the message sent to Kafka was formatted as CommandEnvelope
	var env CommandEnvelope
	err = json.Unmarshal(mockWriter.lastMessage.Value, &env)
	require.NoError(t, err)

	assert.Equal(t, "corr-123", env.CorrelationID)
	assert.Equal(t, "test_workflow", env.Workflow)
	assert.Equal(t, "responses-topic", env.ReplyTopic)
	// Plain text messages should populate WARPP-friendly keys
	assert.Equal(t, "test query", env.Attrs["utter"])
	assert.Equal(t, "test query", env.Attrs["query"])
	assert.Equal(t, "custom_value", env.Attrs["custom_field"])
	assert.Equal(t, []byte("corr-123"), mockWriter.lastMessage.Key)
}

func TestOrchestratorCommandEnvelopeWithJSONMessage(t *testing.T) {
	t.Parallel()
	mockWriter := &MockWriter{}
	tool := NewSendMessageToolWithOrchestratorTopic(mockWriter, "orchestrator.commands")

	// Message is already JSON, should be parsed as attrs
	messageJSON := `{"query":"what is this?","context":"test"}`
	req := SendMessageRequest{
		Topic:    "orchestrator.commands",
		Message:  messageJSON,
		Key:      "msg-456",
		Workflow: "analysis",
	}

	raw, err := json.Marshal(req)
	require.NoError(t, err)

	ctx := context.Background()
	result, err := tool.Call(ctx, raw)
	require.NoError(t, err)

	response := result.(SendMessageResponse)
	assert.True(t, response.OK)

	// Verify the envelope
	var env CommandEnvelope
	err = json.Unmarshal(mockWriter.lastMessage.Value, &env)
	require.NoError(t, err)

	assert.Equal(t, "msg-456", env.CorrelationID)
	assert.Equal(t, "analysis", env.Workflow)
	assert.Equal(t, "what is this?", env.Attrs["query"])
	assert.Equal(t, "test", env.Attrs["context"])
}

func TestOrchestratorCommandEnvelopeAutoGenerateCorrelationID(t *testing.T) {
	t.Parallel()
	mockWriter := &MockWriter{}
	tool := NewSendMessageToolWithOrchestratorTopic(mockWriter, "orchestrator.commands")

	req := SendMessageRequest{
		Topic:    "orchestrator.commands",
		Message:  "test",
		Workflow: "test_workflow",
		// No Key provided, should auto-generate correlation ID
	}

	raw, err := json.Marshal(req)
	require.NoError(t, err)

	ctx := context.Background()
	result, err := tool.Call(ctx, raw)
	require.NoError(t, err)

	response := result.(SendMessageResponse)
	assert.True(t, response.OK)
	assert.NotEmpty(t, response.CorrelationID)

	// Verify the envelope has the auto-generated ID
	var env CommandEnvelope
	err = json.Unmarshal(mockWriter.lastMessage.Value, &env)
	require.NoError(t, err)

	assert.Equal(t, response.CorrelationID, env.CorrelationID)
	assert.NotEmpty(t, env.CorrelationID)
}

func TestRegularTopicPassthrough(t *testing.T) {
	t.Parallel()
	mockWriter := &MockWriter{}
	tool := NewSendMessageTool(mockWriter)

	req := SendMessageRequest{
		Topic:   "regular-topic",
		Message: "plain message",
		Key:     "key-1",
	}

	raw, err := json.Marshal(req)
	require.NoError(t, err)

	ctx := context.Background()
	result, err := tool.Call(ctx, raw)
	require.NoError(t, err)

	response := result.(SendMessageResponse)
	assert.True(t, response.OK)

	// For regular topics, message should be sent as-is (not as CommandEnvelope)
	assert.Equal(t, []byte("plain message"), mockWriter.lastMessage.Value)
	assert.Equal(t, []byte("key-1"), mockWriter.lastMessage.Key)
}
