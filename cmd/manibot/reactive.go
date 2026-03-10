package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"manifold/internal/persistence"

	"github.com/google/uuid"
	"github.com/matrix-org/gomatrix"
)

type matrixTypingSender interface {
	UserTyping(roomID string, typing bool, timeout int64) (*gomatrix.RespTyping, error)
}

func handleDirectMessage(matrixClient matrixMessageSender, httpClient *http.Client, cfg config, pulseStore persistence.PulseStore, roomID, prompt string) {
	sessionID := sessionIDForRoom(cfg.ManifoldSessionPrefix, roomID)
	projectID := resolveRoomProjectID(context.Background(), pulseStore, roomID, cfg.ManifoldProjectID)
	response, err := callManifold(httpClient, cfg, roomID, sessionID, projectID, prompt)
	if err != nil {
		log.Printf("manifold prompt error (room=%s session=%s): %v", roomID, sessionID, err)
		_ = sendMatrixMessage(matrixClient, roomID, "Sorry, I hit an upstream error talking to Manifold.")
		return
	}

	if err := sendMatrixMessage(matrixClient, roomID, response.Result); err != nil {
		log.Printf("send message error (room=%s): %v", roomID, err)
	}
}

func handleReactiveMessage(matrixClient *gomatrix.Client, httpClient *http.Client, cfg config, pulseStore persistence.PulseStore, claimStore persistence.ReactiveClaimStore, history *roomEventHistory, roomID string, event gomatrix.Event, body, prompt string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.ReactiveWaitSeconds)*time.Second)
	defer cancel()

	claimToken, claimed, err := acquireReactiveLease(ctx, claimStore, roomID, cfg.MatrixBotUserID, event.ID, time.Duration(cfg.ReactiveLeaseSeconds)*time.Second)
	if err != nil {
		log.Printf("reactive claim error (room=%s event=%s): %v", roomID, event.ID, err)
		handleDirectMessage(matrixClient, httpClient, cfg, pulseStore, roomID, prompt)
		return
	}
	if !claimed {
		return
	}
	defer func() {
		if err := claimStore.Release(context.Background(), roomID, claimToken); err != nil && err != persistence.ErrNotFound {
			log.Printf("reactive release error (room=%s event=%s): %v", roomID, event.ID, err)
		}
	}()

	recent := history.Snapshot(roomID, cfg.ReactiveContextMessages)
	if !shouldRespondAfterLease(event, recent, cfg.MatrixBotUserID, strings.TrimSpace(cfg.BotPrefix) != "") {
		return
	}

	setTyping(matrixClient, roomID, true)
	defer setTyping(matrixClient, roomID, false)

	reactivePrompt := buildReactivePrompt(body, prompt, recent)
	handleDirectMessage(matrixClient, httpClient, cfg, pulseStore, roomID, reactivePrompt)
}

func acquireReactiveLease(ctx context.Context, store persistence.ReactiveClaimStore, roomID, botID, eventID string, leaseDuration time.Duration) (string, bool, error) {
	if store == nil {
		return "", false, persistence.ErrNotFound
	}
	for {
		claimToken := uuid.NewString()
		claimed, err := store.TryClaim(ctx, roomID, botID, claimToken, eventID, time.Now().UTC().Add(leaseDuration))
		if err != nil {
			return "", false, err
		}
		if claimed {
			return claimToken, true, nil
		}

		waitFor := 2 * time.Second
		claim, err := store.GetActiveClaim(ctx, roomID)
		if err != nil && err != persistence.ErrNotFound {
			return "", false, err
		}
		if err == nil {
			remaining := time.Until(claim.ExpiresAt)
			if remaining > 0 && remaining < waitFor {
				waitFor = remaining
			}
		}
		if waitFor < 200*time.Millisecond {
			waitFor = 200 * time.Millisecond
		}

		timer := time.NewTimer(waitFor)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", false, nil
		case <-timer.C:
		}
	}
}

func shouldRespondAfterLease(trigger gomatrix.Event, recent []gomatrix.Event, botUserID string, explicitlyAddressed bool) bool {
	if explicitlyAddressed {
		return true
	}

	for _, event := range recent {
		if event.Type != "m.room.message" {
			continue
		}
		if event.ID == trigger.ID {
			continue
		}
		if event.Timestamp < trigger.Timestamp {
			continue
		}
		if strings.TrimSpace(textFromEvent(event)) == "" {
			continue
		}
		if event.Sender == botUserID {
			continue
		}
		return false
	}

	return true
}

func buildReactivePrompt(body, prompt string, recent []gomatrix.Event) string {
	var b strings.Builder
	b.WriteString("[matrix reactive mode]\n")
	b.WriteString("You are responding in a live Matrix room. Recent room context is included below.\n")
	b.WriteString("Use it to avoid repeating what was already said and to stay aligned with the current conversation.\n\n")
	if len(recent) > 0 {
		b.WriteString("Recent messages (oldest first):\n")
		for _, event := range recent {
			text := strings.TrimSpace(textFromEvent(event))
			if event.Type != "m.room.message" || text == "" {
				continue
			}
			b.WriteString(fmt.Sprintf("- %s: %s\n", strings.TrimSpace(event.Sender), text))
		}
		b.WriteString("\n")
	}
	b.WriteString("Current Matrix message:\n")
	b.WriteString(strings.TrimSpace(body))
	b.WriteString("\n\n")
	if strings.TrimSpace(prompt) != "" && strings.TrimSpace(prompt) != strings.TrimSpace(body) {
		b.WriteString("Extracted prompt for you:\n")
		b.WriteString(strings.TrimSpace(prompt))
		b.WriteString("\n\n")
	}
	b.WriteString("Respond naturally and only for yourself. Do not claim actions taken by other bots.\n")
	return b.String()
}

func setTyping(client matrixTypingSender, roomID string, typing bool) {
	if client == nil {
		return
	}
	timeout := int64(0)
	if typing {
		timeout = int64((30 * time.Second) / time.Millisecond)
	}
	if _, err := client.UserTyping(roomID, typing, timeout); err != nil {
		log.Printf("matrix typing error (room=%s typing=%t): %v", roomID, typing, err)
	}
}

func textFromEvent(event gomatrix.Event) string {
	body, _ := event.Content["body"].(string)
	return strings.TrimSpace(body)
}
