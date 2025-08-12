package agent

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// Test that RunWARPP gates fulfillment on both successful authentication and
// availability of the personalized workflow (W*, D*).
func TestRunWARPP_SuccessOrdersGated(t *testing.T) {
	t.Parallel()

	// Simple domain-specific types for the test
	type Workflow struct{ Steps []string }
	type Toolset []string

	var authDone atomic.Bool
	var trimDone atomic.Bool
	var fulfilled atomic.Bool

	plan := Plan[Workflow, Toolset]{
		DetectIntent: func(ctx context.Context, utter string) (string, error) { return "updateAddress", nil },
		FetchFullWorkflowAndTools: func(ctx context.Context, intent string) (Workflow, Toolset, error) {
			return Workflow{Steps: []string{"s1", "s2", "s3"}}, Toolset{"info:GetProfile", "exec:Submit"}, nil
		},
		Authenticator: func(ctx context.Context) (bool, error) {
			// Simulate work
			time.Sleep(20 * time.Millisecond)
			authDone.Store(true)
			return true, nil
		},
		RunInfoTools: func(ctx context.Context, full Toolset) (map[string]any, error) {
			// Simulate attribute gathering
			time.Sleep(10 * time.Millisecond)
			return map[string]any{"tier": "gold"}, nil
		},
		Trim: func(ctx context.Context, w Workflow, attrs map[string]any) (Workflow, Toolset, error) {
			// Simulate pruning; ensure it's marked done before fulfillment
			trimmed := Workflow{Steps: []string{"s1", "s3"}}
			tools := Toolset{"exec:Submit"}
			trimDone.Store(true)
			return trimmed, tools, nil
		},
		Fulfill: func(ctx context.Context, wStar Workflow, dStar Toolset) error {
			// Both gates must have completed
			if !authDone.Load() {
				t.Fatalf("fulfillment started before authentication completed")
			}
			if !trimDone.Load() {
				t.Fatalf("fulfillment started before trimming completed")
			}
			// Validate inputs carried through
			if len(wStar.Steps) != 2 || len(dStar) != 1 {
				t.Fatalf("unexpected personalized artifacts: %#v / %#v", wStar, dStar)
			}
			fulfilled.Store(true)
			return nil
		},
		ErrAuth:      errors.New("unauthorized"),
		StageTimeout: 2 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := RunWARPP[Workflow, Toolset](ctx, "please update my address", plan); err != nil {
		t.Fatalf("RunWARPP failed: %v", err)
	}
	if !fulfilled.Load() {
		t.Fatalf("fulfillment was not called")
	}
}

// Test that authentication failure prevents fulfillment and returns an error.
func TestRunWARPP_AuthFailure(t *testing.T) {
	t.Parallel()

	type Workflow struct{ Steps []string }
	type Toolset []string

	var fulfilled atomic.Bool
	plan := Plan[Workflow, Toolset]{
		DetectIntent: func(ctx context.Context, utter string) (string, error) { return "x", nil },
		FetchFullWorkflowAndTools: func(ctx context.Context, intent string) (Workflow, Toolset, error) {
			return Workflow{}, nil, nil
		},
		Authenticator: func(ctx context.Context) (bool, error) { return false, nil },
		RunInfoTools:  func(ctx context.Context, _ Toolset) (map[string]any, error) { return map[string]any{}, nil },
		Trim: func(ctx context.Context, w Workflow, attrs map[string]any) (Workflow, Toolset, error) {
			return w, nil, nil
		},
		Fulfill: func(ctx context.Context, wStar Workflow, dStar Toolset) error {
			fulfilled.Store(true)
			return nil
		},
		ErrAuth: errors.New("auth required"),
	}

	err := RunWARPP[Workflow, Toolset](context.Background(), "go", plan)
	if err == nil || err.Error() != "auth required" {
		t.Fatalf("expected auth error, got %v", err)
	}
	if fulfilled.Load() {
		t.Fatalf("fulfillment should not have run on auth failure")
	}
}
