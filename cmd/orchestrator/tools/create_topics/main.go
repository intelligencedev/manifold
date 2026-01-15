//go:build enterprise
// +build enterprise

package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/segmentio/kafka-go"
)

func parseCSV(s string) []string {
	fields := strings.Split(s, ",")
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

func main() {
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" {
		brokers = "localhost:9092"
	}
	commands := os.Getenv("KAFKA_COMMANDS_TOPIC")
	if commands == "" {
		commands = "dev.manifold.orchestrator.commands"
	}
	responses := os.Getenv("KAFKA_RESPONSES_TOPIC")
	if responses == "" {
		responses = "dev.manifold.orchestrator.responses"
	}
	brokerList := parseCSV(brokers)
	if len(brokerList) == 0 {
		log.Fatal().Msg("no Kafka brokers configured")
	}

	ctx := context.Background()
	conn, err := kafka.DialContext(ctx, "tcp", brokerList[0])
	if err != nil {
		log.Fatal().Err(err).Msg("dial")
	}
	defer func() { _ = conn.Close() }()

	ctrl, err := conn.Controller()
	if err != nil {
		log.Fatal().Err(err).Msg("controller")
	}
	ctrlAddr := net.JoinHostPort(ctrl.Host, fmt.Sprint(ctrl.Port))
	cw, err := kafka.DialContext(ctx, "tcp", ctrlAddr)
	if err != nil {
		log.Fatal().Err(err).Msg("dial controller")
	}
	defer func() { _ = cw.Close() }()

	topics := []kafka.TopicConfig{
		{Topic: commands, NumPartitions: 1, ReplicationFactor: 1},
		{Topic: responses, NumPartitions: 1, ReplicationFactor: 1},
	}
	for _, t := range topics {
		parts, err := cw.ReadPartitions(t.Topic)
		if err == nil && len(parts) > 0 {
			fmt.Printf("topic exists: %s\n", t.Topic)
			continue
		}
		if err := cw.CreateTopics(t); err != nil {
			log.Fatal().Err(err).Str("topic", t.Topic).Msg("create topic")
		}
		fmt.Printf("created topic: %s\n", t.Topic)
	}
}
