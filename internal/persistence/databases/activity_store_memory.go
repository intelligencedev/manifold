package databases

import (
	"context"
	"sort"
	"strings"
	"sync"

	"manifold/internal/persistence"
)

func NewMemorySpecialistActivityStore() persistence.SpecialistActivityStore {
	return &memSpecialistActivityStore{
		activities: map[string]map[string]persistence.SpecialistActivityRecord{},
	}
}

type memSpecialistActivityStore struct {
	mu         sync.RWMutex
	activities map[string]map[string]persistence.SpecialistActivityRecord
}

func (s *memSpecialistActivityStore) Init(context.Context) error { return nil }

func (s *memSpecialistActivityStore) ListSessionActivities(_ context.Context, userID *int64, sessionID string) ([]persistence.SpecialistActivityRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return []persistence.SpecialistActivityRecord{}, nil
	}
	byID := s.activities[sessionID]
	if len(byID) == 0 {
		return []persistence.SpecialistActivityRecord{}, nil
	}
	out := make([]persistence.SpecialistActivityRecord, 0, len(byID))
	for _, activity := range byID {
		if !hasAccess(userID, activity.UserID) {
			continue
		}
		out = append(out, copySpecialistActivityRecord(activity))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].StartedAt.Before(out[j].StartedAt)
		}
		return out[i].UpdatedAt.Before(out[j].UpdatedAt)
	})
	return out, nil
}

func (s *memSpecialistActivityStore) UpsertSessionActivities(_ context.Context, userID *int64, sessionID string, activities []persistence.SpecialistActivityRecord) error {
	if len(activities) == 0 {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	byID := s.activities[sessionID]
	if byID == nil {
		byID = map[string]persistence.SpecialistActivityRecord{}
		s.activities[sessionID] = byID
	}
	for _, activity := range activities {
		id := strings.TrimSpace(activity.ID)
		if id == "" {
			continue
		}
		clone := copySpecialistActivityRecord(activity)
		clone.SessionID = sessionID
		if clone.UserID == nil {
			clone.UserID = copyUserID(userID)
		}
		byID[id] = clone
	}
	return nil
}

func (s *memSpecialistActivityStore) DeleteSessionActivities(_ context.Context, userID *int64, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sessionID = strings.TrimSpace(sessionID)
	byID := s.activities[sessionID]
	if len(byID) == 0 {
		return nil
	}
	for _, activity := range byID {
		if !hasAccess(userID, activity.UserID) {
			return persistence.ErrForbidden
		}
		break
	}
	delete(s.activities, sessionID)
	return nil
}

func (s *memSpecialistActivityStore) DeleteRunActivities(_ context.Context, userID *int64, sessionID string, runID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sessionID = strings.TrimSpace(sessionID)
	runID = strings.TrimSpace(runID)
	if sessionID == "" || runID == "" {
		return nil
	}
	byID := s.activities[sessionID]
	if len(byID) == 0 {
		return nil
	}
	for id, activity := range byID {
		if !hasAccess(userID, activity.UserID) {
			continue
		}
		if activity.RunID == runID {
			delete(byID, id)
		}
	}
	if len(byID) == 0 {
		delete(s.activities, sessionID)
	}
	return nil
}

func copySpecialistActivityRecord(activity persistence.SpecialistActivityRecord) persistence.SpecialistActivityRecord {
	clone := activity
	clone.UserID = copyUserID(activity.UserID)
	if len(activity.Entries) > 0 {
		clone.Entries = append([]persistence.SpecialistActivityEntry(nil), activity.Entries...)
	}
	if len(activity.ThoughtSummaries) > 0 {
		clone.ThoughtSummaries = append([]string(nil), activity.ThoughtSummaries...)
	}
	if activity.FinishedAt != nil {
		finished := *activity.FinishedAt
		clone.FinishedAt = &finished
	}
	return clone
}
