package memory

import (
	"context"
	"manifold/internal/observability"
	"sort"
	"strings"
	"time"
)

func (em *EvolvingMemory) ExportMemories() []*MemoryEntry {
	em.mu.RLock()
	defer em.mu.RUnlock()
	return em.snapshotEntriesLocked()
}

func (em *EvolvingMemory) shouldPromoteToUserScopeLocked(entry *MemoryEntry) bool {
	if entry == nil || entry.Scope == MemoryScopeUser || em.promotionAccessThreshold <= 0 {
		return false
	}
	if entry.AccessCount < em.promotionAccessThreshold || entry.MemoryType != MemoryProcedural {
		return false
	}
	return entry.StructuredFeedback != nil && entry.StructuredFeedback.Correct && entry.StructuredFeedback.Type == FeedbackSuccess
}

func (em *EvolvingMemory) promoteToUserScopeAsync(store EvolvingMemoryPromotionStore, entryID string) {
	if store == nil || strings.TrimSpace(entryID) == "" {
		return
	}
	go func() {
		bgctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := store.PromoteToUserScope(bgctx, em.userID, entryID); err != nil {
			observability.LoggerWithTrace(bgctx).Error().Err(err).Msg("evolving_memory_promote_failed")
		}
	}()
}

func (em *EvolvingMemory) startStoreJanitor(interval time.Duration) {
	store, ok := em.store.(EvolvingMemoryJanitorStore)
	if !ok || interval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			bgctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			removed, err := store.DeleteExpired(bgctx, time.Now().UTC())
			cancel()
			if err != nil {
				observability.LoggerWithTrace(context.Background()).Error().Err(err).Msg("evolving_memory_expired_delete_failed")
				continue
			}
			if removed > 0 && em.metrics != nil {
				em.metrics.RecordPruned(context.Background(), "expired", int(removed))
			}
		}
	}()
}

func filterExpiredEntries(entries []*MemoryEntry, now time.Time) []*MemoryEntry {
	if len(entries) == 0 {
		return entries
	}
	out := entries[:0]
	for _, entry := range entries {
		if entry == nil || (entry.ExpiresAt != nil && !entry.ExpiresAt.After(now)) {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func (em *EvolvingMemory) snapshotEntriesLocked() []*MemoryEntry {
	return cloneEntrySlice(em.entries)
}

func (em *EvolvingMemory) persistEntriesAsync(entries []*MemoryEntry) {
	if em.store == nil {
		return
	}
	if _, ok := em.store.(EvolvingMemoryDeltaStore); ok {
		em.persistDirtyEntriesAsync()
		return
	}

	em.mu.Lock()
	em.pendingPersist = entries
	em.persistVersion++
	version := em.persistVersion
	delay := em.persistDelay
	uid := em.userID
	sid := em.sessionID
	em.mu.Unlock()

	go func(targetVersion uint64) {
		if delay > 0 {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			<-timer.C
		}

		em.mu.Lock()
		if em.persistVersion != targetVersion {
			em.mu.Unlock()
			return
		}
		entries := em.pendingPersist
		em.pendingPersist = nil
		em.mu.Unlock()

		bgctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := em.store.Save(bgctx, uid, sid, entries); err != nil {
			observability.LoggerWithTrace(bgctx).Error().Err(err).Msg("evolving_memory_persist_failed")
		}
	}(version)
}

func (em *EvolvingMemory) persistDirtyEntriesAsync() {
	em.mu.Lock()
	if len(em.dirtyIDs) == 0 && len(em.deletedIDs) == 0 {
		em.mu.Unlock()
		return
	}
	em.persistVersion++
	version := em.persistVersion
	delay := em.persistDelay
	uid := em.userID
	sid := em.sessionID
	em.mu.Unlock()

	go func(targetVersion uint64) {
		if delay > 0 {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			<-timer.C
		}

		em.mu.Lock()
		if em.persistVersion != targetVersion {
			em.mu.Unlock()
			return
		}
		dirtyIDs := em.dirtyIDs
		deletedIDs := em.deletedIDs
		em.dirtyIDs = make(map[string]struct{})
		em.deletedIDs = make(map[string]struct{})
		entries := em.entriesForIDsLocked(dirtyIDs)
		idsToDelete := stringSetKeys(deletedIDs)
		em.mu.Unlock()

		deltaStore, ok := em.store.(EvolvingMemoryDeltaStore)
		if !ok {
			return
		}

		bgctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if len(entries) > 0 {
			if err := deltaStore.Upsert(bgctx, uid, sid, entries); err != nil {
				observability.LoggerWithTrace(bgctx).Error().Err(err).Msg("evolving_memory_delta_upsert_failed")
				return
			}
		}
		if len(idsToDelete) > 0 {
			if err := deltaStore.Delete(bgctx, uid, sid, idsToDelete); err != nil {
				observability.LoggerWithTrace(bgctx).Error().Err(err).Msg("evolving_memory_delta_delete_failed")
			}
		}
	}(version)
}

func (em *EvolvingMemory) touchAccessAsync(deltaStore EvolvingMemoryDeltaStore, ids []string, at time.Time) {
	if len(ids) == 0 {
		return
	}
	idsCopy := append([]string(nil), ids...)
	go func() {
		bgctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := deltaStore.TouchAccess(bgctx, idsCopy, at); err != nil {
			observability.LoggerWithTrace(bgctx).Error().Err(err).Msg("evolving_memory_touch_access_failed")
		}
	}()
}

func (em *EvolvingMemory) markDirtyLocked(id string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	if em.dirtyIDs == nil {
		em.dirtyIDs = make(map[string]struct{})
	}
	em.dirtyIDs[id] = struct{}{}
	delete(em.deletedIDs, id)
}

func (em *EvolvingMemory) markDeletedLocked(ids []string) {
	if len(ids) == 0 {
		return
	}
	if em.deletedIDs == nil {
		em.deletedIDs = make(map[string]struct{})
	}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		delete(em.dirtyIDs, id)
		em.deletedIDs[id] = struct{}{}
	}
}

func (em *EvolvingMemory) entriesForIDsLocked(ids map[string]struct{}) []*MemoryEntry {
	if len(ids) == 0 {
		return nil
	}
	entries := make([]*MemoryEntry, 0, len(ids))
	for _, entry := range em.entries {
		if entry == nil {
			continue
		}
		if _, ok := ids[entry.ID]; ok {
			entries = append(entries, cloneEntry(entry))
		}
	}
	return entries
}

func stringSetKeys(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
