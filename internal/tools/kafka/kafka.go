package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
)

// Writer interface for Kafka message writing.
type Writer interface {
	WriteMessages(ctx context.Context, msgs ...kafka.Message) error
}

type sendMessageTool struct {
	producer              Writer
	orchestratorTopicName string // e.g., "dev.manifold.orchestrator.commands"
}

type SendMessageRequest struct {
	Topic   string                 `json:"topic"`
	Message string                 `json:"message"`
	Key     string                 `json:"key,omitempty"`
	Headers map[string]interface{} `json:"headers,omitempty"`
	// Orchestrator-specific fields (only used when topic is orchestrator commands topic)
	Workflow   string         `json:"workflow,omitempty"`
	ReplyTopic string         `json:"reply_topic,omitempty"`
	Attrs      map[string]any `json:"attrs,omitempty"`
}

type SendMessageResponse struct {
	OK            bool   `json:"ok"`
	Error         string `json:"error,omitempty"`
	CorrelationID string `json:"correlation_id,omitempty"`
	Offset        int64  `json:"offset,omitempty"`
	Segment       int32  `json:"segment,omitempty"`
}

// CommandEnvelope matches the orchestrator's expected message format
type CommandEnvelope struct {
	CorrelationID string         `json:"correlation_id"`
	Workflow      string         `json:"workflow,omitempty"`
	ReplyTopic    string         `json:"reply_topic,omitempty"`
	Attrs         map[string]any `json:"attrs,omitempty"`
}

// NewSendMessageTool creates a new Kafka message sender tool.
// orchestratorTopicName should be the full topic name for orchestrator commands
// (e.g., "dev.manifold.orchestrator.commands"). If not provided, defaults to
// "orchestrator.commands" pattern matching.
func NewSendMessageTool(producer Writer) *sendMessageTool {
	return &sendMessageTool{
		producer:              producer,
		orchestratorTopicName: "", // Will be detected dynamically or set via environment
	}
}

// NewSendMessageToolWithOrchestratorTopic creates a tool with explicit orchestrator topic name.
func NewSendMessageToolWithOrchestratorTopic(producer Writer, orchestratorTopic string) *sendMessageTool {
	return &sendMessageTool{
		producer:              producer,
		orchestratorTopicName: orchestratorTopic,
	}
}

func (t *sendMessageTool) Name() string {
	return "kafka_send_message"
}

func (t *sendMessageTool) JSONSchema() map[string]any {
	return map[string]any{
		"description": "Send a message to a Kafka topic. When sending to the orchestrator commands topic, automatically formats the message as a CommandEnvelope.",
		"parameters": map[string]any{
			"type":     "object",
			"required": []string{"topic", "message"},
			"properties": map[string]any{
				"topic": map[string]any{
					"type":        "string",
					"description": "The Kafka topic to send the message to",
				},
				"message": map[string]any{
					"type":        "string",
					"description": "The message content to send. For orchestrator commands topic, this becomes the workflow input or attributes.",
				},
				"key": map[string]any{
					"type":        "string",
					"description": "Optional message key for partitioning. For orchestrator commands, this is used as the correlation_id if not explicitly provided.",
				},
				"headers": map[string]any{
					"type":        "object",
					"description": "Optional message headers as key-value pairs",
				},
				"workflow": map[string]any{
					"type":        "string",
					"description": "Optional workflow name (only for orchestrator commands topic)",
				},
				"reply_topic": map[string]any{
					"type":        "string",
					"description": "Optional reply topic for responses (only for orchestrator commands topic)",
				},
				"attrs": map[string]any{
					"type":        "object",
					"description": "Optional workflow attributes (only for orchestrator commands topic)",
				},
			},
		},
	}
}

func (t *sendMessageTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var req SendMessageRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return SendMessageResponse{
			OK:    false,
			Error: fmt.Sprintf("invalid request: %v", err),
		}, nil
	}

	if req.Topic == "" {
		return SendMessageResponse{
			OK:    false,
			Error: "topic is required",
		}, nil
	}

	if req.Message == "" {
		return SendMessageResponse{
			OK:    false,
			Error: "message is required",
		}, nil
	}

	// Check if this is an orchestrator commands topic
	isOrchestratorTopic := t.isOrchestratorTopic(req.Topic)

	// Build message headers if provided
	var headers []kafka.Header
	for k, v := range req.Headers {
		headers = append(headers, kafka.Header{
			Key:   k,
			Value: []byte(fmt.Sprintf("%v", v)),
		})
	}

	// Determine the actual message value and key
	var msgValue []byte
	var msgKey []byte

	if isOrchestratorTopic {
		// For orchestrator topics, format as CommandEnvelope
		corrID := req.Key
		if corrID == "" {
			// Auto-generate correlation ID if not provided
			corrID = t.generateCorrelationID()
		}

		// Try to parse message as JSON attrs, otherwise map plain text to WARPP-friendly keys
		var attrs map[string]any
		if err := json.Unmarshal([]byte(req.Message), &attrs); err == nil {
			// Message was valid JSON, use as attrs
		} else {
			// Message is plain text. WARPP personalization expects either
			// "utter" or "query" to be present. Populate both for
			// maximum compatibility, so downstream steps using ${A.query}
			// or ${A.utter} both work.
			attrs = map[string]any{
				"utter": req.Message,
				"query": req.Message,
			}
		}

		// Merge with explicitly provided attrs
		for k, v := range req.Attrs {
			attrs[k] = v
		}

		// Build CommandEnvelope
		env := CommandEnvelope{
			CorrelationID: corrID,
			Workflow:      req.Workflow,
			ReplyTopic:    req.ReplyTopic,
			Attrs:         attrs,
		}

		payload, err := json.Marshal(env)
		if err != nil {
			return SendMessageResponse{
				OK:    false,
				Error: fmt.Sprintf("failed to marshal command envelope: %v", err),
			}, nil
		}
		msgValue = payload
		msgKey = []byte(corrID)
	} else {
		// For regular topics, send message as-is
		msgValue = []byte(req.Message)
		if req.Key != "" {
			msgKey = []byte(req.Key)
		}
	}

	// Create the Kafka message
	msg := kafka.Message{
		Topic:   req.Topic,
		Value:   msgValue,
		Headers: headers,
		Key:     msgKey,
	}

	// Send the message
	err := t.producer.WriteMessages(ctx, msg)
	if err != nil {
		return SendMessageResponse{
			OK:    false,
			Error: fmt.Sprintf("failed to send message: %v", err),
		}, nil
	}

	return SendMessageResponse{
		OK:            true,
		CorrelationID: string(msgKey),
	}, nil
}

// isOrchestratorTopic checks if the topic name matches the orchestrator commands topic pattern
func (t *sendMessageTool) isOrchestratorTopic(topic string) bool {
	// If explicit orchestrator topic is configured, use exact match
	if t.orchestratorTopicName != "" {
		return topic == t.orchestratorTopicName
	}

	// Default pattern matching: topic ends with ".orchestrator.commands"
	return strings.HasSuffix(topic, ".orchestrator.commands") || topic == "orchestrator.commands"
}

// generateCorrelationID generates a unique correlation ID
func (t *sendMessageTool) generateCorrelationID() string {
	return uuid.New().String()
}
