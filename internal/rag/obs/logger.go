package obs

import (
    "encoding/json"
    "fmt"
    "os"
    "sync"
)

// JSONLogger is a minimal structured logger writing one JSON object per line.
type JSONLogger struct{
    mu sync.Mutex
}

func (l *JSONLogger) log(level, msg string, fields map[string]any) {
    l.mu.Lock()
    defer l.mu.Unlock()
    if fields == nil { fields = map[string]any{} }
    fields["level"] = level
    fields["msg"] = msg
    enc, err := json.Marshal(fields)
    if err != nil {
        // fallback
        _, _ = fmt.Fprintf(os.Stderr, "log marshal error: %v\n", err)
        return
    }
    _, _ = os.Stdout.Write(append(enc, '\n'))
}

func (l *JSONLogger) Info(msg string, fields map[string]any)  { l.log("info", msg, fields) }
func (l *JSONLogger) Error(msg string, fields map[string]any) { l.log("error", msg, fields) }
func (l *JSONLogger) Debug(msg string, fields map[string]any) { l.log("debug", msg, fields) }

