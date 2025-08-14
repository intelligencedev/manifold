package agent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"
)

// RunWARPP orchestrates a WARPP execution as described in
// examples/documents/warpp_agents_golang.md. It implements the runtime
// protocol: Orchestrator detects intent and fetches full artifacts, launches
// Authenticator and Personalizer in parallel, gates on (authOK ∧ have W*,D*),
// then invokes Fulfillment with the personalized workflow and filtered tools.
//
// The function is generic over the workflow type W and the toolset type T so it
// can be integrated in different domains without imposing a concrete schema
// here. Provide the domain-specific operations via the Plan callbacks.
func RunWARPP[W any, T any](ctx context.Context, utter string, plan Plan[W, T]) error {
	if plan.DetectIntent == nil || plan.FetchFullWorkflowAndTools == nil || plan.Authenticator == nil || plan.RunInfoTools == nil || plan.Trim == nil || plan.Fulfill == nil {
		return errors.New("warpp plan: all callbacks must be provided")
	}

	// Stage 1: intent + full artifacts (W, D)
	intent, err := plan.DetectIntent(ctx, utter)
	if err != nil {
		return fmt.Errorf("detect intent: %w", err)
	}
	fullW, fullD, err := plan.FetchFullWorkflowAndTools(ctx, intent)
	if err != nil {
		return fmt.Errorf("fetch workflow/tools: %w", err)
	}

	// Optional: deadline for personalization/authentication if provided.
	if plan.StageTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, plan.StageTimeout)
		defer cancel()
	}

	// Parallel execution: Authenticator and Personalizer
	g, ctx := errgroup.WithContext(ctx)
	authCh := make(chan AuthResult, 1)
	trimCh := make(chan TrimResult[W, T], 1)

	// 2a) Authenticator
	g.Go(func() error {
		ok, err := plan.Authenticator(ctx)
		select {
		case authCh <- AuthResult{OK: ok, Err: err}:
		default:
		}
		return nil // Don't return errors to prevent context cancellation races
	})

	// 2b) Personalizer (info-tools ⇒ attributes ⇒ TRIM)
	g.Go(func() error {
		attrs, err := plan.RunInfoTools(ctx, fullD)
		if err != nil {
			select {
			case trimCh <- TrimResult[W, T]{Err: err}:
			default:
			}
			return nil // Don't return errors to prevent context cancellation races
		}
		wStar, dStar, err := plan.Trim(ctx, fullW, attrs)
		if err != nil {
			select {
			case trimCh <- TrimResult[W, T]{Err: err}:
			default:
			}
			return nil // Don't return errors to prevent context cancellation races
		}
		select {
		case trimCh <- TrimResult[W, T]{W: wStar, D: dStar}:
		default:
		}
		return nil
	})

	// Wait for both gates with cancellation support.
	var (
		wStar W
		dStar T
	)
	need := 2
	for need > 0 {
		select {
		case ar := <-authCh:
			if ar.Err != nil {
				return fmt.Errorf("authenticator: %w", ar.Err)
			}
			if !ar.OK {
				if plan.ErrAuth != nil {
					return plan.ErrAuth
				}
				return errors.New("authentication failed")
			}
			need--
		case tr := <-trimCh:
			if tr.Err != nil {
				return fmt.Errorf("personalizer: %w", tr.Err)
			}
			wStar, dStar = tr.W, tr.D
			need--
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// Stage 3: Fulfillment (post-gate)
	if err := plan.Fulfill(ctx, wStar, dStar); err != nil {
		return fmt.Errorf("fulfillment: %w", err)
	}
	// Ensure goroutines finished cleanly.
	return g.Wait()
}

// Note: Go does not currently support type parameters on methods. If you wish
// to expose a method on Engine, prefer a thin wrapper that closes over the
// concrete workflow/toolset types for your domain and calls RunWARPP.

// Plan bundles the domain-specific callbacks required to execute a WARPP run.
//
// Types:
//   - W: workflow representation (e.g., a struct with steps/guards/tools)
//   - T: toolset representation (e.g., []Tool or a richer registry view)
type Plan[W any, T any] struct {
	// DetectIntent returns the intent string for the utterance.
	DetectIntent func(ctx context.Context, utter string) (intent string, err error)
	// FetchFullWorkflowAndTools returns the complete workflow and toolset for the intent.
	FetchFullWorkflowAndTools func(ctx context.Context, intent string) (W, T, error)
	// Authenticator performs MFA/identity checks; must return OK for fulfillment to proceed.
	Authenticator func(ctx context.Context) (ok bool, err error)
	// RunInfoTools gathers user attributes using information-gathering tools from the full toolset.
	RunInfoTools func(ctx context.Context, fullTools T) (attrs map[string]any, err error)
	// Trim prunes the full workflow using collected attributes and returns the personalized
	// workflow alongside a filtered toolset limited to those referenced by the trimmed plan.
	Trim func(ctx context.Context, fullW W, attrs map[string]any) (wStar W, dStar T, err error)
	// Fulfill executes the personalized workflow using only the filtered toolset.
	Fulfill func(ctx context.Context, wStar W, dStar T) error

	// Optional error returned when authentication fails (without a concrete error).
	ErrAuth error
	// Optional timeout applied to the parallel stage (auth + personalization).
	StageTimeout time.Duration
}

// AuthResult conveys the outcome of the authenticator stage.
type AuthResult struct {
	OK  bool
	Err error
}

// TrimResult conveys the outcome of the personalization stage.
type TrimResult[W any, T any] struct {
	W   W
	D   T
	Err error
}
