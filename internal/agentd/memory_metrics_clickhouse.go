package agentd

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"

	"manifold/internal/config"
)

const (
	memoryMetricSearchLatency = "evolving_memory_search_latency_seconds"
	memoryMetricSearchTotal   = "evolving_memory_search_total"
	memoryMetricSearchHits    = "evolving_memory_search_hits"
	memoryMetricEvolveTotal   = "evolving_memory_evolve_total"
	memoryMetricSize          = "evolving_memory_size"
	memoryMetricSmartMerge    = "evolving_memory_smart_merge_total"
	memoryMetricPruned        = "evolving_memory_pruned_total"
)

type memoryMetricsProvider interface {
	MemoryMetrics(ctx context.Context, userID int64, window time.Duration) (memoryMetricsSnapshot, time.Duration, error)
	Source() string
}

type memoryMetricTotals struct {
	Searches         int64   `json:"searches"`
	Hits             int64   `json:"hits"`
	AvgHitsPerSearch float64 `json:"avgHitsPerSearch"`
	Evolves          int64   `json:"evolves"`
	EvolveErrors     int64   `json:"evolveErrors"`
	SmartMerges      int64   `json:"smartMerges"`
	Pruned           int64   `json:"pruned"`
}

type memoryLatencyMetrics struct {
	AvgMs float64 `json:"avgMs,omitempty"`
}

type memorySizeMetric struct {
	User    string `json:"user"`
	Session string `json:"session"`
	Size    int64  `json:"size"`
}

type memoryReasonMetric struct {
	Reason string `json:"reason"`
	Count  int64  `json:"count"`
}

type memoryResultMetric struct {
	Result string `json:"result"`
	Count  int64  `json:"count"`
}

type memoryMetricsSnapshot struct {
	Totals          memoryMetricTotals   `json:"totals"`
	Latency         memoryLatencyMetrics `json:"latency"`
	Sizes           []memorySizeMetric   `json:"sizes"`
	PrunedByReason  []memoryReasonMetric `json:"prunedByReason"`
	EvolvesByResult []memoryResultMetric `json:"evolvesByResult"`
	Warnings        []string             `json:"warnings,omitempty"`
}

type clickhouseMemoryMetrics struct {
	conn            clickhouse.Conn
	sumTable        string
	gaugeTable      string
	histogramTable  string
	timestampColumn string
	valueColumn     string
	userAttrExpr    string
	sessionAttrExpr string
	resultAttrExpr  string
	reasonAttrExpr  string
	lookback        time.Duration
	timeout         time.Duration
}

func newClickHouseMemoryMetrics(ctx context.Context, cfg config.ClickHouseConfig) (*clickhouseMemoryMetrics, error) {
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
	} else if opts.Auth.Database == "" && opts.Settings != nil {
		if raw, ok := opts.Settings["database"]; ok {
			opts.Auth.Database = fmt.Sprint(raw)
			delete(opts.Settings, "database")
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
	pingCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := conn.Ping(pingCtx); err != nil {
		return nil, fmt.Errorf("clickhouse ping: %w", err)
	}

	sumTable, err := sanitizeIdentifier(cfg.MetricsTable, true)
	if err != nil {
		return nil, fmt.Errorf("invalid metrics table: %w", err)
	}
	timestampColumn, err := sanitizeIdentifier(cfg.TimestampColumn, false)
	if err != nil {
		return nil, fmt.Errorf("invalid timestamp column: %w", err)
	}
	valueColumn, err := sanitizeIdentifier(cfg.ValueColumn, false)
	if err != nil {
		return nil, fmt.Errorf("invalid value column: %w", err)
	}
	userAttrExpr, err := buildAttributeExpr("user")
	if err != nil {
		return nil, fmt.Errorf("invalid user attribute key: %w", err)
	}
	sessionAttrExpr, err := buildAttributeExpr("session")
	if err != nil {
		return nil, fmt.Errorf("invalid session attribute key: %w", err)
	}
	resultAttrExpr, err := buildAttributeExpr("result")
	if err != nil {
		return nil, fmt.Errorf("invalid result attribute key: %w", err)
	}
	reasonAttrExpr, err := buildAttributeExpr("reason")
	if err != nil {
		return nil, fmt.Errorf("invalid reason attribute key: %w", err)
	}

	lookback := time.Duration(cfg.LookbackHours) * time.Hour
	if lookback <= 0 {
		lookback = 24 * time.Hour
	}

	return &clickhouseMemoryMetrics{
		conn:            conn,
		sumTable:        sumTable,
		gaugeTable:      deriveMetricTableName(sumTable, "gauge"),
		histogramTable:  deriveMetricTableName(sumTable, "histogram"),
		timestampColumn: timestampColumn,
		valueColumn:     valueColumn,
		userAttrExpr:    userAttrExpr,
		sessionAttrExpr: sessionAttrExpr,
		resultAttrExpr:  resultAttrExpr,
		reasonAttrExpr:  reasonAttrExpr,
		lookback:        lookback,
		timeout:         timeout,
	}, nil
}

func deriveMetricTableName(sumTable, kind string) string {
	if strings.HasSuffix(sumTable, "_sum") {
		return strings.TrimSuffix(sumTable, "_sum") + "_" + kind
	}
	if strings.HasSuffix(sumTable, ".metrics_sum") {
		return strings.TrimSuffix(sumTable, "_sum") + "_" + kind
	}
	if sumTable == "metrics" || strings.HasSuffix(sumTable, ".metrics") {
		return sumTable + "_" + kind
	}
	return sumTable + "_" + kind
}

func (c *clickhouseMemoryMetrics) Source() string { return "clickhouse" }

func (c *clickhouseMemoryMetrics) MemoryMetrics(ctx context.Context, userID int64, window time.Duration) (memoryMetricsSnapshot, time.Duration, error) {
	if c.conn == nil {
		return memoryMetricsSnapshot{}, 0, errors.New("clickhouse connection is nil")
	}
	if window <= 0 {
		window = c.lookback
	}
	start := time.Now().Add(-window)
	var snapshot memoryMetricsSnapshot

	if err := c.loadCounterMetrics(ctx, &snapshot, userID, start); err != nil {
		return memoryMetricsSnapshot{}, 0, err
	}
	if err := c.loadLatencyMetrics(ctx, &snapshot, userID, start); err != nil {
		snapshot.Warnings = append(snapshot.Warnings, err.Error())
	}
	if err := c.loadSizeMetrics(ctx, &snapshot, userID, start); err != nil {
		snapshot.Warnings = append(snapshot.Warnings, err.Error())
	}
	if snapshot.Totals.Searches > 0 {
		snapshot.Totals.AvgHitsPerSearch = float64(snapshot.Totals.Hits) / float64(snapshot.Totals.Searches)
	}
	return snapshot, window, nil
}

func (c *clickhouseMemoryMetrics) loadCounterMetrics(ctx context.Context, snapshot *memoryMetricsSnapshot, userID int64, start time.Time) error {
	filter, args := c.userFilter(userID)
	args = append([]any{start}, args...)
	query := fmt.Sprintf(`
SELECT MetricName, result, reason, sum(delta) AS total
FROM (
    SELECT
		MetricName,
        %s AS result,
        %s AS reason,
        greatest(max(%s) - min(%s), 0) AS delta
    FROM %s
	WHERE MetricName IN (?, ?, ?, ?, ?)
        AND %s >= ?%s
    GROUP BY MetricName, Attributes
)
GROUP BY MetricName, result, reason
`, c.resultAttrExpr, c.reasonAttrExpr, c.valueColumn, c.valueColumn, c.sumTable, c.timestampColumn, filter)

	args = append([]any{memoryMetricSearchTotal, memoryMetricSearchHits, memoryMetricEvolveTotal, memoryMetricSmartMerge, memoryMetricPruned}, args...)
	execCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	rows, err := c.conn.Query(execCtx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var metricName, result, reason string
		var total float64
		if err := rows.Scan(&metricName, &result, &reason, &total); err != nil {
			return err
		}
		count := int64(math.Round(total))
		switch metricName {
		case memoryMetricSearchTotal:
			snapshot.Totals.Searches += count
		case memoryMetricSearchHits:
			snapshot.Totals.Hits += count
		case memoryMetricEvolveTotal:
			snapshot.Totals.Evolves += count
			if result == "" {
				result = "unknown"
			}
			if result == "error" {
				snapshot.Totals.EvolveErrors += count
			}
			snapshot.EvolvesByResult = append(snapshot.EvolvesByResult, memoryResultMetric{Result: result, Count: count})
		case memoryMetricSmartMerge:
			snapshot.Totals.SmartMerges += count
		case memoryMetricPruned:
			snapshot.Totals.Pruned += count
			if reason == "" {
				reason = "unknown"
			}
			snapshot.PrunedByReason = append(snapshot.PrunedByReason, memoryReasonMetric{Reason: reason, Count: count})
		}
	}
	return rows.Err()
}

func (c *clickhouseMemoryMetrics) loadLatencyMetrics(ctx context.Context, snapshot *memoryMetricsSnapshot, userID int64, start time.Time) error {
	filter, args := c.userFilter(userID)
	args = append([]any{memoryMetricSearchLatency, start}, args...)
	query := fmt.Sprintf(`
SELECT sum(count_delta) AS searches, sum(sum_delta) AS seconds
FROM (
    SELECT
        greatest(max(Count) - min(Count), 0) AS count_delta,
        greatest(max(Sum) - min(Sum), 0) AS sum_delta
    FROM %s
    WHERE MetricName = ?
        AND %s >= ?%s
    GROUP BY Attributes
)
`, c.histogramTable, c.timestampColumn, filter)

	execCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	row := c.conn.QueryRow(execCtx, query, args...)
	var searches uint64
	var seconds float64
	if err := row.Scan(&searches, &seconds); err != nil {
		if isClickHouseMissingTable(err) {
			return fmt.Errorf("memory latency metrics unavailable: %w", err)
		}
		return err
	}
	if snapshot.Totals.Searches == 0 {
		snapshot.Totals.Searches = int64(searches)
	}
	if searches > 0 {
		snapshot.Latency.AvgMs = seconds / float64(searches) * 1000
	}
	return nil
}

func (c *clickhouseMemoryMetrics) loadSizeMetrics(ctx context.Context, snapshot *memoryMetricsSnapshot, userID int64, start time.Time) error {
	filter, args := c.userFilter(userID)
	args = append([]any{memoryMetricSize, start}, args...)
	query := fmt.Sprintf(`
SELECT %s AS user_id, %s AS session_id, argMax(%s, %s) AS size
FROM %s
WHERE MetricName = ?
    AND %s >= ?%s
GROUP BY user_id, session_id
ORDER BY size DESC
LIMIT 20
`, c.userAttrExpr, c.sessionAttrExpr, c.valueColumn, c.timestampColumn, c.gaugeTable, c.timestampColumn, filter)

	execCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	rows, err := c.conn.Query(execCtx, query, args...)
	if err != nil {
		if isClickHouseMissingTable(err) {
			return fmt.Errorf("memory size metrics unavailable: %w", err)
		}
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var user, session string
		var size float64
		if err := rows.Scan(&user, &session, &size); err != nil {
			return err
		}
		if user == "" {
			user = "system"
		}
		if session == "" {
			session = "default"
		}
		snapshot.Sizes = append(snapshot.Sizes, memorySizeMetric{User: user, Session: session, Size: int64(math.Round(size))})
	}
	return rows.Err()
}

func (c *clickhouseMemoryMetrics) userFilter(userID int64) (string, []any) {
	if userID == systemUserID {
		return "", nil
	}
	return fmt.Sprintf(" AND %s = ?", c.userAttrExpr), []any{fmt.Sprint(userID)}
}

func isClickHouseMissingTable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unknown_table") || strings.Contains(msg, "doesn't exist") || strings.Contains(msg, "does not exist")
}
