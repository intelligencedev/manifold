package databases

import (
	"context"
	"manifold/internal/persistence"
	pulsecore "manifold/internal/pulse"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type memPulseStore struct {
	mu    sync.RWMutex
	rooms map[string]persistence.PulseRoom
	tasks map[string]map[string]persistence.PulseTask
}

func (s *memPulseStore) Init(ctx context.Context) error { return nil }

func pulseScopeKey(roomID, routeTarget string) string {
	return strings.TrimSpace(roomID) + "\x00" + strings.TrimSpace(routeTarget)
}

func (s *memPulseStore) EnsureRoom(ctx context.Context, roomID, routeTarget string) (persistence.PulseRoom, error) {
	roomID = strings.TrimSpace(roomID)
	routeTarget = strings.TrimSpace(routeTarget)
	if roomID == "" {
		return persistence.PulseRoom{}, persistence.ErrNotFound
	}
	scopeKey := pulseScopeKey(roomID, routeTarget)

	s.mu.Lock()
	defer s.mu.Unlock()
	if room, ok := s.rooms[scopeKey]; ok {
		return clonePulseRoom(room), nil
	}
	now := time.Now().UTC()
	room := persistence.PulseRoom{
		RoomID:      roomID,
		RouteTarget: routeTarget,
		Enabled:     true,
		Revision:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.rooms[scopeKey] = room
	return clonePulseRoom(room), nil
}

func (s *memPulseStore) GetRoom(ctx context.Context, roomID, routeTarget string) (persistence.PulseRoom, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	room, ok := s.rooms[pulseScopeKey(roomID, routeTarget)]
	if !ok {
		return persistence.PulseRoom{}, persistence.ErrNotFound
	}
	return clonePulseRoom(room), nil
}

func (s *memPulseStore) ListRooms(ctx context.Context, routeTarget string) ([]persistence.PulseRoom, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	routeTarget = strings.TrimSpace(routeTarget)
	out := make([]persistence.PulseRoom, 0, len(s.rooms))
	for _, room := range s.rooms {
		if routeTarget != "" && room.RouteTarget != routeTarget {
			continue
		}
		out = append(out, clonePulseRoom(room))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].RoomID == out[j].RoomID {
			return out[i].RouteTarget < out[j].RouteTarget
		}
		return out[i].RoomID < out[j].RoomID
	})
	return out, nil
}

func (s *memPulseStore) UpsertRoom(ctx context.Context, room persistence.PulseRoom) (persistence.PulseRoom, error) {
	roomID := strings.TrimSpace(room.RoomID)
	routeTarget := strings.TrimSpace(room.RouteTarget)
	if roomID == "" {
		return persistence.PulseRoom{}, persistence.ErrNotFound
	}
	scopeKey := pulseScopeKey(roomID, routeTarget)

	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	existing, ok := s.rooms[scopeKey]
	if ok {
		room.CreatedAt = existing.CreatedAt
		room.Revision = existing.Revision + 1
		if room.ActiveClaimToken == "" {
			room.ActiveClaimToken = existing.ActiveClaimToken
			room.ActiveClaimUntil = existing.ActiveClaimUntil
		}
		if room.LastPulseAttemptAt.IsZero() {
			room.LastPulseAttemptAt = existing.LastPulseAttemptAt
		}
		if room.LastPulseCompletedAt.IsZero() {
			room.LastPulseCompletedAt = existing.LastPulseCompletedAt
		}
		if room.LastPulseSummary == "" {
			room.LastPulseSummary = existing.LastPulseSummary
		}
		if room.LastPulseError == "" {
			room.LastPulseError = existing.LastPulseError
		}
	} else {
		room.CreatedAt = now
		room.Revision = 1
	}
	room.RoomID = roomID
	room.RouteTarget = routeTarget
	room.UpdatedAt = now
	s.rooms[scopeKey] = room
	return clonePulseRoom(room), nil
}

func (s *memPulseStore) ListTasks(ctx context.Context, roomID, routeTarget string) ([]persistence.PulseTask, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	roomTasks := s.tasks[pulseScopeKey(roomID, routeTarget)]
	out := make([]persistence.PulseTask, 0, len(roomTasks))
	for _, task := range roomTasks {
		out = append(out, clonePulseTask(task))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *memPulseStore) UpsertTask(ctx context.Context, task persistence.PulseTask) (persistence.PulseTask, error) {
	roomID := strings.TrimSpace(task.RoomID)
	routeTarget := strings.TrimSpace(task.RouteTarget)
	if roomID == "" {
		return persistence.PulseTask{}, persistence.ErrNotFound
	}
	if _, err := s.EnsureRoom(ctx, roomID, routeTarget); err != nil {
		return persistence.PulseTask{}, err
	}
	scopeKey := pulseScopeKey(roomID, routeTarget)

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tasks[scopeKey] == nil {
		s.tasks[scopeKey] = map[string]persistence.PulseTask{}
	}
	now := time.Now().UTC()
	if strings.TrimSpace(task.ID) == "" {
		task.ID = uuid.NewString()
	}
	task.RoomID = roomID
	task.RouteTarget = routeTarget
	existing, ok := s.tasks[scopeKey][task.ID]
	if ok {
		task.CreatedAt = existing.CreatedAt
		if task.LastRunAt.IsZero() {
			task.LastRunAt = existing.LastRunAt
		}
		if task.LastResultSummary == "" {
			task.LastResultSummary = existing.LastResultSummary
		}
	} else {
		task.CreatedAt = now
	}
	var err error
	task, err = pulsecore.NormalizeTaskSchedule(task)
	if err != nil {
		return persistence.PulseTask{}, err
	}
	task.UpdatedAt = now
	s.tasks[scopeKey][task.ID] = task
	return clonePulseTask(task), nil
}

func (s *memPulseStore) DeleteTask(ctx context.Context, roomID, routeTarget, taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	roomTasks := s.tasks[pulseScopeKey(roomID, routeTarget)]
	if roomTasks == nil {
		return persistence.ErrNotFound
	}
	if _, ok := roomTasks[strings.TrimSpace(taskID)]; !ok {
		return persistence.ErrNotFound
	}
	delete(roomTasks, strings.TrimSpace(taskID))
	return nil
}

func (s *memPulseStore) ClaimRoom(ctx context.Context, roomID, routeTarget, token string, leaseUntil time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	room, ok := s.rooms[pulseScopeKey(roomID, routeTarget)]
	if !ok {
		return false, persistence.ErrNotFound
	}
	now := time.Now().UTC()
	if room.ActiveClaimToken != "" && room.ActiveClaimToken != token && room.ActiveClaimUntil.After(now) {
		return false, nil
	}
	room.ActiveClaimToken = token
	room.ActiveClaimUntil = leaseUntil.UTC()
	room.LastPulseAttemptAt = now
	room.UpdatedAt = now
	room.Revision++
	s.rooms[pulseScopeKey(room.RoomID, room.RouteTarget)] = room
	return true, nil
}

func (s *memPulseStore) ClearRoomClaim(ctx context.Context, roomID, routeTarget string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	room, ok := s.rooms[pulseScopeKey(roomID, routeTarget)]
	if !ok {
		return persistence.ErrNotFound
	}
	now := time.Now().UTC()
	room.ActiveClaimToken = ""
	room.ActiveClaimUntil = time.Time{}
	room.UpdatedAt = now
	room.Revision++
	s.rooms[pulseScopeKey(room.RoomID, room.RouteTarget)] = room
	return nil
}

func (s *memPulseStore) CompleteRoomPulse(ctx context.Context, completion persistence.RoomPulseCompletion) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	scopeKey := pulseScopeKey(completion.RoomID, completion.RouteTarget)
	room, ok := s.rooms[scopeKey]
	if !ok {
		return persistence.ErrNotFound
	}
	if room.ActiveClaimToken != completion.Token {
		return persistence.ErrRevisionConflict
	}
	completedAt := completion.CompletedAt.UTC()
	room.ActiveClaimToken = ""
	room.ActiveClaimUntil = time.Time{}
	room.LastPulseCompletedAt = completedAt
	room.LastPulseSummary = completion.Summary
	room.LastPulseError = completion.Error
	room.UpdatedAt = completedAt
	room.Revision++
	s.rooms[scopeKey] = room
	if len(completion.DueTaskIDs) == 0 {
		return nil
	}
	for _, taskID := range completion.DueTaskIDs {
		task, ok := s.tasks[scopeKey][taskID]
		if !ok {
			continue
		}
		task.LastRunAt = completedAt
		task.LastResultSummary = completion.Summary
		task.UpdatedAt = completedAt
		s.tasks[scopeKey][taskID] = task
	}
	return nil
}
