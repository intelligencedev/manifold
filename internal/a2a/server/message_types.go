// Package server provides the A2A server implementation
package server

import (
	"time"
)

// ServerMessage extends the basic Message type with additional fields
// This avoids conflicts with the interfaces.Message type
type ServerMessage struct {
	ID        string    `json:"id,omitempty"`
	Role      string    `json:"role"`
	Parts     []Part    `json:"parts,omitempty"`
	CreatedAt time.Time `json:"createdAt,omitempty"`
}

// ToInterfaceMessage converts a ServerMessage to an interfaces.Message
func (sm ServerMessage) ToInterfaceMessage() Message {
	// For simplicity, just extract the first text part if available
	content := ""
	for _, part := range sm.Parts {
		if tp, ok := part.(TextPart); ok {
			content = tp.Text
			break
		}
	}

	return Message{
		Role:    sm.Role,
		Content: content,
	}
}
