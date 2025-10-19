package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
)

func main() {
	r := kafka.NewReader(kafka.ReaderConfig{Brokers: []string{"localhost:9092"}, GroupID: "debug-resp-reader-1", Topic: "dev.manifold.orchestrator.responses", MinBytes: 1, MaxBytes: 10e6})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	defer r.Close()

	for {
		m, err := r.FetchMessage(ctx)
		if err != nil {
			fmt.Println("fetch err:", err)
			return
		}
		var v map[string]interface{}
		if err := json.Unmarshal(m.Value, &v); err != nil {
			fmt.Println("unmarshal err:", err)
			_ = r.CommitMessages(context.Background(), m)
			continue
		}
		b, _ := json.MarshalIndent(v, "", "  ")
		fmt.Println(string(b))
		_ = r.CommitMessages(context.Background(), m)
	}
}
