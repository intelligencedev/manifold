package push

import (
	"context"
	"encoding/json"
	"net/http"
)

func SendPush(ctx context.Context, cfg PushNotificationConfig, payload any) error {
	// json.Marshal(payload), build req, set headers:
	//   Authorization (if cfg.Authentication)
	//   X-A2A-Notification-Token
	// do http.Client.Do
	return nil
}