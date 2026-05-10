package matrixgw

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

const maxSeenEventIDs = 10000

// InboundMessage is the gateway-local dispatch payload derived from a Matrix
// room event.
type InboundMessage struct {
	RoomID  string
	EventID string
	Sender  string
	Body    string
	Target  string
	Prompt  string
}

// MessageHandler receives routed Matrix messages from the gateway.
type MessageHandler interface {
	HandleMatrixMessage(context.Context, InboundMessage) error
}

// MessageHandlerFunc adapts a function into a MessageHandler.
type MessageHandlerFunc func(context.Context, InboundMessage) error

func (fn MessageHandlerFunc) HandleMatrixMessage(ctx context.Context, message InboundMessage) error {
	return fn(ctx, message)
}

type syncState struct {
	since       string
	initialized bool
	seen        map[string]struct{}
}

func newSyncState() *syncState {
	return &syncState{seen: map[string]struct{}{}}
}

func (s *Service) runLoop(ctx context.Context) {
	state := newSyncState()
	for {
		if err := s.pollOnce(ctx, state); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
				return
			}
			log.Warn().Err(err).Msg("matrix_gateway_sync_failed")
			select {
			case <-ctx.Done():
				return
			case <-time.After(s.syncRetryDelay()):
			}
		}
	}
}

func (s *Service) pollOnce(ctx context.Context, state *syncState) error {
	if s == nil || s.syncClient == nil {
		return nil
	}

	resp, err := s.syncClient.Sync(ctx, state.since, s.syncTimeoutMS(), "online")
	if err != nil {
		return err
	}
	state.since = resp.NextBatch

	for _, roomID := range resp.Invites {
		if err := s.syncClient.JoinRoom(ctx, roomID); err != nil {
			log.Warn().Str("room_id", roomID).Err(err).Msg("matrix_gateway_join_failed")
		}
	}

	if !state.initialized {
		state.initialized = true
		if !s.cfg.ProcessBacklog {
			log.Info().Msg("matrix_gateway_startup_sync_complete")
			return nil
		}
	}

	handler := s.messageHandler()
	if handler == nil {
		return nil
	}

	for roomID, events := range resp.Joined {
		roomCfg, ok := s.rooms[roomID]
		if !ok {
			continue
		}
		for _, event := range events {
			message, ok := routeEvent(state, roomID, s.cfg.UserID, roomCfg, event)
			if !ok {
				continue
			}
			if err := handler.HandleMatrixMessage(ctx, message); err != nil {
				log.Warn().Str("room_id", roomID).Str("event_id", message.EventID).Err(err).Msg("matrix_gateway_handler_failed")
			}
		}
	}

	return nil
}

func routeEvent(state *syncState, roomID, userID string, roomCfg RoomConfig, event Event) (InboundMessage, bool) {
	if event.Type != "m.room.message" {
		return InboundMessage{}, false
	}
	if seenEvent(state, event.ID) {
		return InboundMessage{}, false
	}
	if event.Sender == strings.TrimSpace(userID) {
		return InboundMessage{}, false
	}

	body := strings.TrimSpace(event.Body)
	if body == "" {
		return InboundMessage{}, false
	}

	routed, ok := RouteMessage(roomCfg, body)
	if !ok {
		return InboundMessage{}, false
	}

	return InboundMessage{
		RoomID:  roomID,
		EventID: strings.TrimSpace(event.ID),
		Sender:  strings.TrimSpace(event.Sender),
		Body:    body,
		Target:  routed.Target,
		Prompt:  routed.Prompt,
	}, true
}

func seenEvent(state *syncState, eventID string) bool {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return false
	}
	if _, ok := state.seen[eventID]; ok {
		return true
	}
	state.seen[eventID] = struct{}{}
	if len(state.seen) > maxSeenEventIDs {
		state.seen = map[string]struct{}{}
	}
	return false
}
