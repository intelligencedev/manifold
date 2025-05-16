package server

// These types will be defined in the models package
// They are imported here for now

type AgentCard struct{}
type AgentProvider struct{}
type AgentCapabilities struct{}
type AgentAuthentication struct{}
type AgentSkill struct{}
type Task struct{}
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCanceled  TaskStatus = "canceled"
)

type Message struct{}
type Part struct{}
type TextPart struct{}
type FilePart struct{}
type DataPart struct{}
type FileContent struct{}
type Artifact struct{}
type PushNotificationConfig struct{}
type AuthenticationInfo struct{}
type TaskPushNotificationConfig struct{}
