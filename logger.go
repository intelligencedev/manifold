package main

import (
	"os"

	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/bridges/otellogrus"
)

var log = logrus.New()

func init() {
	log.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
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

// AddOTelLogrusHook adds OpenTelemetry logrus bridge hook to the logger
func AddOTelLogrusHook(serviceName string) {
	hook := otellogrus.NewHook(serviceName)
	log.AddHook(hook)
}
