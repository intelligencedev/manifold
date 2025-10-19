package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
)

// Runner represents the workflow executor used by the orchestrator. The result
// must be JSON-serializable.
type Runner interface {
	// Execute runs the workflow and returns a JSON-serializable result or an error.
	// The publish function may be used by the runner to emit per-step results.
	Execute(ctx context.Context, workflow string, attrs map[string]any, publish func(ctx context.Context, stepID string, payload []byte) error) (map[string]any, error)
}

// Producer abstracts the kafka writer behavior needed by the handler.
type Producer interface {
	WriteMessages(ctx context.Context, msgs ...kafka.Message) error
}

// CommandEnvelope is the expected input message structure.
type CommandEnvelope struct {
	CorrelationID string         `json:"correlation_id"`
	Workflow      string         `json:"workflow,omitempty"`
	ReplyTopic    string         `json:"reply_topic,omitempty"`
	Attrs         map[string]any `json:"attrs,omitempty"`
}

// ResponseEnvelope is the output message structure (for both success and DLQ).
type ResponseEnvelope struct {
	CorrelationID string         `json:"correlation_id"`
	Status        string         `json:"status"`
	Result        map[string]any `json:"result,omitempty"`
	Error         string         `json:"error,omitempty"`
}

// HandleCommandMessage processes a single Kafka message containing a command
// envelope. It publishes either a success response or a DLQ message. Transient
// errors are returned so the caller may retry; non-transient errors are handled
// internally and nil is returned to allow committing the offset.
func HandleCommandMessage(
	ctx context.Context,
	runner Runner,
	dedupe DedupeStore,
	producer Producer,
	msg kafka.Message,
	defaultReplyTopic string,
	dedupeTTL time.Duration,
	workflowTimeout time.Duration,
) error {
	// Best-effort correlation id for logs, even if the payload is malformed.
	corrIDForLog := string(msg.Key)

	var cmd CommandEnvelope
	if err := json.Unmarshal(msg.Value, &cmd); err != nil {
		// Malformed JSON -> DLQ and return nil so the caller can commit.
		replyTopic := defaultReplyTopic
		corr := corrIDForLog
		env := ResponseEnvelope{CorrelationID: corr, Status: "error", Error: fmt.Sprintf("malformed command JSON: %v", err)}
		payload, _ := json.Marshal(env)
		dlqTopic := dlqTopicFor(replyTopic)
		if werr := producer.WriteMessages(ctx, kafka.Message{Topic: dlqTopic, Key: []byte(corr), Value: payload}); werr != nil {
			log.Printf("failed to publish DLQ for malformed JSON (corr_id=%s): %v", corr, werr)
		} else {
			log.Printf("published DLQ for malformed JSON (corr_id=%s) to topic=%s", corr, dlqTopic)
		}
		return nil
	}

	corrID := cmd.CorrelationID
	if corrID == "" {
		// Missing correlation id is a permanent error.
		replyTopic := pickReplyTopic(cmd.ReplyTopic, defaultReplyTopic)
		env := ResponseEnvelope{CorrelationID: corrIDForLog, Status: "error", Error: "missing correlation_id"}
		payload, _ := json.Marshal(env)
		dlqTopic := dlqTopicFor(replyTopic)
		if werr := producer.WriteMessages(ctx, kafka.Message{Topic: dlqTopic, Key: []byte(corrIDForLog), Value: payload}); werr != nil {
			log.Printf("failed to publish DLQ for missing correlation_id: %v", werr)
		} else {
			log.Printf("published DLQ for missing correlation_id to topic=%s", dlqTopic)
		}
		return nil
	}
	corrIDForLog = corrID

	// Dedupe check by correlation id.
	if prev, err := dedupe.Get(ctx, corrID); err != nil {
		// Treat Redis errors as transient (retryable by caller).
		return fmt.Errorf("dedupe get failed: %w", err)
	} else if prev != "" {
		log.Printf("dedupe hit, skipping processing (corr_id=%s)", corrID)
		return nil
	}

	// Determine workflow: if missing, this is a non-transient error.
	workflow := strings.TrimSpace(cmd.Workflow)
	if workflow == "" {
		replyTopic := pickReplyTopic(cmd.ReplyTopic, defaultReplyTopic)
		env := ResponseEnvelope{CorrelationID: corrID, Status: "error", Error: "missing workflow"}
		payload, _ := json.Marshal(env)
		dlqTopic := dlqTopicFor(replyTopic)
		if werr := producer.WriteMessages(ctx, kafka.Message{Topic: dlqTopic, Key: []byte(corrID), Value: payload}); werr != nil {
			log.Printf("failed to publish DLQ for missing workflow (corr_id=%s): %v", corrID, werr)
		} else {
			log.Printf("published DLQ for missing workflow (corr_id=%s) to topic=%s", corrID, dlqTopic)
		}
		return nil
	}

	replyTopic := pickReplyTopic(cmd.ReplyTopic, defaultReplyTopic)

	// Execute the workflow with a timeout only if configured (>0). A zero or
	// negative duration disables the global workflow timeout to support long
	// running workflows while relying on per-step/tool timeouts for safety.
	var runCtx context.Context = ctx
	var cancel context.CancelFunc = func() {}
	if workflowTimeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, workflowTimeout)
	}
	defer cancel()

	// Build a publisher closure that the runner can call for per-step results.
	publishFn := func(pctx context.Context, stepID string, payload []byte) error {
		// Extract original query from command attrs (prefer query, then utter, then echo)
		var origQuery string
		if cmd.Attrs != nil {
			if q, ok := cmd.Attrs["query"]; ok {
				origQuery = fmt.Sprintf("%v", q)
			} else if u, ok := cmd.Attrs["utter"]; ok {
				origQuery = fmt.Sprintf("%v", u)
			} else if e, ok := cmd.Attrs["echo"]; ok {
				origQuery = fmt.Sprintf("%v", e)
			}
		}
		// Construct a simple envelope for step results including the original query.
		env := ResponseEnvelope{CorrelationID: corrID, Status: "step_result", Result: map[string]any{"step_id": stepID, "payload": string(payload), "query": origQuery}}
		b, _ := json.Marshal(env)
		// Use the reply topic for step results. Failures here are best-effort
		// and should not abort workflow execution; log on error.
		if werr := producer.WriteMessages(pctx, kafka.Message{Topic: replyTopic, Key: []byte(corrID), Value: b}); werr != nil {
			log.Printf("failed to publish step result (corr_id=%s step=%s): %v", corrID, stepID, werr)
			return werr
		}
		return nil
	}

	result, err := runner.Execute(runCtx, workflow, cmd.Attrs, publishFn)
	if err != nil {
		// Transient errors should bubble up; permanent errors go to DLQ here.
		if isTransientError(err) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return fmt.Errorf("transient execute error (corr_id=%s): %w", corrID, err)
		}

		// Non-transient: publish to DLQ and return nil so offset can be committed.
		env := ResponseEnvelope{CorrelationID: corrID, Status: "error", Error: err.Error()}
		payload, _ := json.Marshal(env)
		dlqTopic := dlqTopicFor(replyTopic)
		if werr := producer.WriteMessages(ctx, kafka.Message{Topic: dlqTopic, Key: []byte(corrID), Value: payload}); werr != nil {
			log.Printf("failed to publish DLQ for non-transient error (corr_id=%s): %v", corrID, werr)
		} else {
			log.Printf("published DLQ for non-transient error (corr_id=%s) to topic=%s", corrID, dlqTopic)
		}
		return nil
	}

	// Success path: publish response.
	resp := ResponseEnvelope{CorrelationID: corrID, Status: "success", Result: result}
	payload, err := json.Marshal(resp)
	if err != nil {
		// If we cannot marshal the response, treat as transient to retry.
		return fmt.Errorf("response marshal failed (corr_id=%s): %w", corrID, err)
	}
	if werr := producer.WriteMessages(ctx, kafka.Message{Topic: replyTopic, Key: []byte(corrID), Value: payload}); werr != nil {
		// Producer failures are transient to allow retry.
		return fmt.Errorf("producer write failed (corr_id=%s): %w", corrID, werr)
	}

	// Store in dedupe to avoid re-processing.
	if err := dedupe.Set(ctx, corrID, string(payload), dedupeTTL); err != nil {
		// Dedupe set failure is transient.
		return fmt.Errorf("dedupe set failed (corr_id=%s): %w", corrID, err)
	}

	log.Printf("processed command successfully (corr_id=%s, workflow=%s)", corrID, workflow)
	return nil
}

func pickReplyTopic(cmdTopic, defaultTopic string) string {
	if t := strings.TrimSpace(cmdTopic); t != "" {
		return t
	}
	return defaultTopic
}

// dlqTopicFor returns a DLQ topic name for a given reply topic. If the
// provided topic already ends with ".dlq", it is returned unchanged. This
// avoids creating topics like "responses.dlq.dlq" when callers provide a
// reply topic that already targets the DLQ.
func dlqTopicFor(replyTopic string) string {
	rt := strings.TrimSpace(replyTopic)
	if rt == "" {
		return ""
	}
	if strings.HasSuffix(rt, ".dlq") {
		return rt
	}
	return rt + ".dlq"
}

// isTransientError performs a simple heuristic on error text for transient cases.
func isTransientError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "timeout") ||
		strings.Contains(s, "temporary") ||
		strings.Contains(s, "temporarily unavailable") ||
		strings.Contains(s, "transient") ||
		strings.Contains(s, "retry") ||
		strings.Contains(s, "too many requests")
}
