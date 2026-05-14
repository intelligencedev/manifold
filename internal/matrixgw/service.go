package matrixgw

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"manifold/internal/config"
)

// Service owns the lifecycle and validated room configuration for the built-in
// Matrix gateway.
type Service struct {
	cfg        config.MatrixConfig
	rooms      map[string]RoomConfig
	syncClient SyncClient
	handler    MessageHandler
	outbound   func(context.Context, OutboundMessage) error
	mu         sync.Mutex
	cancel     context.CancelFunc
	done       chan struct{}
}

type OutboundMessage struct {
	RoomID        string
	Target        string
	Body          string
	FormattedBody string
	MsgType       string
	MediaURL      string
	MediaMIME     string
	MediaSize     int64
}

// New validates the Matrix gateway configuration and prepares room routing.
func New(cfg config.MatrixConfig) (*Service, error) {
	service := &Service{cfg: cfg}
	if !cfg.Enabled {
		return service, nil
	}

	if strings.TrimSpace(cfg.HomeserverURL) == "" {
		return nil, fmt.Errorf("matrix gateway requires homeserverURL when enabled")
	}
	if strings.TrimSpace(cfg.UserID) == "" {
		return nil, fmt.Errorf("matrix gateway requires userID when enabled")
	}
	if strings.TrimSpace(cfg.AccessToken) == "" {
		return nil, fmt.Errorf("matrix gateway requires accessToken when enabled")
	}

	rooms, err := buildRooms(cfg.Rooms)
	if err != nil {
		return nil, err
	}
	if len(rooms) == 0 {
		return nil, fmt.Errorf("matrix gateway requires at least one configured room when enabled")
	}
	syncClient, err := NewSyncClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("create matrix sync client: %w", err)
	}

	service.rooms = rooms
	service.syncClient = syncClient
	return service, nil
}

// Start begins the gateway lifecycle and starts the Matrix sync loop.
func (s *Service) Start(ctx context.Context) error {
	if s == nil || !s.cfg.Enabled {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return nil
	}

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	s.cancel = cancel
	s.done = done

	go func() {
		defer close(done)
		log.Info().
			Str("user_id", s.cfg.UserID).
			Int("rooms", len(s.rooms)).
			Msg("matrix_gateway_initialized")
		s.runLoop(runCtx)
	}()

	return nil
}

// Close stops the gateway lifecycle.
func (s *Service) Close() error {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	cancel := s.cancel
	done := s.done
	s.cancel = nil
	s.done = nil
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}

	return nil
}

// Rooms returns a copy of the validated room routing table.
func (s *Service) Rooms() map[string]RoomConfig {
	if s == nil || len(s.rooms) == 0 {
		return nil
	}

	out := make(map[string]RoomConfig, len(s.rooms))
	for roomID, cfg := range s.rooms {
		mentions := make(map[string]string, len(cfg.Mentions))
		for alias, target := range cfg.Mentions {
			mentions[alias] = target
		}
		out[roomID] = RoomConfig{
			DefaultTarget:    cfg.DefaultTarget,
			AllowUnmentioned: cfg.AllowUnmentioned,
			Mentions:         mentions,
		}
	}

	return out
}

func buildRooms(rooms []config.MatrixRoomConfig) (map[string]RoomConfig, error) {
	indexed := make(map[string]RoomConfig, len(rooms))
	for _, room := range rooms {
		roomID := strings.TrimSpace(room.RoomID)
		if roomID == "" {
			return nil, fmt.Errorf("matrix gateway roomID is required")
		}
		if _, exists := indexed[roomID]; exists {
			return nil, fmt.Errorf("matrix gateway room %q is configured more than once", roomID)
		}

		mentions := make(map[string]string, len(room.Mentions))
		for alias, target := range room.Mentions {
			alias = strings.TrimSpace(alias)
			target = strings.TrimSpace(target)
			if alias == "" || target == "" {
				continue
			}
			mentions[alias] = target
		}

		indexed[roomID] = RoomConfig{
			DefaultTarget:    strings.TrimSpace(room.DefaultTarget),
			AllowUnmentioned: room.AllowUnmentioned,
			Mentions:         mentions,
		}
	}

	return indexed, nil
}

// SetHandler installs the Matrix message callback invoked for routed events.
func (s *Service) SetHandler(handler MessageHandler) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handler = handler
}

// SetSyncClient overrides the Matrix sync client. Intended for tests.
func (s *Service) SetSyncClient(client SyncClient) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.syncClient = client
}

func (s *Service) SetOutboundRecorder(recorder func(context.Context, OutboundMessage) error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outbound = recorder
}

func (s *Service) messageHandler() MessageHandler {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.handler
}

func (s *Service) outboundRecorder() func(context.Context, OutboundMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.outbound
}

func (s *Service) syncTimeoutMS() int {
	if s == nil || s.cfg.SyncTimeoutSeconds <= 0 {
		return 30000
	}
	return s.cfg.SyncTimeoutSeconds * 1000
}

func (s *Service) syncRetryDelay() time.Duration {
	if s == nil || s.cfg.SyncRetryDelaySeconds <= 0 {
		return 3 * time.Second
	}
	return time.Duration(s.cfg.SyncRetryDelaySeconds) * time.Second
}
