package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"

	"github.com/segmentio/kafka-go"
)

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

	ctx := context.Background()
	conn, err := kafka.DialContext(ctx, "tcp", brokers)
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	ctrl, err := conn.Controller()
	if err != nil {
		log.Fatalf("controller: %v", err)
	}
	ctrlAddr := net.JoinHostPort(ctrl.Host, fmt.Sprint(ctrl.Port))
	cw, err := kafka.DialContext(ctx, "tcp", ctrlAddr)
	if err != nil {
		log.Fatalf("dial controller: %v", err)
	}
	defer cw.Close()

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
			log.Fatalf("create topic %s: %v", t.Topic, err)
		}
		fmt.Printf("created topic: %s\n", t.Topic)
	}
}
