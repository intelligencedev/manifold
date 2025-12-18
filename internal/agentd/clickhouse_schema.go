package agentd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/rs/zerolog/log"

	"manifold/internal/config"
)

// ensureClickHouseTables validates that required ClickHouse tables exist and
// creates them if they don't. Uses OTel standard schema for traces, metrics, and logs.
func ensureClickHouseTables(ctx context.Context, cfg config.ClickHouseConfig) error {
	dsn := strings.TrimSpace(cfg.DSN)
	if dsn == "" {
		return nil
	}

	opts, err := clickhouse.ParseDSN(dsn)
	if err != nil {
		return fmt.Errorf("parse clickhouse dsn: %w", err)
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

	if opts.Auth.Database == "" {
		opts.Auth.Database = "otel"
	}

	conn, err := clickhouse.Open(opts)
	if err != nil {
		return fmt.Errorf("open clickhouse connection: %w", err)
	}
	defer conn.Close()

	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	ctxTimeout, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Ensure database exists
	dbName := opts.Auth.Database
	if err := conn.Exec(ctxTimeout, fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", dbName)); err != nil {
		return fmt.Errorf("create database %s: %w", dbName, err)
	}

	// Get table names from config with defaults
	metricsTable := strings.TrimSpace(cfg.MetricsTable)
	if metricsTable == "" {
		metricsTable = "metrics"
	}
	tracesTable := strings.TrimSpace(cfg.TracesTable)
	if tracesTable == "" {
		tracesTable = "traces"
	}
	logsTable := strings.TrimSpace(cfg.LogsTable)
	if logsTable == "" {
		logsTable = "logs"
	}

	// Create tables if they don't exist
	if err := createMetricsTableIfNotExists(ctxTimeout, conn, dbName, metricsTable); err != nil {
		log.Warn().Err(err).Msgf("failed to create metrics table %s", metricsTable)
	}

	if err := createTracesTableIfNotExists(ctxTimeout, conn, dbName, tracesTable); err != nil {
		log.Warn().Err(err).Msgf("failed to create traces table %s", tracesTable)
	}

	if err := createLogsTableIfNotExists(ctxTimeout, conn, dbName, logsTable); err != nil {
		log.Warn().Err(err).Msgf("failed to create logs table %s", logsTable)
	}

	return nil
}

// createMetricsTableIfNotExists creates the metrics table using OTel schema if it doesn't exist.
// Schema based on otelcol-contrib ClickHouse exporter.
func createMetricsTableIfNotExists(ctx context.Context, conn clickhouse.Conn, db, table string) error {
	sql := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s.%s (
	ResourceAttributes Map(LowCardinality(String), String),
	ResourceSchemaUrl String,
	ScopeName String,
	ScopeVersion String,
	ScopeAttributes Map(LowCardinality(String), String),
	ScopeDroppedAttrCount UInt32,
	ScopeSchemaUrl String,
	ServiceName LowCardinality(String),
	MetricName String,
	MetricDescription String,
	MetricUnit String,
	Attributes Map(LowCardinality(String), String),
	StartTimeUnix DateTime64(9),
	TimeUnix DateTime64(9),
	Value Float64,
	Flags UInt32,
	Exemplars Nested(
		FilteredAttributes Map(LowCardinality(String), String),
		TimeUnix DateTime64(9),
		Value Float64,
		SpanId String,
		TraceId String
	),
	AggTemp Int32,
	IsMonotonic Bool
) ENGINE = MergeTree()
ORDER BY (MetricName, ServiceName, TimeUnix)
TTL TimeUnix + INTERVAL 30 DAY
SETTINGS index_granularity = 8192
`, db, table)

	if err := conn.Exec(ctx, sql); err != nil {
		// Check if it's already-exists error; those are acceptable
		if !strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("create metrics table: %w", err)
		}
	}
	log.Info().Str("table", fmt.Sprintf("%s.%s", db, table)).Msg("metrics table ready")
	return nil
}

// createTracesTableIfNotExists creates the traces table using OTel schema if it doesn't exist.
// Schema based on otelcol-contrib ClickHouse exporter.
func createTracesTableIfNotExists(ctx context.Context, conn clickhouse.Conn, db, table string) error {
	sql := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s.%s (
	Timestamp DateTime64(9),
	TraceId String,
	SpanId String,
	ParentSpanId String,
	TraceState String,
	SpanName LowCardinality(String),
	SpanKind LowCardinality(String),
	ServiceName LowCardinality(String),
	ResourceAttributes Map(LowCardinality(String), String),
	ScopeName String,
	ScopeVersion String,
	SpanAttributes Map(LowCardinality(String), String),
	Duration Int64,
	StatusCode LowCardinality(String),
	StatusMessage String,
	Events Nested(
		Timestamp DateTime64(9),
		Name LowCardinality(String),
		Attributes Map(LowCardinality(String), String)
	),
	Links Nested(
		TraceId String,
		SpanId String,
		TraceState String,
		Attributes Map(LowCardinality(String), String)
	)
) ENGINE = MergeTree()
ORDER BY (ServiceName, SpanName, Timestamp)
TTL Timestamp + INTERVAL 30 DAY
SETTINGS index_granularity = 8192
`, db, table)

	if err := conn.Exec(ctx, sql); err != nil {
		// Check if it's already-exists error; those are acceptable
		if !strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("create traces table: %w", err)
		}
	}
	log.Info().Str("table", fmt.Sprintf("%s.%s", db, table)).Msg("traces table ready")
	return nil
}

// createLogsTableIfNotExists creates the logs table using OTel schema if it doesn't exist.
// Schema based on otelcol-contrib ClickHouse exporter.
func createLogsTableIfNotExists(ctx context.Context, conn clickhouse.Conn, db, table string) error {
	sql := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s.%s (
	Timestamp DateTime64(9),
	TraceId String,
	SpanId String,
	TraceFlags UInt32,
	SeverityText LowCardinality(String),
	SeverityNumber Int32,
	ServiceName LowCardinality(String),
	Body String,
	ResourceSchemaUrl String,
	ResourceAttributes Map(LowCardinality(String), String),
	ScopeSchemaUrl String,
	ScopeName String,
	ScopeVersion String,
	ScopeAttributes Map(LowCardinality(String), String),
	LogAttributes Map(LowCardinality(String), String)
) ENGINE = MergeTree()
ORDER BY (Timestamp)
TTL Timestamp + INTERVAL 30 DAY
SETTINGS index_granularity = 8192
`, db, table)

	if err := conn.Exec(ctx, sql); err != nil {
		// Check if it's already-exists error; those are acceptable
		if !strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("create logs table: %w", err)
		}
	}
	log.Info().Str("table", fmt.Sprintf("%s.%s", db, table)).Msg("logs table ready")
	return nil
}
