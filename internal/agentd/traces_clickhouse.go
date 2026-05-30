package agentd

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"

	"manifold/internal/config"
	llmpkg "manifold/internal/llm"
)

// clickhouseTraceMetrics queries the ClickHouse traces table populated by the
// OpenTelemetry Collector clickhouse exporter.
//
// NOTE: The exporter schema for contrib v0.102.x stores span attributes in
// SpanAttributes (Map(String,String)) and duration in nanoseconds.
type clickhouseTraceMetrics struct {
	conn    clickhouse.Conn
	table   string
	timeout time.Duration
}

func newClickHouseTraceMetrics(ctx context.Context, cfg config.ClickHouseConfig) (*clickhouseTraceMetrics, error) {
	dsn := strings.TrimSpace(cfg.DSN)
	if dsn == "" {
		return nil, nil
	}

	opts, err := clickhouse.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse clickhouse dsn: %w", err)
	}
	if cfg.Database != "" {
		opts.Auth.Database = cfg.Database
	} else if opts.Auth.Database == "" {
		if opts.Settings != nil {
			if raw, ok := opts.Settings["database"]; ok {
				opts.Auth.Database = fmt.Sprint(raw)
				delete(opts.Settings, "database")
			}
		}
	}

	conn, err := clickhouse.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("open clickhouse connection: %w", err)
	}

	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	tableName := strings.TrimSpace(cfg.TracesTable)
	if tableName == "" {
		tableName = "traces"
	}
	table, err := sanitizeIdentifier(tableName, true)
	if err != nil {
		return nil, fmt.Errorf("invalid traces table: %w", err)
	}

	ctxPing, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := conn.Ping(ctxPing); err != nil {
		return nil, fmt.Errorf("clickhouse ping: %w", err)
	}

	return &clickhouseTraceMetrics{conn: conn, table: table, timeout: timeout}, nil
}

func (c *clickhouseTraceMetrics) Source() string {
	return "clickhouse"
}

func (c *clickhouseTraceMetrics) Traces(ctx context.Context, window time.Duration, limit int) ([]llmpkg.TraceSnapshot, time.Duration, error) {
	if c == nil || c.conn == nil {
		return nil, 0, errors.New("clickhouse connection is nil")
	}
	if limit <= 0 {
		limit = 200
	}
	if window <= 0 {
		window = 24 * time.Hour
	}

	start := time.Now().Add(-window)

	// Table schema (otelcol-contrib clickhouse exporter v0.102.x):
	// Timestamp: DateTime64(9) (span start), Duration: Int64 (nanoseconds),
	// StatusCode: LowCardinality(String), SpanName: LowCardinality(String),
	// TraceId: String, SpanAttributes: Map(String,String)
	query := fmt.Sprintf(`
SELECT
  TraceId,
  SpanName,
  SpanAttributes['llm.model'] AS model,
  StatusCode,
  Duration,
  Timestamp,
  SpanAttributes['llm.prompt_tokens'] AS prompt_tokens,
  SpanAttributes['llm.completion_tokens'] AS completion_tokens,
  SpanAttributes['llm.total_tokens'] AS total_tokens
FROM %s
WHERE Timestamp >= ?
  AND ResourceAttributes['service.instance.id'] = 'manifold'
  AND (SpanAttributes['llm.model'] != '' OR SpanName LIKE '%%Chat%%')
ORDER BY Timestamp DESC
LIMIT ?
`, c.table)

	execCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	rows, err := c.conn.Query(execCtx, query, start, limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []llmpkg.TraceSnapshot
	for rows.Next() {
		var (
			traceID    string
			spanName   string
			model      string
			statusCode string
			durationNS int64
			ts         time.Time
			promptStr  string
			compStr    string
			totalStr   string
		)
		if err := rows.Scan(&traceID, &spanName, &model, &statusCode, &durationNS, &ts, &promptStr, &compStr, &totalStr); err != nil {
			return nil, 0, err
		}
		promptTokens := parseInt64Loose(promptStr)
		completionTokens := parseInt64Loose(compStr)
		totalTokens := parseInt64Loose(totalStr)
		if totalTokens == 0 {
			totalTokens = promptTokens + completionTokens
		}
		status := "ok"
		if strings.Contains(strings.ToUpper(statusCode), "ERROR") {
			status = "error"
		}
		durationMS := int64(0)
		if durationNS > 0 {
			durationMS = durationNS / int64(time.Millisecond)
		}
		name := strings.TrimSpace(spanName)
		endTS := ts
		if durationNS > 0 {
			endTS = ts.Add(time.Duration(durationNS))
		}

		out = append(out, llmpkg.TraceSnapshot{
			TraceID:          strings.TrimSpace(traceID),
			Name:             name,
			Model:            strings.TrimSpace(model),
			Status:           status,
			DurationMillis:   durationMS,
			Timestamp:        endTS.Unix(),
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      totalTokens,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	// We don't attempt to compute an accurate "applied" window based on oldest
	// returned row; just echo window for now.
	return out, window, nil
}

func (c *clickhouseTraceMetrics) TracesForUser(ctx context.Context, userID int64, window time.Duration, limit int) ([]llmpkg.TraceSnapshot, time.Duration, error) {
	if userID == systemUserID {
		return c.Traces(ctx, window, limit)
	}
	if c == nil || c.conn == nil {
		return nil, 0, errors.New("clickhouse connection is nil")
	}
	if limit <= 0 {
		limit = 200
	}
	if window < 0 {
		window = 24 * time.Hour
	}

	start := time.Now().Add(-window)
	query := fmt.Sprintf(`
SELECT
  TraceId,
  SpanName,
  SpanAttributes['llm.model'] AS model,
  StatusCode,
  Duration,
  Timestamp,
  SpanAttributes['llm.prompt_tokens'] AS prompt_tokens,
  SpanAttributes['llm.completion_tokens'] AS completion_tokens,
  SpanAttributes['llm.total_tokens'] AS total_tokens
FROM %s
WHERE Timestamp >= ?
  AND SpanAttributes['enduser.id'] = ?
  AND ResourceAttributes['service.instance.id'] = 'manifold'
  AND (SpanAttributes['llm.model'] != '' OR SpanName LIKE '%%Chat%%')
ORDER BY Timestamp DESC
LIMIT ?
`, c.table)

	execCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	rows, err := c.conn.Query(execCtx, query, start, fmt.Sprint(userID), limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []llmpkg.TraceSnapshot
	for rows.Next() {
		var (
			traceID    string
			spanName   string
			model      string
			statusCode string
			durationNS int64
			ts         time.Time
			promptStr  string
			compStr    string
			totalStr   string
		)
		if err := rows.Scan(
			&traceID,
			&spanName,
			&model,
			&statusCode,
			&durationNS,
			&ts,
			&promptStr,
			&compStr,
			&totalStr,
		); err != nil {
			return nil, 0, err
		}
		promptTokens := parseInt64Loose(promptStr)
		completionTokens := parseInt64Loose(compStr)
		totalTokens := parseInt64Loose(totalStr)
		if totalTokens == 0 {
			totalTokens = promptTokens + completionTokens
		}
		status := "ok"
		if strings.Contains(strings.ToUpper(statusCode), "ERROR") {
			status = "error"
		}
		durationMS := int64(0)
		if durationNS > 0 {
			durationMS = time.Duration(durationNS).Milliseconds()
		}
		name := strings.TrimSpace(spanName)
		endTS := ts
		if durationNS > 0 {
			endTS = ts.Add(time.Duration(durationNS))
		}

		out = append(out, llmpkg.TraceSnapshot{
			TraceID:          strings.TrimSpace(traceID),
			Name:             name,
			Model:            strings.TrimSpace(model),
			Status:           status,
			DurationMillis:   durationMS,
			Timestamp:        endTS.Unix(),
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      totalTokens,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, window, nil
}

// clickhouseRunMetrics derives recent "runs" from ClickHouse traces.
// A run is modeled as an LLM span (a completed request).
type clickhouseRunMetrics struct {
	traces *clickhouseTraceMetrics
}

func newClickHouseRunMetrics(tm *clickhouseTraceMetrics) *clickhouseRunMetrics {
	if tm == nil {
		return nil
	}
	return &clickhouseRunMetrics{traces: tm}
}

func (c *clickhouseRunMetrics) RecentRuns(ctx context.Context, window time.Duration, limit int) ([]AgentRun, error) {
	if c == nil || c.traces == nil {
		return nil, nil
	}
	window, limit = normalizeRecentRunsArgs(window, limit)
	execCtx, cancel := context.WithTimeout(ctx, c.traces.timeout)
	defer cancel()
	rows, err := c.traces.conn.Query(execCtx, recentRunsQuery(c.traces.table), time.Now().Add(-window), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRecentRuns(rows, limit)
}

func normalizeRecentRunsArgs(window time.Duration, limit int) (time.Duration, int) {
	if limit <= 0 {
		limit = 50
	}
	if window <= 0 {
		window = 24 * time.Hour
	}
	return window, limit
}

func recentRunsQuery(table string) string {
	return fmt.Sprintf(`
SELECT
  TraceId,
  SpanName,
  SpanAttributes['llm.prompt_preview'] AS prompt_preview,
  SpanAttributes['llm.model'] AS model,
  StatusCode,
  Duration,
  Timestamp,
  SpanAttributes['llm.prompt_tokens'] AS prompt_tokens,
  SpanAttributes['llm.completion_tokens'] AS completion_tokens,
  SpanAttributes['llm.total_tokens'] AS total_tokens
FROM %s
WHERE Timestamp >= ?
  AND ResourceAttributes['service.instance.id'] = 'manifold'
  AND (SpanAttributes['llm.model'] != '' OR SpanName LIKE '%%Chat%%')
ORDER BY Timestamp DESC
LIMIT ?
`, table)
}

type recentRunRow struct {
	traceID    string
	spanName   string
	promptPrev string
	model      string
	statusCode string
	durationNS int64
	startTS    time.Time
	promptStr  string
	compStr    string
	totalStr   string
}

func scanRecentRuns(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}, limit int) ([]AgentRun, error) {
	out := make([]AgentRun, 0, limit)
	for rows.Next() {
		row, err := scanRecentRunRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, agentRunFromClickHouseRow(row))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func scanRecentRunRow(rows interface{ Scan(dest ...any) error }) (recentRunRow, error) {
	var row recentRunRow
	err := rows.Scan(
		&row.traceID,
		&row.spanName,
		&row.promptPrev,
		&row.model,
		&row.statusCode,
		&row.durationNS,
		&row.startTS,
		&row.promptStr,
		&row.compStr,
		&row.totalStr,
	)
	return row, err
}

func agentRunFromClickHouseRow(row recentRunRow) AgentRun {
	return AgentRun{
		ID:        strings.TrimSpace(row.traceID),
		Prompt:    recentRunPrompt(row),
		CreatedAt: recentRunCreatedAt(row).UTC().Format(time.RFC3339),
		Status:    recentRunStatus(row.statusCode),
		Tokens:    int(recentRunTokens(row)),
	}
}

func recentRunStatus(statusCode string) string {
	if strings.Contains(strings.ToUpper(statusCode), "ERROR") {
		return "failed"
	}
	return "completed"
}

func recentRunTokens(row recentRunRow) int64 {
	promptTokens := parseInt64Loose(row.promptStr)
	completionTokens := parseInt64Loose(row.compStr)
	totalTokens := parseInt64Loose(row.totalStr)
	if totalTokens == 0 {
		return promptTokens + completionTokens
	}
	return totalTokens
}

func recentRunCreatedAt(row recentRunRow) time.Time {
	if row.durationNS > 0 {
		return row.startTS.Add(time.Duration(row.durationNS))
	}
	return row.startTS
}

func recentRunPrompt(row recentRunRow) string {
	for _, candidate := range []string{row.promptPrev, row.spanName, row.model} {
		if prompt := strings.TrimSpace(candidate); prompt != "" {
			return prompt
		}
	}
	return "LLM run"
}

func parseInt64Loose(raw string) int64 {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0
	}
	// Sometimes exporters store numeric values as strings; tolerate floats.
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return int64(f)
	}
	return 0
}
