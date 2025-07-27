package main

import (
	"io"
	"os"
	"time"

	"github.com/sirupsen/logrus"
)

var log = logrus.New()

func init() {
	log.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: time.RFC3339Nano, // OTLP prefers RFC‑3339
	})

	logPath := "manifold.log"
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		// Create the log file if it doesn't exist
		logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			panic(err)
		}
		defer logFile.Close()
	}

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		panic(err)
	}

	mw := io.MultiWriter(os.Stdout, logFile)
	// Output logs to stdout for log collectors
	log.SetOutput(mw)
	levelStr := os.Getenv("LOG_LEVEL")
	if levelStr == "" {
		levelStr = "info"
	}
	if level, err := logrus.ParseLevel(levelStr); err == nil {
		log.SetLevel(level)
	} else {
		log.SetLevel(logrus.InfoLevel)
	}
}
