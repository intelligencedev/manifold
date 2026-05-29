package agentd

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"manifold/internal/agent"
	persist "manifold/internal/persistence"
)

type chatActivityCollector struct {
	mu                 sync.Mutex
	sessionID          string
	runID              string
	userID             *int64
	assistantMessageID string
	records            map[string]persist.SpecialistActivityRecord
}

func newChatActivityCollector(sessionID, runID string, userID *int64, assistantMessageID string) *chatActivityCollector {
	return &chatActivityCollector{
		sessionID:          sessionID,
		runID:              runID,
		userID:             cloneCollectorUserID(userID),
		assistantMessageID: strings.TrimSpace(assistantMessageID),
		records:            map[string]persist.SpecialistActivityRecord{},
	}
}

func (c *chatActivityCollector) Handle(ev agent.AgentTrace) {
	callID := strings.TrimSpace(ev.CallID)
	agentName := strings.TrimSpace(ev.Agent)
	if callID == "" || agentName == "" {
		return
	}
	now := time.Now().UTC()
	id := fmt.Sprintf("%s:%s", c.runID, callID)

	c.mu.Lock()
	defer c.mu.Unlock()

	record := c.activityRecord(id, callID, agentName, ev, now)
	updateActivityRecordIdentity(&record, ev, agentName)
	record.UpdatedAt = now
	applyActivityEvent(&record, ev, now)
	c.records[id] = record
}

func (c *chatActivityCollector) activityRecord(id, callID, agentName string, ev agent.AgentTrace, now time.Time) persist.SpecialistActivityRecord {
	if record, ok := c.records[id]; ok {
		return record
	}
	return persist.SpecialistActivityRecord{
		ID:                 id,
		SessionID:          c.sessionID,
		UserID:             cloneCollectorUserID(c.userID),
		RunID:              c.runID,
		AssistantMessageID: c.assistantMessageID,
		CallID:             callID,
		ParentCallID:       strings.TrimSpace(ev.ParentCallID),
		Agent:              agentName,
		Team:               strings.TrimSpace(ev.Team),
		Model:              strings.TrimSpace(ev.Model),
		Prompt:             ev.Content,
		Depth:              ev.Depth,
		Status:             "running",
		Entries:            []persist.SpecialistActivityEntry{},
		ThoughtSummaries:   []string{},
		StartedAt:          now,
		UpdatedAt:          now,
	}
}

func updateActivityRecordIdentity(record *persist.SpecialistActivityRecord, ev agent.AgentTrace, agentName string) {
	if parent := strings.TrimSpace(ev.ParentCallID); parent != "" {
		record.ParentCallID = parent
	}
	record.Agent = agentName
	if team := strings.TrimSpace(ev.Team); team != "" {
		record.Team = team
	}
	if model := strings.TrimSpace(ev.Model); model != "" {
		record.Model = model
	}
	if ev.Depth > 0 {
		record.Depth = ev.Depth
	}
}

func applyActivityEvent(record *persist.SpecialistActivityRecord, ev agent.AgentTrace, now time.Time) {
	switch ev.Type {
	case "agent_start":
		record.Status = "running"
		if ev.Content != "" {
			record.Prompt = ev.Content
		}
	case "agent_delta":
		record.Status = "running"
		record.Content += ev.Content
	case "agent_final":
		record.Status = "done"
		if ev.Content != "" {
			record.Content = ev.Content
		}
		finished := now
		record.FinishedAt = &finished
	case "agent_tool_start":
		record.Status = "running"
		record.Entries = append(record.Entries, persist.SpecialistActivityEntry{
			ID:        entryID(ev.ToolID, now),
			Type:      "tool",
			Title:     ev.Title,
			Args:      ev.Args,
			CreatedAt: now,
		})
	case "agent_tool_result":
		record.Status = "running"
		record.Entries = append(record.Entries, persist.SpecialistActivityEntry{
			ID:        entryID(ev.ToolID, now),
			Type:      "tool",
			Title:     ev.Title,
			Args:      ev.Args,
			Data:      ev.Data,
			CreatedAt: now,
		})
	case "agent_error":
		record.Status = "error"
		record.Error = ev.Error
		record.Entries = append(record.Entries, persist.SpecialistActivityEntry{
			ID:        entryID("error", now),
			Type:      "error",
			Content:   ev.Error,
			CreatedAt: now,
		})
		finished := now
		record.FinishedAt = &finished
	case "agent_thought_summary":
		record.Status = "running"
		appendThoughtSummary(record, ev.ThoughtSummary)
	}
}

func (c *chatActivityCollector) Snapshot() []persist.SpecialistActivityRecord {
	c.mu.Lock()
	defer c.mu.Unlock()

	out := make([]persist.SpecialistActivityRecord, 0, len(c.records))
	for _, record := range c.records {
		out = append(out, cloneCollectorRecord(record))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].StartedAt.Before(out[j].StartedAt)
		}
		return out[i].UpdatedAt.Before(out[j].UpdatedAt)
	})
	return out
}

func appendThoughtSummary(record *persist.SpecialistActivityRecord, summary string) {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return
	}
	existing := record.ThoughtSummaries
	if len(existing) == 0 {
		record.ThoughtSummaries = []string{summary}
		return
	}
	last := existing[len(existing)-1]
	if summary == last {
		return
	}
	if len(summary) > len(last) && strings.HasPrefix(summary, last) {
		next := append([]string(nil), existing...)
		next[len(next)-1] = summary
		record.ThoughtSummaries = next
		return
	}
	record.ThoughtSummaries = append(append([]string(nil), existing...), summary)
}

func entryID(seed string, now time.Time) string {
	if seed != "" {
		return fmt.Sprintf("%s:%d", seed, now.UnixNano())
	}
	return fmt.Sprintf("entry:%d", now.UnixNano())
}

func cloneCollectorUserID(userID *int64) *int64 {
	if userID == nil {
		return nil
	}
	v := *userID
	return &v
}

func cloneCollectorRecord(record persist.SpecialistActivityRecord) persist.SpecialistActivityRecord {
	clone := record
	clone.UserID = cloneCollectorUserID(record.UserID)
	if len(record.Entries) > 0 {
		clone.Entries = append([]persist.SpecialistActivityEntry(nil), record.Entries...)
	}
	if len(record.ThoughtSummaries) > 0 {
		clone.ThoughtSummaries = append([]string(nil), record.ThoughtSummaries...)
	}
	if record.FinishedAt != nil {
		finished := *record.FinishedAt
		clone.FinishedAt = &finished
	}
	return clone
}
