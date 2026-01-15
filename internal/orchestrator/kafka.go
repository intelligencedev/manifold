//go:build enterprise
// +build enterprise

package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
)

// StartKafkaConsumer starts a consumer that reads command messages from the given
// topic and processes them using a worker pool. Messages are committed only after
// successful handling (or DLQ publication after retries on transient errors).
func StartKafkaConsumer(
	ctx context.Context,
	brokers []string,
	groupID string,
	commandsTopic string,
	readerConfig *kafka.ReaderConfig, // optional override; if nil, a default config is used
	producer *kafka.Writer,
	runner Runner,
	dedupe DedupeStore,
	workerCount int,
	defaultReplyTopic string,
	dedupeTTL time.Duration,
	workflowTimeout time.Duration,
) error {
	// Build reader configuration.
	rc := kafka.ReaderConfig{
		Brokers:  brokers,
		GroupID:  groupID,
		Topic:    commandsTopic,
		MinBytes: 1,
		MaxBytes: 10e6, // ~10MB
	}
	if readerConfig != nil {
		// Apply provided config but enforce brokers, group, and topic.
		rc = *readerConfig
		rc.Brokers = brokers
		rc.GroupID = groupID
		rc.Topic = commandsTopic
		if rc.MinBytes == 0 {
			rc.MinBytes = 1
		}
		if rc.MaxBytes == 0 {
			rc.MaxBytes = 10e6
		}
	}

	reader := kafka.NewReader(rc)
	defer func() {
		if err := reader.Close(); err != nil {
			log.Printf("error closing Kafka reader: %v", err)
		}
	}()

	jobs := make(chan kafka.Message, max(64, workerCount*4))

	var wg sync.WaitGroup
	wg.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go func(workerID int) {
			defer wg.Done()
			for msg := range jobs {
				// Try to handle with limited retries on transient errors.
				maxAttempts := 3
				attempt := 0
				var lastErr error
				for {
					attempt++
					if err := HandleCommandMessage(ctx, runner, dedupe, producer, msg, defaultReplyTopic, dedupeTTL, workflowTimeout); err != nil {
						lastErr = err
						// Backoff and retry unless we've exhausted attempts or context canceled.
						if attempt < maxAttempts && ctx.Err() == nil {
							backoff := time.Duration(200*(1<<uint(attempt-1))) * time.Millisecond
							log.Printf("worker=%d transient error, will retry (attempt=%d/%d, sleep=%s): %v", workerID, attempt, maxAttempts, backoff, err)
							sleepCtx, cancel := context.WithTimeout(ctx, backoff)
							<-sleepCtx.Done()
							cancel()
							continue
						}

						// Retries exhausted or context canceled: publish DLQ and break.
						publishDLQAfterRetries(ctx, producer, msg, defaultReplyTopic, attempt, lastErr)
					} else {
						// Success: break.
						lastErr = nil
					}
					break
				}

				// Commit message regardless of outcome (success or DLQ after retries).
				if err := reader.CommitMessages(ctx, msg); err != nil {
					log.Printf("commit failed (topic=%s partition=%d offset=%d): %v", msg.Topic, msg.Partition, msg.Offset, err)
				} else {
					log.Printf("committed message (topic=%s partition=%d offset=%d)", msg.Topic, msg.Partition, msg.Offset)
				}
			}
		}(i)
	}

	// Reader loop: fetch messages and enqueue into jobs channel.
	go func() {
		defer close(jobs)
		for {
			if ctx.Err() != nil {
				return
			}
			m, err := reader.FetchMessage(ctx)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return
				}
				// Log and continue on transient fetch errors.
				log.Printf("fetch error: %v", err)
				// Small delay to avoid tight error loop.
				t := time.NewTimer(500 * time.Millisecond)
				select {
				case <-t.C:
				case <-ctx.Done():
					if !t.Stop() {
						<-t.C
					}
					return
				}
				continue
			}

			select {
			case jobs <- m:
				// queued
			case <-ctx.Done():
				// Stop reading; message will be re-fetched later since it's not committed yet.
				return
			}
		}
	}()

	// Wait for workers to drain once the context is canceled and jobs channel is closed.
	wg.Wait()
	return ctx.Err()
}

func publishDLQAfterRetries(ctx context.Context, producer *kafka.Writer, msg kafka.Message, defaultReplyTopic string, attempts int, lastErr error) {
	// Try to extract reply topic and correlation id from the message body; fall back to defaults.
	replyTopic := defaultReplyTopic
	corrID := string(msg.Key)
	var cmd CommandEnvelope
	if err := json.Unmarshal(msg.Value, &cmd); err == nil {
		if cmd.ReplyTopic != "" {
			replyTopic = cmd.ReplyTopic
		}
		if cmd.CorrelationID != "" {
			corrID = cmd.CorrelationID
		}
	}

	dlq := ResponseEnvelope{
		CorrelationID: corrID,
		Status:        "error",
		Error:         fmt.Sprintf("transient failure after %d attempts: %v", attempts, lastErr),
	}
	payload, _ := json.Marshal(dlq)
	dlqTopic := replyTopic + ".dlq"
	if err := producer.WriteMessages(ctx, kafka.Message{Topic: dlqTopic, Key: []byte(corrID), Value: payload}); err != nil {
		log.Printf("failed to publish DLQ after retries (corr_id=%s): %v", corrID, err)
	} else {
		log.Printf("published DLQ after retries (corr_id=%s) to topic=%s", corrID, dlqTopic)
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
