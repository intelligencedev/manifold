// Package server provides the A2A server implementation
package server

import (
	"time"
)

// These types define the core data structures for the A2A protocol

// Task represents an A2A task
type Task struct {
	ID          string     `json:"id"`
	Status      TaskStatus `json:"status"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	CanceledAt  *time.Time `json:"canceledAt,omitempty"`
	Artifacts   []Artifact `json:"artifacts,omitempty"`
	Messages    []Message  `json:"messages,omitempty"`
}

// TaskStatus represents the status of an A2A task
type TaskStatus string

// Task status constants
const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCanceled  TaskStatus = "canceled"
)

// Message represents a message in an A2A conversation
type Message struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
	Parts     []Part    `json:"parts"`
}

// Part is a union type for different content parts in a message
type Part interface {
	GetType() string
}

// TextPart represents a text content part
type TextPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (p TextPart) GetType() string {
	return p.Type
}

// FilePart represents a file reference part
type FilePart struct {
	Type     string       `json:"type"`
	FileID   string       `json:"fileId"`
	Filename string       `json:"filename"`
	MimeType string       `json:"mimeType"`
	Content  *FileContent `json:"content,omitempty"`
}

func (p FilePart) GetType() string {
	return p.Type
}

// FileContent represents the content of a file
type FileContent struct {
	Data     []byte `json:"-"` // Not serialized to JSON
	DataURI  string `json:"dataUri,omitempty"`
	TextData string `json:"textData,omitempty"`
}

// DataPart represents a structured data part
type DataPart struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

func (p DataPart) GetType() string {
	return p.Type
}

// Artifact represents an output artifact from a task
type Artifact struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	MimeType  string    `json:"mimeType,omitempty"`
	Name      string    `json:"name,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	Data      []byte    `json:"-"` // Not serialized to JSON
	URI       string    `json:"uri,omitempty"`
	TextData  string    `json:"textData,omitempty"`
}

// PushNotificationConfig represents a push notification configuration for a task
type PushNotificationConfig struct {
	WebhookURL string            `json:"webhookUrl"`
	Headers    map[string]string `json:"headers,omitempty"`
	Events     []string          `json:"events,omitempty"`
}

// AuthenticationInfo represents authentication information for a task
type AuthenticationInfo struct {
	Type       string                 `json:"type"`
	Properties map[string]interface{} `json:"properties,omitempty"`
}

// AgentCard represents an A2A agent card for discovery
type AgentCard struct {
	Name                      string               `json:"name"`
	Description               string               `json:"description"`
	URL                       string               `json:"url"`
	Provider                  *AgentProvider       `json:"provider,omitempty"`
	Capabilities              *AgentCapabilities   `json:"capabilities,omitempty"`
	Authentication            *AgentAuthentication `json:"authentication,omitempty"`
	DefaultInputContentTypes  []string             `json:"defaultInputContentTypes,omitempty"`
	DefaultOutputContentTypes []string             `json:"defaultOutputContentTypes,omitempty"`
	Skills                    []AgentSkill         `json:"skills,omitempty"`
}

// AgentProvider represents information about the provider of an agent
type AgentProvider struct {
	Organization string `json:"organization"`
	URL          string `json:"url,omitempty"`
}

// AgentCapabilities represents the capabilities of an agent
type AgentCapabilities struct {
	Streaming              bool `json:"streaming,omitempty"`
	PushNotifications      bool `json:"pushNotifications,omitempty"`
	StateTransitionHistory bool `json:"stateTransitionHistory,omitempty"`
}

// AgentAuthentication represents the authentication requirements of an agent
type AgentAuthentication struct {
	Type string `json:"type"`
}

// AgentSkill represents a skill offered by an agent
type AgentSkill struct {
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	InputContentTypes  []string `json:"inputContentTypes,omitempty"`
	OutputContentTypes []string `json:"outputContentTypes,omitempty"`
}
