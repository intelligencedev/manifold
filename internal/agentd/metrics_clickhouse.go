package agentd

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"

	"manifold/internal/config"
	llmpkg "manifold/internal/llm"
)

type clickhouseTokenMetrics struct {
	conn              clickhouse.Conn
	table             string
	timestampColumn   string
	valueColumn       string
	attributeExpr     string
	userAttributeExpr string
	promptMetric      string
	completionMetric  string
	lookback          time.Duration
	timeout           time.Duration
}

func newClickHouseTokenMetrics(ctx context.Context, cfg config.ClickHouseConfig) (tokenMetricsProvider, error) {
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
				switch v := raw.(type) {
				case string:
					opts.Auth.Database = v
				case fmt.Stringer:
					opts.Auth.Database = v.String()
				default:
					opts.Auth.Database = fmt.Sprint(v)
				}
				delete(opts.Settings, "database")
			}
		}
	}

	conn, err := clickhouse.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("open clickhouse connection: %w", err)
	}
	lookback := time.Duration(cfg.LookbackHours) * time.Hour
	if lookback <= 0 {
		lookback = 24 * time.Hour
	}
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	table, err := sanitizeIdentifier(cfg.MetricsTable, true)
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
	attributeExpr, err := buildAttributeExpr(cfg.ModelAttributeKey)
	if err != nil {
		return nil, fmt.Errorf("invalid model attribute key: %w", err)
	}
	userAttributeExpr, err := buildAttributeExpr("enduser.id")
	if err != nil {
		return nil, fmt.Errorf("invalid user attribute key: %w", err)
	}

	ctxPing, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := conn.Ping(ctxPing); err != nil {
		return nil, fmt.Errorf("clickhouse ping: %w", err)
	}

	return &clickhouseTokenMetrics{
		conn:              conn,
		table:             table,
		timestampColumn:   timestampColumn,
		valueColumn:       valueColumn,
		attributeExpr:     attributeExpr,
		userAttributeExpr: userAttributeExpr,
		promptMetric:      cfg.PromptMetricName,
		completionMetric:  cfg.CompletionMetricName,
		lookback:          lookback,
		timeout:           timeout,
	}, nil
}

func (c *clickhouseTokenMetrics) TokenTotals(ctx context.Context, window time.Duration) ([]llmpkg.TokenTotal, time.Duration, error) {
	if c.conn == nil {
		return nil, 0, errors.New("clickhouse connection is nil")
	}

	if window <= 0 {
		window = c.lookback
	}

	start := time.Now().Add(-window)
	query := fmt.Sprintf(`
SELECT
    %s AS model,
		MetricName,
		greatest(max(%s) - min(%s), 0) AS total_tokens
FROM %s
	WHERE MetricName IN (?, ?)
		AND %s >= ?
	GROUP BY model, MetricName
`, c.attributeExpr, c.valueColumn, c.valueColumn, c.table, c.timestampColumn)

	execCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	rows, err := c.conn.Query(execCtx, query, c.promptMetric, c.completionMetric, start)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	totals := make(map[string]llmpkg.TokenTotal)
	for rows.Next() {
		var model string
		var metric string
		var total float64
		if err := rows.Scan(&model, &metric, &total); err != nil {
			return nil, 0, err
		}
		model = strings.TrimSpace(model)
		if model == "" {
			model = "unknown"
		}
		entry := totals[model]
		entry.Model = model
		switch metric {
		case c.promptMetric:
			entry.Prompt = int64(math.Round(total))
		case c.completionMetric:
			entry.Completion = int64(math.Round(total))
		default:
			continue
		}
		entry.Total = entry.Prompt + entry.Completion
		totals[model] = entry
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	out := make([]llmpkg.TokenTotal, 0, len(totals))
	for _, v := range totals {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Total == out[j].Total {
			return out[i].Model < out[j].Model
		}
		return out[i].Total > out[j].Total
	})

	return out, window, nil
}

func (c *clickhouseTokenMetrics) TokenTotalsForUser(ctx context.Context, userID int64, window time.Duration) ([]llmpkg.TokenTotal, time.Duration, error) {
	if userID == systemUserID {
		return c.TokenTotals(ctx, window)
	}
	if c.conn == nil {
		return nil, 0, errors.New("clickhouse connection is nil")
	}
	if window <= 0 {
		window = c.lookback
	}
	start := time.Now().Add(-window)
	query := fmt.Sprintf(`
SELECT
    %s AS model,
		MetricName,
		greatest(max(%s) - min(%s), 0) AS total_tokens
FROM %s
	WHERE MetricName IN (?, ?)
		AND %s >= ?
		AND %s = ?
	GROUP BY model, MetricName
`, c.attributeExpr, c.valueColumn, c.valueColumn, c.table, c.timestampColumn, c.userAttributeExpr)

	execCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	rows, err := c.conn.Query(execCtx, query, c.promptMetric, c.completionMetric, start, fmt.Sprint(userID))
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	totals := make(map[string]llmpkg.TokenTotal)
	for rows.Next() {
		var model string
		var metric string
		var total float64
		if err := rows.Scan(&model, &metric, &total); err != nil {
			return nil, 0, err
		}
		model = strings.TrimSpace(model)
		if model == "" {
			model = "unknown"
		}
		entry := totals[model]
		entry.Model = model
		switch metric {
		case c.promptMetric:
			entry.Prompt = int64(math.Round(total))
		case c.completionMetric:
			entry.Completion = int64(math.Round(total))
		default:
			continue
		}
		entry.Total = entry.Prompt + entry.Completion
		totals[model] = entry
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	out := make([]llmpkg.TokenTotal, 0, len(totals))
	for _, v := range totals {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Total == out[j].Total {
			return out[i].Model < out[j].Model
		}
		return out[i].Total > out[j].Total
	})
	return out, window, nil
}

func (c *clickhouseTokenMetrics) Source() string {
	return "clickhouse"
}

var (
	identNoDotPattern   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	identWithDotPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$`)
	attrKeyPattern      = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
)

func sanitizeIdentifier(input string, allowDot bool) (string, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return "", errors.New("identifier is empty")
	}
	pattern := identNoDotPattern
	if allowDot {
		pattern = identWithDotPattern
	}
	if !pattern.MatchString(s) {
		return "", fmt.Errorf("identifier contains invalid characters: %s", s)
	}
	return s, nil
}

func buildAttributeExpr(key string) (string, error) {
	s := strings.TrimSpace(key)
	if s == "" {
		return "", errors.New("attribute key is empty")
	}
	if !attrKeyPattern.MatchString(s) {
		return "", fmt.Errorf("attribute key contains invalid characters: %s", s)
	}
	return fmt.Sprintf("COALESCE(Attributes['%s'], '')", s), nil
}
