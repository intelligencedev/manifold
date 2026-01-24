package observability

import (
	"context"
	"encoding/json"
	"time"

	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
)

// OTelWriter implements io.Writer and bridges zerolog output to OpenTelemetry logs.
// It parses JSON log entries from zerolog and emits them as OTLP log records.
type OTelWriter struct {
	logger log.Logger
}

// NewOTelWriter creates a new OTelWriter that sends logs to the global OTLP log provider.
func NewOTelWriter(name string) *OTelWriter {
	return &OTelWriter{
		logger: global.GetLoggerProvider().Logger(name),
	}
}

// Write implements io.Writer. It parses a zerolog JSON line and emits an OTLP log record.
func (w *OTelWriter) Write(p []byte) (n int, err error) {
	n = len(p)

	var entry map[string]any
	if err := json.Unmarshal(p, &entry); err != nil {
		// If we can't parse, emit raw message
		w.emitRaw(string(p))
		return n, nil
	}

	w.emitStructured(entry)
	return n, nil
}

func (w *OTelWriter) emitRaw(msg string) {
	ctx := context.Background()
	var rec log.Record
	rec.SetTimestamp(time.Now())
	rec.SetBody(log.StringValue(msg))
	rec.SetSeverity(log.SeverityInfo)
	w.logger.Emit(ctx, rec)
}

func (w *OTelWriter) emitStructured(entry map[string]any) {
	ctx := context.Background()
	var rec log.Record

	// Extract timestamp
	if ts, ok := entry["time"].(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			rec.SetTimestamp(t)
		} else {
			rec.SetTimestamp(time.Now())
		}
		delete(entry, "time")
	} else {
		rec.SetTimestamp(time.Now())
	}

	// Extract level -> severity
	if lvl, ok := entry["level"].(string); ok {
		rec.SetSeverity(zerologLevelToSeverity(lvl))
		rec.SetSeverityText(lvl)
		delete(entry, "level")
	} else {
		rec.SetSeverity(log.SeverityInfo)
		rec.SetSeverityText("info")
	}

	// Extract message -> body
	if msg, ok := entry["message"].(string); ok {
		rec.SetBody(log.StringValue(msg))
		delete(entry, "message")
	} else if msg, ok := entry["msg"].(string); ok {
		rec.SetBody(log.StringValue(msg))
		delete(entry, "msg")
	}

	// Remaining fields become attributes
	attrs := make([]log.KeyValue, 0, len(entry))
	for k, v := range entry {
		attrs = append(attrs, log.KeyValue{Key: k, Value: anyToLogValue(v)})
	}
	rec.AddAttributes(attrs...)

	w.logger.Emit(ctx, rec)
}

func zerologLevelToSeverity(level string) log.Severity {
	switch level {
	case "trace":
		return log.SeverityTrace
	case "debug":
		return log.SeverityDebug
	case "info":
		return log.SeverityInfo
	case "warn", "warning":
		return log.SeverityWarn
	case "error":
		return log.SeverityError
	case "fatal":
		return log.SeverityFatal
	case "panic":
		return log.SeverityFatal4
	default:
		return log.SeverityInfo
	}
}

func anyToLogValue(v any) log.Value {
	switch val := v.(type) {
	case string:
		return log.StringValue(val)
	case int:
		return log.IntValue(val)
	case int64:
		return log.Int64Value(val)
	case float64:
		return log.Float64Value(val)
	case bool:
		return log.BoolValue(val)
	case nil:
		return log.StringValue("")
	default:
		// For complex types, marshal to JSON string
		if b, err := json.Marshal(val); err == nil {
			return log.StringValue(string(b))
		}
		return log.StringValue("")
	}
}
