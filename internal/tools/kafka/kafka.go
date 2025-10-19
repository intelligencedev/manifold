package kafka

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/segmentio/kafka-go"
)

// Writer interface for Kafka message writing.
type Writer interface {
	WriteMessages(ctx context.Context, msgs ...kafka.Message) error
}

type sendMessageTool struct {
	producer Writer
}

type SendMessageRequest struct {
	Topic   string                 `json:"topic"`
	Message string                 `json:"message"`
	Key     string                 `json:"key,omitempty"`
	Headers map[string]interface{} `json:"headers,omitempty"`
}

type SendMessageResponse struct {
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
	Offset  int64  `json:"offset,omitempty"`
	Segment int32  `json:"segment,omitempty"`
}

// NewSendMessageTool creates a new Kafka message sender tool.
func NewSendMessageTool(producer Writer) *sendMessageTool {
	return &sendMessageTool{producer: producer}
}

func (t *sendMessageTool) Name() string {
	return "kafka_send_message"
}

func (t *sendMessageTool) JSONSchema() map[string]any {
	return map[string]any{
		"description": "Send a message to a Kafka topic.",
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
					"description": "The message content to send",
				},
				"key": map[string]any{
					"type":        "string",
					"description": "Optional message key for partitioning",
				},
				"headers": map[string]any{
					"type":        "object",
					"description": "Optional message headers as key-value pairs",
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

	// Build message headers if provided
	var headers []kafka.Header
	for k, v := range req.Headers {
		headers = append(headers, kafka.Header{
			Key:   k,
			Value: []byte(fmt.Sprintf("%v", v)),
		})
	}

	// Create the Kafka message
	msg := kafka.Message{
		Topic:   req.Topic,
		Value:   []byte(req.Message),
		Headers: headers,
	}

	if req.Key != "" {
		msg.Key = []byte(req.Key)
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
		OK: true,
	}, nil
}
