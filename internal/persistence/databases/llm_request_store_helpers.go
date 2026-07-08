package databases

import (
	"context"
	"sort"
	"strings"

	"manifold/internal/persistence"
)

func parentUserMessageIDForAssistantMessage(ctx context.Context, chat persistence.ChatStore, userID *int64, sessionID, messageID string) (string, bool, error) {
	if chat == nil || strings.TrimSpace(sessionID) == "" || strings.TrimSpace(messageID) == "" {
		return "", false, nil
	}
	messages, err := chat.ListMessages(ctx, userID, sessionID, 0)
	if err != nil {
		return "", false, err
	}
	targetIndex := -1
	for i, message := range messages {
		if strings.TrimSpace(message.ID) == strings.TrimSpace(messageID) {
			targetIndex = i
			break
		}
	}
	if targetIndex < 0 || !strings.EqualFold(strings.TrimSpace(messages[targetIndex].Role), "assistant") {
		return "", false, nil
	}
	for i := targetIndex - 1; i >= 0; i-- {
		message := messages[i]
		if !strings.EqualFold(strings.TrimSpace(message.Role), "user") {
			continue
		}
		id := strings.TrimSpace(message.ID)
		if id == "" {
			continue
		}
		return id, true, nil
	}
	return "", false, nil
}

func sortLLMRequestsByCreatedAt(out []persistence.LLMRequest) {
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
}
