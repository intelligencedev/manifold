package agentd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"

	"manifold/internal/config"
)

// LogEntry represents a single log entry returned to the UI.
type LogEntry struct {
	Timestamp int64  `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	Service   string `json:"service,omitempty"`
	TraceID   string `json:"traceId,omitempty"`
	SpanID    string `json:"spanId,omitempty"`
}

type clickhouseLogMetrics struct {
	conn    clickhouse.Conn
	table   string
	timeout time.Duration
}

func newClickHouseLogMetrics(ctx context.Context, cfg config.ClickHouseConfig) (*clickhouseLogMetrics, error) {
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

	tableName := strings.TrimSpace(cfg.LogsTable)
	if tableName == "" {
		tableName = "logs"
	}
	table, err := sanitizeIdentifier(tableName, true)
	if err != nil {
		return nil, fmt.Errorf("invalid logs table: %w", err)
	}

	ctxPing, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := conn.Ping(ctxPing); err != nil {
		return nil, fmt.Errorf("clickhouse ping: %w", err)
	}

	return &clickhouseLogMetrics{conn: conn, table: table, timeout: timeout}, nil
}

func (c *clickhouseLogMetrics) Source() string {
	return "clickhouse"
}

func (c *clickhouseLogMetrics) Logs(ctx context.Context, window time.Duration, limit int) ([]LogEntry, time.Duration, error) {
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
	query := fmt.Sprintf(`
SELECT
  Timestamp,
  SeverityText,
  COALESCE(NULLIF(Body, ''), LogAttributes['message'], LogAttributes['msg']) AS body,
  ServiceName,
  TraceId,
  SpanId
FROM %s
WHERE Timestamp >= ?
  AND (ServiceName = 'manifold' OR ResourceAttributes['service.instance.id'] = 'manifold')
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

	var out []LogEntry
	for rows.Next() {
		var (
			ts      time.Time
			level   string
			message string
			service string
			traceID string
			spanID  string
		)
		if err := rows.Scan(&ts, &level, &message, &service, &traceID, &spanID); err != nil {
			return nil, 0, err
		}
		lvl := strings.ToLower(strings.TrimSpace(level))
		if lvl == "" {
			lvl = "info"
		}
		msg := strings.TrimSpace(message)
		out = append(out, LogEntry{
			Timestamp: ts.Unix(),
			Level:     lvl,
			Message:   msg,
			Service:   strings.TrimSpace(service),
			TraceID:   strings.TrimSpace(traceID),
			SpanID:    strings.TrimSpace(spanID),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return out, window, nil
}

func (c *clickhouseLogMetrics) LogsForUser(ctx context.Context, userID int64, window time.Duration, limit int) ([]LogEntry, time.Duration, error) {
	if userID == systemUserID {
		return c.Logs(ctx, window, limit)
	}
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
	query := fmt.Sprintf(`
SELECT
  Timestamp,
  SeverityText,
  COALESCE(NULLIF(Body, ''), LogAttributes['message'], LogAttributes['msg']) AS body,
  ServiceName,
  TraceId,
  SpanId
FROM %s
WHERE Timestamp >= ?
  AND LogAttributes['enduser.id'] = ?
  AND (ServiceName = 'manifold' OR ResourceAttributes['service.instance.id'] = 'manifold')
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

	var out []LogEntry
	for rows.Next() {
		var (
			ts      time.Time
			level   string
			message string
			service string
			traceID string
			spanID  string
		)
		if err := rows.Scan(&ts, &level, &message, &service, &traceID, &spanID); err != nil {
			return nil, 0, err
		}
		lvl := strings.ToLower(strings.TrimSpace(level))
		if lvl == "" {
			lvl = "info"
		}
		msg := strings.TrimSpace(message)
		out = append(out, LogEntry{
			Timestamp: ts.Unix(),
			Level:     lvl,
			Message:   msg,
			Service:   strings.TrimSpace(service),
			TraceID:   strings.TrimSpace(traceID),
			SpanID:    strings.TrimSpace(spanID),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return out, window, nil
}
