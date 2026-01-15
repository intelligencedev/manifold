package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
)

// CommandEnvelope mirrors the orchestrator input message structure (minimal).
type CommandEnvelope struct {
	CorrelationID string         `json:"correlation_id"`
	Workflow      string         `json:"workflow,omitempty"`
	ReplyTopic    string         `json:"reply_topic,omitempty"`
	Attrs         map[string]any `json:"attrs,omitempty"`
}

// ResponseEnvelope mirrors the orchestrator output message structure (minimal).
type ResponseEnvelope struct {
	CorrelationID string         `json:"correlation_id"`
	Status        string         `json:"status"`
	Result        map[string]any `json:"result,omitempty"`
	Error         string         `json:"error,omitempty"`
}

func genID(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// fallback to time-based id
		return fmt.Sprintf("id-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

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
	// Allow override of brokers via flag/env for flexibility.
	brokersCSV := flag.String("brokers", "localhost:9092", "comma-separated Kafka brokers")
	commandsTopic := flag.String("commands-topic", "dev.manifold.orchestrator.commands", "commands topic")
	responsesTopic := flag.String("responses-topic", "dev.manifold.orchestrator.responses", "responses topic")
	timeout := flag.Duration("timeout", 15*time.Second, "wait timeout for response")
	flag.Parse()

	brokers := parseCSV(*brokersCSV)
	if len(brokers) == 0 {
		log.Fatal("no Kafka brokers configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	corr := genID(8)
	cmd := CommandEnvelope{
		CorrelationID: corr,
		Workflow:      "kafka_op",
		ReplyTopic:    *responsesTopic,
		// noop.json expects ${A.query}
		Attrs: map[string]any{"query": "go 1.24"},
	}
	payload, err := json.Marshal(cmd)
	if err != nil {
		log.Fatalf("failed to marshal command: %v", err)
	}

	// Produce message to commands topic
	w := kafka.NewWriter(kafka.WriterConfig{Brokers: brokers, Topic: *commandsTopic})
	defer func() {
		if err := w.Close(); err != nil {
			log.Printf("close writer: %v", err)
		}
	}()

	msg := kafka.Message{Key: []byte(corr), Value: payload}
	if err := w.WriteMessages(ctx, msg); err != nil {
		log.Fatalf("failed to write command message: %v", err)
	}
	fmt.Printf("published command corr_id=%s to topic=%s\n", corr, *commandsTopic)

	// Start reader for responses topic and wait for matching correlation id.
	r := kafka.NewReader(kafka.ReaderConfig{Brokers: brokers, GroupID: "integration-test-reader-" + corr, Topic: *responsesTopic, MinBytes: 1, MaxBytes: 10e6})
	defer func() {
		if err := r.Close(); err != nil {
			log.Printf("close reader: %v", err)
		}
	}()

	for {
		m, err := r.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				log.Fatalf("timeout waiting for response (corr_id=%s)", corr)
			}
			log.Fatalf("fetch error: %v", err)
		}
		var resp ResponseEnvelope
		if err := json.Unmarshal(m.Value, &resp); err != nil {
			// commit and continue
			_ = r.CommitMessages(context.Background(), m)
			continue
		}
		if resp.CorrelationID == corr {
			// print and exit
			b, _ := json.MarshalIndent(resp, "", "  ")
			fmt.Println(string(b))
			_ = r.CommitMessages(context.Background(), m)
			return
		}
		// Not our message; commit and continue
		_ = r.CommitMessages(context.Background(), m)
	}
}
