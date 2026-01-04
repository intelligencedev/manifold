package workspaces

import (
	"context"
	"encoding/json"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/segmentio/kafka-go"

	"manifold/internal/config"
)

// ProjectCommitEvent is emitted on project commits for async consumers.
type ProjectCommitEvent struct {
	TenantID         string    `json:"tenant_id"`
	ProjectID        string    `json:"project_id"`
	UserID           int64     `json:"user_id"`
	SessionID        string    `json:"session_id"`
	Generation       int64     `json:"generation"`
	SkillsGeneration int64     `json:"skills_generation"`
	ChangedPaths     []string  `json:"changed_paths"`
	Timestamp        time.Time `json:"timestamp"`
	CommitID         string    `json:"commit_id"`
}

// KafkaCommitPublisher publishes commit events.
type KafkaCommitPublisher struct {
	writer *kafka.Writer
}

// NewKafkaCommitPublisher builds a publisher when enabled.
func NewKafkaCommitPublisher(cfg config.ProjectsKafkaConfig) (*KafkaCommitPublisher, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	writer := &kafka.Writer{
		Addr:     kafka.TCP(cfg.Brokers),
		Topic:    cfg.Topic,
		Balancer: &kafka.LeastBytes{},
	}
	return &KafkaCommitPublisher{writer: writer}, nil
}

// Publish writes a commit event to Kafka.
func (p *KafkaCommitPublisher) Publish(ctx context.Context, ev ProjectCommitEvent) error {
	if p == nil || p.writer == nil {
		return nil
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	msg := kafka.Message{Value: payload, Time: time.Now()}
	return p.writer.WriteMessages(ctx, msg)
}

// Close shuts down the writer.
func (p *KafkaCommitPublisher) Close() {
	if p == nil || p.writer == nil {
		return
	}
	if err := p.writer.Close(); err != nil {
		log.Warn().Err(err).Msg("kafka_writer_close_failed")
	}
}
