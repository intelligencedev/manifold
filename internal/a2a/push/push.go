// Package push provides push notification functionality for A2A
package push

import (
	"context"
)

// PushNotificationConfig represents a push notification configuration
type PushNotificationConfig struct {
	WebhookURL string            `json:"webhookUrl"`
	Headers    map[string]string `json:"headers,omitempty"`
	Events     []string          `json:"events,omitempty"`
}

// SendPush sends a push notification to the configured webhook
func SendPush(ctx context.Context, cfg PushNotificationConfig, payload interface{}) error {
	// json.Marshal(payload), build req, set headers:
	//   Authorization (if cfg.Authentication)
	//   X-A2A-Notification-Token
	// do http.Client.Do
	return nil
}
