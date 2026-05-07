package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"manifold/internal/llm"
	"manifold/internal/observability"
	"manifold/internal/persistence"
	"strings"
	"sync"
)

func (m *Manager) ensureSummary(ctx context.Context, userID *int64, session persistence.ChatSession, messages []persistence.ChatMessage, targetCompactor llm.CompactionProvider, targetModel string, policy SummaryPolicy) (string, int, *SummaryResult) {
	if !m.enabled || m.summary == nil {
		return session.Summary, session.SummarizedCount, nil
	}

	total := len(messages)
	if total == 0 {
		return session.Summary, session.SummarizedCount, nil
	}

	// Token-based: estimate token usage and roll summary incrementally based on
	// token budget (context window minus reserve buffer).
	targetCtxSize := m.resolveTargetContextWindowTokens(policy, targetModel)
	plainCtxSize := m.resolvePlainTextContextWindowTokens(policy)
	ctxSize := targetCtxSize

	reserveBuffer := m.reserveBufferTokens
	if reserveBuffer <= 0 {
		reserveBuffer = defaultReserveBuffer
	}

	// Force summarization once the chat exceeds the configured max tail size.
	// This keeps the raw transcript short even for very large-context models.
	maxTail := m.maxKeepLastMessages
	if maxTail < m.minKeepLastMessages {
		maxTail = m.minKeepLastMessages
	}
	compactionEnabled := m.responsesCompactionEnabled(targetCompactor)
	if !compactionEnabled {
		ctxSize = minPositiveInt(targetCtxSize, plainCtxSize)
		if ctxSize <= 0 {
			ctxSize = targetCtxSize
		}
	}
	if ctxSize <= 0 {
		ctxSize = 32_000 // Conservative default for memory budgeting
	}

	budget := ctxSize - reserveBuffer
	if budget <= 0 {
		budget = ctxSize / 2
	}
	forceByCount := !compactionEnabled && maxTail > 0 && total > maxTail

	estimated := 0
	for _, msg := range messages {
		estimated += len([]rune(strings.TrimSpace(msg.Content)))/4 + 1
	}
	if !forceByCount && estimated <= budget {
		// Compaction mode: compact after milestones, not every turn.
		if compactionEnabled {
			delta := total - session.SummarizedCount
			if delta <= 0 {
				return session.Summary, session.SummarizedCount, nil
			}
			toolOutputs := 0
			if session.SummarizedCount >= 0 && session.SummarizedCount < total {
				for _, msg := range messages[session.SummarizedCount:] {
					if msg.Role == "tool" {
						toolOutputs++
					}
				}
			}
			if delta < compactionMinDeltaMessages && toolOutputs < compactionToolMilestoneOutputs {
				return session.Summary, session.SummarizedCount, nil
			}
		}
		return session.Summary, session.SummarizedCount, nil
	}

	// When force-by-count is the only trigger (token budget is still within
	// limits), defer summarization until a minimum batch of new messages has
	// accumulated.  Calling the summary model on every single new message is
	// expensive — for slow local/thinking models each call can block for 60-120 s.
	if forceByCount && estimated <= budget {
		potentialDelta := (total - maxTail) - session.SummarizedCount
		if potentialDelta < minForceCountBatch {
			return session.Summary, session.SummarizedCount, nil
		}
	}

	// Decide how many early messages to include in the next summary chunk.
	// For classic summarization we keep a small raw tail; for Responses compaction
	// we prefer to compact the full eligible delta so the compaction blob fully
	// represents prior state.
	minTail := m.minKeepLastMessages
	if compactionEnabled {
		minTail = 0
	} else {
		if minTail <= 0 {
			minTail = 4
		}
		if forceByCount {
			minTail = maxTail
		}
	}
	if total <= minTail {
		return session.Summary, session.SummarizedCount, nil
	}

	target := total - minTail
	if target <= 0 {
		return session.Summary, session.SummarizedCount, nil
	}

	summarizedCount := session.SummarizedCount
	if summarizedCount > target {
		summarizedCount = target
	}

	if summarizedCount == target {
		return session.Summary, summarizedCount, nil
	}

	start := summarizedCount
	if start < 0 || start > target {
		start = 0
	}

	// Never cut between an assistant tool call and its tool response.
	// If the tail includes tool responses, ensure the boundary keeps the
	// corresponding assistant ToolCalls in the raw tail too.
	if target > start {
		adjustedTarget := adjustIndexForToolDeps(messages, start, target)
		if adjustedTarget < target {
			target = adjustedTarget
			if target <= start {
				return session.Summary, summarizedCount, nil
			}
		}
	}

	chunk := messages[start:target]
	if len(chunk) == 0 {
		return session.Summary, summarizedCount, nil
	}

	// Log summarization trigger with consistent format as Engine.maybeSummarize
	log := observability.LoggerWithTrace(ctx)
	log.Info().
		Str("session", session.ID).
		Int("messages", total).
		Int("estimated_tokens", estimated).
		Int("token_budget", budget).
		Int("context_window", ctxSize).
		Int("target_context_window", targetCtxSize).
		Int("plain_text_context_window", plainCtxSize).
		Int("reserve_buffer", reserveBuffer).
		Int("summarizing_count", len(chunk)).
		Msg("summarization_triggered")

	// Build result metadata for the caller to notify users
	result := &SummaryResult{
		Triggered:       true,
		EstimatedTokens: estimated,
		TokenBudget:     budget,
		MessageCount:    total,
		SummarizedCount: len(chunk),
	}

	summary, err := m.summarizeChunk(ctx, session.Summary, chunk, targetCompactor, targetModel)
	if err != nil {
		log.Error().Err(err).Str("session", session.ID).Msg("chat_summary_failed")
		return session.Summary, summarizedCount, result
	}

	if err := m.store.UpdateSummary(ctx, userID, session.ID, summary, target); err != nil {
		log.Error().Err(err).Str("session", session.ID).Msg("chat_summary_persist_failed")
		return session.Summary, summarizedCount, result
	}

	log.Info().Str("session", session.ID).Int("messages", target).Msg("chat_summary_updated")
	return summary, target, result
}

func (m *Manager) resolveTargetContextWindowTokens(policy SummaryPolicy, targetModel string) int {
	ctxSize := policy.TargetContextWindowTokens
	if ctxSize <= 0 {
		ctxSize = m.contextWindowTokens
	}
	if ctxSize <= 0 && targetModel != "" {
		if size, _ := llm.ContextSize(targetModel); size > 0 {
			ctxSize = size
		}
	}
	if ctxSize <= 0 {
		ctxSize = 32_000
	}
	return ctxSize
}

func (m *Manager) resolvePlainTextContextWindowTokens(policy SummaryPolicy) int {
	ctxSize := policy.PlainTextContextWindowTokens
	if ctxSize <= 0 {
		ctxSize = m.plainTextContextWindowTokens
	}
	if ctxSize <= 0 && m.summaryModel != "" {
		if size, _ := llm.ContextSize(m.summaryModel); size > 0 {
			ctxSize = size
		}
	}
	if ctxSize <= 0 {
		ctxSize = 32_000
	}
	return ctxSize
}

func minPositiveInt(vals ...int) int {
	best := 0
	for _, value := range vals {
		if value <= 0 {
			continue
		}
		if best == 0 || value < best {
			best = value
		}
	}
	return best
}

func (m *Manager) summarizeChunk(ctx context.Context, existingSummary string, chunk []persistence.ChatMessage, targetCompactor llm.CompactionProvider, targetModel string) (string, error) {
	if m.summary == nil {
		return existingSummary, fmt.Errorf("llm provider unavailable")
	}

	// Decode existing summary to get both compaction and plain components
	existing := decodeDualSummary(existingSummary)
	log := observability.LoggerWithTrace(ctx)

	if m.responsesCompactionEnabled(targetCompactor) {
		// Run both compaction and plain text summarization in parallel.
		// This ensures we have a plain text fallback if the user switches to a non-OpenAI model.
		var (
			wg            sync.WaitGroup
			compactionErr error
			plainErr      error
			compactionRes string
			plainRes      string
		)

		wg.Add(2)

		// Goroutine 1: Compaction summarization (OpenAI Responses API)
		go func() {
			defer wg.Done()
			res, err := m.compactChunk(ctx, targetCompactor, targetModel, existing.Compaction, chunk)
			if err != nil {
				compactionErr = err
				log.Warn().Err(err).Msg("compaction_summarization_failed")
				return
			}
			compactionRes = res
		}()

		// Goroutine 2: Plain text summarization (for non-OpenAI model fallback)
		go func() {
			defer wg.Done()
			res, err := m.plainSummarize(ctx, existing.Plain, chunk)
			if err != nil {
				plainErr = err
				log.Warn().Err(err).Msg("plain_summarization_failed")
				return
			}
			plainRes = res
		}()

		wg.Wait()

		// Build dual summary with whatever succeeded
		ds := dualSummary{
			Compaction: compactionRes,
			Plain:      plainRes,
		}

		// If compaction failed but plain succeeded, use plain only
		if compactionErr != nil && plainErr == nil {
			return ds.Plain, nil
		}
		// If both failed, return error
		if compactionErr != nil && plainErr != nil {
			return existingSummary, fmt.Errorf("both summarization methods failed: compaction: %v, plain: %v", compactionErr, plainErr)
		}
		// If plain failed but compaction succeeded, still return dual (plain will be empty)
		// Future requests to non-OpenAI models will get no summary, which is safer than garbage

		return encodeDualSummary(ds), nil
	}

	// Non-compaction mode: just do plain text summarization
	plainRes, err := m.plainSummarize(ctx, existing.Plain, chunk)
	if err != nil {
		return existingSummary, err
	}
	return plainRes, nil
}

func (m *Manager) responsesCompactionEnabled(targetCompactor llm.CompactionProvider) bool {
	return targetCompactor != nil && !m.compactionUnavailable.Load()
}

// plainSummarize generates a plain text summary of the conversation chunk.
// This is used for non-OpenAI models and as a fallback when compaction is enabled.
func (m *Manager) plainSummarize(ctx context.Context, existingPlainSummary string, chunk []persistence.ChatMessage) (string, error) {
	sections := m.buildPlainSummarySections(existingPlainSummary, chunk)
	summary, err := m.reducePlainSummarySections(ctx, sections)
	if err != nil {
		return existingPlainSummary, err
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return existingPlainSummary, fmt.Errorf("empty summary returned")
	}
	return summary, nil
}

func (m *Manager) buildPlainSummarySections(existingPlainSummary string, chunk []persistence.ChatMessage) []string {
	sections := make([]string, 0, len(chunk)+1)
	if existing := strings.TrimSpace(existingPlainSummary); existing != "" {
		sections = append(sections, "Existing summary:\n"+existing)
	}

	limit := maxSummarizeChunkSize
	if m.maxSummaryChunkTokens > 0 {
		limit = m.maxSummaryChunkTokens
	}

	for _, msg := range chunk {
		summaryMsg := buildSummaryPromptMessage(msg)
		var section strings.Builder
		section.WriteString("Role: ")
		section.WriteString(summaryMsg.Role)
		section.WriteString("\n")
		if len(summaryMsg.ToolCalls) > 0 {
			section.WriteString("Tool calls: ")
			section.WriteString(strings.Join(summaryMsg.ToolCalls, ", "))
			section.WriteString("\n")
		}
		if strings.TrimSpace(summaryMsg.ToolID) != "" {
			section.WriteString("Tool ID: ")
			section.WriteString(summaryMsg.ToolID)
			section.WriteString("\n")
		}
		content := truncateForSummary(strings.TrimSpace(summaryMsg.Content), limit)
		if content == "" {
			content = "(no content)"
		}
		section.WriteString(content)
		sections = append(sections, section.String())
	}

	return sections
}

func (m *Manager) reducePlainSummarySections(ctx context.Context, sections []string) (string, error) {
	current := make([]string, 0, len(sections))
	chunkBudget := m.summaryReductionChunkBudget()
	for _, section := range sections {
		current = append(current, splitSummarySection(section, chunkBudget)...)
	}
	if len(current) == 0 {
		return "", fmt.Errorf("empty summary input")
	}

	if len(current) == 1 {
		summary, err := m.runPlainSummaryPass(ctx, current)
		if err != nil {
			return "", err
		}
		summary = strings.TrimSpace(summary)
		if summary == "" {
			return "", fmt.Errorf("empty summary returned")
		}
		return summary, nil
	}

	for pass := 0; pass < 8; pass++ {
		chunks := packSummarySections(current, chunkBudget)
		if len(chunks) == 1 {
			summary, err := m.runPlainSummaryPass(ctx, chunks[0])
			if err != nil {
				return "", err
			}
			summary = strings.TrimSpace(summary)
			if summary == "" {
				return "", fmt.Errorf("empty summary returned")
			}
			return summary, nil
		}
		next := make([]string, 0, len(chunks))
		for _, chunk := range chunks {
			summary, err := m.runPlainSummaryPass(ctx, chunk)
			if err != nil {
				return "", err
			}
			summary = strings.TrimSpace(summary)
			if summary == "" {
				return "", fmt.Errorf("empty summary returned")
			}
			next = append(next, summary)
		}

		if len(next) == 1 && estimateSummaryTextTokens(next[0]) <= chunkBudget {
			return next[0], nil
		}

		if len(next) == 1 && len(current) == 1 && strings.TrimSpace(next[0]) == strings.TrimSpace(current[0]) {
			current = splitSummarySection(next[0], maxInt(1, chunkBudget/2))
			continue
		}

		current = current[:0]
		for _, summary := range next {
			current = append(current, splitSummarySection(summary, chunkBudget)...)
		}
	}

	return truncateForSummary(strings.Join(current, "\n\n"), chunkBudget*4), nil
}

func (m *Manager) runPlainSummaryPass(ctx context.Context, sections []string) (string, error) {
	msgs := m.buildPlainSummaryPassMessages(sections)
	if limit := m.summaryPromptTokenLimit(); limit > 0 && estimateMessagesTokens(msgs) > limit {
		if len(sections) > 1 {
			mid := len(sections) / 2
			left, err := m.runPlainSummaryPass(ctx, sections[:mid])
			if err != nil {
				return "", err
			}
			right, err := m.runPlainSummaryPass(ctx, sections[mid:])
			if err != nil {
				return "", err
			}

			reduced := []string{left, right}
			if estimateMessagesTokens(m.buildPlainSummaryPassMessages(reduced)) > limit {
				merged := m.truncateSectionForPromptLimit(strings.Join(reduced, "\n\n"), limit)
				return m.runPlainSummaryPass(ctx, []string{merged})
			}
			return m.runPlainSummaryPass(ctx, reduced)
		}

		split := splitSummarySection(sections[0], maxInt(1, m.summaryReductionChunkBudget()/2))
		if len(split) > 1 {
			compressed := make([]string, 0, len(split))
			for _, part := range split {
				summary, err := m.runPlainSummaryPass(ctx, []string{part})
				if err != nil {
					return "", err
				}
				compressed = append(compressed, summary)
			}
			return m.runPlainSummaryPass(ctx, compressed)
		}

		truncated := m.truncateSectionForPromptLimit(sections[0], limit)
		if strings.TrimSpace(truncated) != strings.TrimSpace(sections[0]) {
			return m.runPlainSummaryPass(ctx, []string{truncated})
		}

		// If the prompt budget cannot even fit the summarizer instructions plus a
		// minimally truncated section, fall back to a deterministic truncation
		// instead of failing or recursing forever.
		return truncateForSummary(strings.TrimSpace(sections[0]), maxInt(32, limit*4)), nil
	}

	var (
		resp llm.Message
		err  error
	)
	if m.summaryCallTimeout > 0 {
		callCtx, cancel := context.WithTimeout(ctx, m.summaryCallTimeout)
		defer cancel()
		resp, err = m.summary.Chat(callCtx, msgs, nil, m.summaryModel)
	} else {
		resp, err = m.summary.Chat(ctx, msgs, nil, m.summaryModel)
	}
	if err != nil {
		return "", fmt.Errorf("summarize chat: %w", err)
	}
	return strings.TrimSpace(resp.Content), nil
}

func (m *Manager) buildPlainSummaryPassMessages(sections []string) []llm.Message {
	var userPrompt strings.Builder
	userPrompt.WriteString("Update the running summary of this chat. Keep it concise but information-dense.\n")
	userPrompt.WriteString("Preserve user goals, preferences, decisions, key facts, identifiers (files, URLs, IDs), tool results/errors, and open questions.\n")
	userPrompt.WriteString("If content includes [TRUNCATED], assume important details may be missing.\n")
	userPrompt.WriteString("\nConversation material:\n")
	for i, section := range sections {
		if i > 0 {
			userPrompt.WriteString("\n\n---\n\n")
		}
		userPrompt.WriteString(strings.TrimSpace(section))
	}
	userPrompt.WriteString("\n\nReturn only the updated summary. Aim for <= 1200 characters; use short bullets if helpful.")

	return []llm.Message{
		{Role: "system", Content: "You are a concise summarizer. Maintain an accurate running summary of a conversation."},
		{Role: "user", Content: userPrompt.String()},
	}
}

func (m *Manager) truncateSectionForPromptLimit(section string, limit int) string {
	if limit <= 0 {
		return strings.TrimSpace(section)
	}
	overhead := estimateMessagesTokens(m.buildPlainSummaryPassMessages([]string{""}))
	available := limit - overhead
	if available <= 0 {
		available = maxInt(1, limit/4)
	}
	return truncateForSummary(strings.TrimSpace(section), available*4)
}

func (m *Manager) summaryReductionChunkBudget() int {
	budget := m.summaryPromptTokenLimit()
	if budget <= 0 {
		budget = maxSummarizeChunkSize
	}
	budget -= 256
	if budget < 64 {
		budget = 64
	}
	return budget
}

func (m *Manager) summaryPromptTokenLimit() int {
	ctxSize := m.resolvePlainTextContextWindowTokens(SummaryPolicy{})

	reserveBuffer := m.reserveBufferTokens
	if reserveBuffer <= 0 {
		reserveBuffer = defaultReserveBuffer
	}

	budget := ctxSize - reserveBuffer
	if budget <= 0 {
		budget = ctxSize / 2
	}
	if budget <= 0 {
		budget = maxSummarizeChunkSize
	}
	return budget
}

func packSummarySections(sections []string, maxTokens int) [][]string {
	if maxTokens <= 0 {
		maxTokens = 1
	}
	chunks := make([][]string, 0, len(sections))
	current := make([]string, 0, len(sections))
	currentTokens := 0
	for _, section := range sections {
		section = strings.TrimSpace(section)
		if section == "" {
			continue
		}
		sectionTokens := estimateSummaryTextTokens(section)
		if len(current) > 0 && currentTokens+sectionTokens > maxTokens {
			chunks = append(chunks, current)
			current = make([]string, 0, len(sections))
			currentTokens = 0
		}
		current = append(current, section)
		currentTokens += sectionTokens
	}
	if len(current) > 0 {
		chunks = append(chunks, current)
	}
	return chunks
}

func splitSummarySection(section string, maxTokens int) []string {
	section = strings.TrimSpace(section)
	if section == "" {
		return nil
	}
	if maxTokens <= 0 || estimateSummaryTextTokens(section) <= maxTokens {
		return []string{section}
	}

	parts := strings.Split(section, "\n\n")
	if len(parts) > 1 {
		result := make([]string, 0, len(parts))
		for _, part := range parts {
			result = append(result, splitSummarySection(part, maxTokens)...)
		}
		if len(result) > 1 {
			return result
		}
	}

	maxRunes := maxTokens * 4
	if maxRunes <= 0 {
		maxRunes = len([]rune(section))
	}
	runes := []rune(section)
	chunks := make([]string, 0, (len(runes)/maxRunes)+1)
	for start := 0; start < len(runes); start += maxRunes {
		end := start + maxRunes
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[start:end]))
	}
	return chunks
}

func estimateSummaryTextTokens(content string) int {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return 1
	}
	return len([]rune(trimmed))/4 + 1
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

type summaryPromptMessage struct {
	Role      string
	Content   string
	ToolCalls []string
	ToolID    string
}

func buildSummaryPromptMessage(msg persistence.ChatMessage) summaryPromptMessage {
	out := summaryPromptMessage{
		Role:    msg.Role,
		Content: strings.TrimSpace(msg.Content),
	}
	raw := strings.TrimSpace(msg.Content)
	if raw == "" {
		return out
	}
	if msg.Role == "assistant" && strings.HasPrefix(raw, "{") {
		var data struct {
			Content   string         `json:"content"`
			ToolCalls []llm.ToolCall `json:"tool_calls"`
		}
		if err := json.Unmarshal([]byte(raw), &data); err == nil && len(data.ToolCalls) > 0 {
			out.Content = strings.TrimSpace(data.Content)
			out.ToolCalls = summarizeToolCalls(data.ToolCalls)
			return out
		}
	}
	if msg.Role == "tool" && strings.HasPrefix(raw, "{") {
		var data struct {
			Content string `json:"content"`
			ToolID  string `json:"tool_id"`
		}
		if err := json.Unmarshal([]byte(raw), &data); err == nil {
			if strings.TrimSpace(data.Content) != "" {
				out.Content = strings.TrimSpace(data.Content)
			} else {
				out.Content = ""
			}
			out.ToolID = strings.TrimSpace(data.ToolID)
			return out
		}
	}
	return out
}

func summarizeToolCalls(calls []llm.ToolCall) []string {
	out := make([]string, 0, len(calls))
	for _, call := range calls {
		name := strings.TrimSpace(call.Name)
		if name == "" {
			continue
		}
		args := strings.TrimSpace(string(call.Args))
		if isEmptySummaryArgs(args) {
			out = append(out, name)
			continue
		}
		args = strings.Join(strings.Fields(args), " ")
		args = truncateInline(args, 160)
		out = append(out, fmt.Sprintf("%s args=%s", name, args))
	}
	return out
}

func isEmptySummaryArgs(raw string) bool {
	switch strings.TrimSpace(raw) {
	case "", "null", "{}", "[]":
		return true
	default:
		return false
	}
}

func truncateInline(content string, limit int) string {
	trimmed := strings.TrimSpace(content)
	if limit <= 0 {
		return trimmed
	}
	runes := []rune(trimmed)
	if len(runes) <= limit {
		return trimmed
	}
	if limit <= 3 {
		return string(runes[:limit])
	}
	return string(runes[:limit-3]) + "..."
}

func truncateForSummary(content string, limit int) string {
	trimmed := strings.TrimSpace(content)
	if limit <= 0 {
		return trimmed
	}
	runes := []rune(trimmed)
	if len(runes) <= limit {
		return trimmed
	}
	markerRunes := []rune("\n[TRUNCATED]\n")
	if limit <= len(markerRunes)+4 {
		if limit <= 0 {
			return ""
		}
		return string(runes[:limit]) + string(markerRunes)
	}
	available := limit - len(markerRunes)
	head := int(float64(available) * 0.6)
	if head < 1 {
		head = 1
	}
	tail := available - head
	if tail < 1 {
		tail = 1
		head = available - tail
	}
	if head+tail > len(runes) {
		return trimmed
	}
	return string(runes[:head]) + string(markerRunes) + string(runes[len(runes)-tail:])
}

func decodePersistedChatMessage(msg persistence.ChatMessage) llm.Message {
	raw := strings.TrimSpace(msg.Content)
	if msg.Role == "assistant" && strings.HasPrefix(raw, "{") {
		var data struct {
			Content   string         `json:"content"`
			ToolCalls []llm.ToolCall `json:"tool_calls"`
		}
		if err := json.Unmarshal([]byte(raw), &data); err == nil && len(data.ToolCalls) > 0 {
			return llm.Message{
				Role:      msg.Role,
				Content:   strings.TrimSpace(data.Content),
				ToolCalls: data.ToolCalls,
			}
		}
	}
	if msg.Role == "tool" && strings.HasPrefix(raw, "{") {
		var data struct {
			Content string `json:"content"`
			ToolID  string `json:"tool_id"`
		}
		if err := json.Unmarshal([]byte(raw), &data); err == nil {
			return llm.Message{
				Role:    msg.Role,
				Content: strings.TrimSpace(data.Content),
				ToolID:  strings.TrimSpace(data.ToolID),
			}
		}
	}
	return llm.Message{Role: msg.Role, Content: msg.Content}
}

func estimateMessagesTokens(msgs []llm.Message) int {
	est := 0
	for _, m := range msgs {
		c := strings.TrimSpace(m.Content)
		if c == "" {
			est++
			continue
		}
		est += len([]rune(c))/4 + 1
	}
	return est
}
