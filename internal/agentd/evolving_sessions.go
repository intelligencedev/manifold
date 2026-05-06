package agentd

import (
	"context"
	"manifold/internal/agent"
	"manifold/internal/agent/memory"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

func (a *app) attachSessionEvolvingMemory(eng *agent.Engine, userID int64, sessionID string) *memory.EvolvingMemory {
	if eng == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		sessionID = "default"
	}
	eng.SessionID = sessionID
	if a.evolvingCfg.LLM == nil {
		eng.EvolvingMemory = nil
		eng.ReMemEnabled = false
		eng.ReMemController = nil
		return nil
	}
	em := a.getOrCreateEvolvingMemoryForSession(userID, sessionID)
	if em == nil {
		eng.EvolvingMemory = nil
		eng.ReMemEnabled = false
		eng.ReMemController = nil
		return nil
	}
	eng.EvolvingMemory = em
	if a.engine != nil && a.engine.ReMemEnabled {
		eng.ReMemEnabled = true
		eng.ReMemController = memory.NewReMemController(memory.ReMemConfig{
			LLM:           a.evolvingCfg.LLM,
			Model:         a.evolvingCfg.Model,
			Memory:        em,
			MaxInnerSteps: a.rememMaxInnerSteps,
		})
	} else {
		eng.ReMemEnabled = false
		eng.ReMemController = nil
	}
	return em
}

func (a *app) getOrCreateEvolvingMemoryForSession(userID int64, sessionID string) *memory.EvolvingMemory {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		sessionID = "default"
	}
	now := time.Now()

	a.evolvingMu.RLock()
	if a.userEvolving != nil {
		if sessions := a.userEvolving[userID]; sessions != nil {
			if em := sessions[sessionID]; em != nil {
				a.evolvingMu.RUnlock()
				a.markEvolvingSessionUsed(userID, sessionID, now)
				return em
			}
		}
	}
	a.evolvingMu.RUnlock()

	a.evolvingMu.Lock()
	defer a.evolvingMu.Unlock()
	if a.userEvolving == nil {
		a.userEvolving = make(map[int64]map[string]*memory.EvolvingMemory)
	}
	if a.evolvingLastUsed == nil {
		a.evolvingLastUsed = make(map[int64]map[string]time.Time)
	}
	if a.userEvolving[userID] == nil {
		a.userEvolving[userID] = make(map[string]*memory.EvolvingMemory)
	}
	if a.evolvingLastUsed[userID] == nil {
		a.evolvingLastUsed[userID] = make(map[string]time.Time)
	}
	if em := a.userEvolving[userID][sessionID]; em != nil {
		a.evolvingLastUsed[userID][sessionID] = now
		return em
	}
	if a.evolvingCfg.LLM == nil {
		return nil
	}
	cfg := a.evolvingCfg
	cfg.UserID = userID
	cfg.SessionID = sessionID
	em := memory.NewEvolvingMemory(cfg)
	a.userEvolving[userID][sessionID] = em
	a.evolvingLastUsed[userID][sessionID] = now
	return em
}

func (a *app) markEvolvingSessionUsed(userID int64, sessionID string, now time.Time) {
	a.evolvingMu.Lock()
	defer a.evolvingMu.Unlock()
	if a.evolvingLastUsed == nil {
		a.evolvingLastUsed = make(map[int64]map[string]time.Time)
	}
	if a.evolvingLastUsed[userID] == nil {
		a.evolvingLastUsed[userID] = make(map[string]time.Time)
	}
	a.evolvingLastUsed[userID][sessionID] = now
}

func (a *app) cleanupExpiredEvolvingSessions(now time.Time) int {
	if a.evolvingSessionTTL <= 0 {
		return 0
	}
	cutoff := now.Add(-a.evolvingSessionTTL)
	removed := 0

	a.evolvingMu.Lock()
	defer a.evolvingMu.Unlock()
	for userID, sessions := range a.evolvingLastUsed {
		for sessionID, lastUsed := range sessions {
			if lastUsed.After(cutoff) {
				continue
			}
			delete(sessions, sessionID)
			if userSessions := a.userEvolving[userID]; userSessions != nil {
				delete(userSessions, sessionID)
				if len(userSessions) == 0 {
					delete(a.userEvolving, userID)
				}
			}
			removed++
		}
		if len(sessions) == 0 {
			delete(a.evolvingLastUsed, userID)
		}
	}
	return removed
}

func (a *app) startEvolvingSessionJanitor(ctx context.Context, interval time.Duration) {
	if interval <= 0 || a.evolvingSessionTTL <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				removed := a.cleanupExpiredEvolvingSessions(time.Now())
				if removed > 0 {
					log.Debug().Int("removed", removed).Msg("evolving_memory_sessions_evicted")
				}
			}
		}
	}()
}
