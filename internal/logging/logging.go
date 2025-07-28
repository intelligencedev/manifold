package logging

import (
    "fmt"
    "io"
    "os"
    "path/filepath"
    "runtime"
    "strings"
    "time"

    "github.com/sirupsen/logrus"
)

// Log is the application wide logger configured with JSON output.
var Log = logrus.New()

type contextHook struct{}

func (contextHook) Levels() []logrus.Level { return logrus.AllLevels }

func packageFromFunc(fn string) string {
    if i := strings.LastIndex(fn, "/"); i >= 0 {
        fn = fn[i+1:]
    }
    if i := strings.Index(fn, "."); i >= 0 {
        return fn[:i]
    }
    return fn
}

func (contextHook) Fire(e *logrus.Entry) error {
    if e.Caller == nil {
        return nil
    }
    pkg := packageFromFunc(e.Caller.Function)
    file := fmt.Sprintf("%s:%d", filepath.Base(e.Caller.File), e.Caller.Line)
    e.Data["package"] = pkg
    e.Data["file"] = file
    return nil
}

func init() {
    Log.SetReportCaller(true)
    Log.SetFormatter(&logrus.JSONFormatter{
        TimestampFormat: time.RFC3339Nano,
        CallerPrettyfier: func(f *runtime.Frame) (string, string) {
            function := filepath.Base(f.Function)
            file := fmt.Sprintf("%s:%d", filepath.Base(f.File), f.Line)
            return function, file
        },
    })
    Log.AddHook(contextHook{})

    logPath := "manifold.log"
    logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
    if err != nil {
        Log.SetOutput(os.Stdout)
    } else {
        mw := io.MultiWriter(os.Stdout, logFile)
        Log.SetOutput(mw)
    }

    levelStr := os.Getenv("LOG_LEVEL")
    if levelStr == "" {
        levelStr = "info"
    }
    if lvl, err := logrus.ParseLevel(levelStr); err == nil {
        Log.SetLevel(lvl)
    } else {
        Log.SetLevel(logrus.InfoLevel)
    }
}

