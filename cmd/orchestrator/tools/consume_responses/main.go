//go:build enterprise
// +build enterprise

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

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
	brokersCSV := flag.String("brokers", os.Getenv("KAFKA_BROKERS"), "comma-separated Kafka brokers")
	topic := flag.String("topic", os.Getenv("KAFKA_RESPONSES_TOPIC"), "responses topic")
	groupID := flag.String("group-id", "debug-resp-reader", "Kafka consumer group ID")
	timeout := flag.Duration("timeout", 10*time.Second, "how long to run")
	flag.Parse()

	if strings.TrimSpace(*brokersCSV) == "" {
		*brokersCSV = "localhost:9092"
	}
	if strings.TrimSpace(*topic) == "" {
		*topic = "dev.manifold.orchestrator.responses"
	}

	brokers := parseCSV(*brokersCSV)
	if len(brokers) == 0 {
		fmt.Fprintln(os.Stderr, "no Kafka brokers configured")
		os.Exit(2)
	}

	r := kafka.NewReader(kafka.ReaderConfig{Brokers: brokers, GroupID: *groupID, Topic: *topic, MinBytes: 1, MaxBytes: 10e6})
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	defer func() {
		if err := r.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "close reader:", err)
		}
	}()

	for {
		m, err := r.FetchMessage(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, "fetch:", err)
			return
		}
		var v map[string]interface{}
		if err := json.Unmarshal(m.Value, &v); err != nil {
			fmt.Fprintln(os.Stderr, "unmarshal:", err)
			_ = r.CommitMessages(context.Background(), m)
			continue
		}
		b, _ := json.MarshalIndent(v, "", "  ")
		fmt.Println(string(b))
		_ = r.CommitMessages(context.Background(), m)
	}
}
