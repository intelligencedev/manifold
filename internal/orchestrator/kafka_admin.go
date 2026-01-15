//go:build enterprise
// +build enterprise

package orchestrator

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/segmentio/kafka-go"
)

// CheckBrokers attempts to dial the provided brokers to verify reachability.
func CheckBrokers(ctx context.Context, brokers []string, timeout time.Duration) error {
	if len(brokers) == 0 {
		return fmt.Errorf("no brokers provided")
	}

	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		for _, b := range brokers {
			conn, err := kafka.DialContext(ctx, "tcp", b)
			if err == nil {
				_ = conn.Close()
				return nil
			}
			lastErr = err
		}
		// small backoff
		select {
		case <-time.After(200 * time.Millisecond):
			// retry
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return fmt.Errorf("failed to reach any broker within %s: last error: %v", timeout, lastErr)
}

// EnsureTopics ensures that each topic exists; if missing it will create it using the cluster controller.
func EnsureTopics(ctx context.Context, brokers []string, configs []kafka.TopicConfig) error {
	if len(brokers) == 0 {
		return fmt.Errorf("no brokers provided")
	}

	// Dial any broker to locate the controller
	conn, err := kafka.DialContext(ctx, "tcp", brokers[0])
	if err != nil {
		return fmt.Errorf("failed to dial broker %s: %w", brokers[0], err)
	}
	defer conn.Close()

	controller, err := conn.Controller()
	if err != nil {
		return fmt.Errorf("failed to get controller: %w", err)
	}
	controllerAddr := net.JoinHostPort(controller.Host, fmt.Sprint(controller.Port))

	ctrlConn, err := kafka.DialContext(ctx, "tcp", controllerAddr)
	if err != nil {
		return fmt.Errorf("failed to dial controller %s: %w", controllerAddr, err)
	}
	defer ctrlConn.Close()

	for _, cfg := range configs {
		topic := cfg.Topic
		// Check if the topic already has partitions
		parts, err := ctrlConn.ReadPartitions(topic)
		if err != nil {
			// log and continue to attempt create
			log.Printf("read partitions for topic=%s error: %v", topic, err)
		}
		if len(parts) > 0 {
			log.Printf("topic exists: %s", topic)
			continue
		}

		// Create topic
		if err := ctrlConn.CreateTopics(cfg); err != nil {
			// If error indicates topic exists, ignore; otherwise return
			log.Printf("create topic %s failed: %v", topic, err)
			return fmt.Errorf("create topic %s: %w", topic, err)
		}
		log.Printf("created topic: %s", topic)
	}
	return nil
}
