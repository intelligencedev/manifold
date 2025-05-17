// Package server provides the A2A server implementation
package server

// MessagePart defines a part of a message in the A2A protocol
type MessagePart interface {
	GetType() string
}

// GetType returns the type of the message part
func (tp TextPart) GetType() string {
	return tp.Type
}

// Request message format used by the server's Send API
type SendMessage struct {
	Role  string        `json:"role"`
	Parts []MessagePart `json:"parts"`
}
