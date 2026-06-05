package commandexec

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"manifold/internal/agent/inputrequest"
	"manifold/internal/config"
)

func testBool(v bool) *bool {
	return &v
}

func testExecConfig(rules ...config.ExecCommandRule) config.ExecConfig {
	return config.ExecConfig{
		CommandRules: rules,
		Sandbox: config.ExecSandboxConfig{
			Enabled:           testBool(false),
			FailIfUnavailable: testBool(true),
			Network:           config.ExecSandboxNetworkConfig{Enabled: testBool(false)},
		},
		MaxCommandSeconds: 5,
	}
}

func TestNormalizeCommandArgsPlainWhitespaceOnly(t *testing.T) {
	t.Parallel()

	command, args, err := NormalizeCommandArgs("go test", []string{"./..."})
	if err != nil {
		t.Fatalf("NormalizeCommandArgs error: %v", err)
	}
	if command != "go" || len(args) != 2 || args[0] != "test" || args[1] != "./..." {
		t.Fatalf("unexpected normalized command=%q args=%#v", command, args)
	}

	if _, _, err := NormalizeCommandArgs(`go test -run "Test X"`, nil); err == nil {
		t.Fatal("expected quoted inline command to be rejected")
	}
}

func TestPrepareDeniesUnmatchedCommandsByDefault(t *testing.T) {
	t.Parallel()

	_, err := Prepare(context.Background(), testExecConfig(config.ExecCommandRule{
		ID:       "allow-echo",
		Decision: DecisionAllow,
		Pattern:  []string{"echo"},
	}), PrepareRequest{Command: "go version", Workdir: t.TempDir(), Context: ContextCLI})
	var policyErr *PolicyError
	if !errors.As(err, &policyErr) {
		t.Fatalf("expected policy error, got %v", err)
	}
	if policyErr.Decision != DecisionDeny || policyErr.PolicyID != "" {
		t.Fatalf("unexpected policy error: %#v", policyErr)
	}
}

func TestPrepareMostRestrictiveRuleWins(t *testing.T) {
	t.Parallel()

	_, err := Prepare(context.Background(), testExecConfig(
		config.ExecCommandRule{ID: "allow-echo", Decision: DecisionAllow, Pattern: []string{"echo"}},
		config.ExecCommandRule{ID: "deny-secret", Decision: DecisionDeny, Pattern: []string{"echo", "secret"}},
	), PrepareRequest{Command: "echo secret", Workdir: t.TempDir(), Context: ContextCLI})
	var policyErr *PolicyError
	if !errors.As(err, &policyErr) {
		t.Fatalf("expected policy error, got %v", err)
	}
	if policyErr.PolicyID != "deny-secret" {
		t.Fatalf("policy id = %q, want deny-secret", policyErr.PolicyID)
	}
}

func TestPrepareMatchesRuleContexts(t *testing.T) {
	t.Parallel()

	_, err := Prepare(context.Background(), testExecConfig(config.ExecCommandRule{
		ID:       "terminal-only",
		Decision: DecisionAllow,
		Pattern:  []string{"echo"},
		Contexts: []string{ContextTerminal},
	}), PrepareRequest{Command: "echo hi", Workdir: t.TempDir(), Context: ContextCLI})
	var policyErr *PolicyError
	if !errors.As(err, &policyErr) {
		t.Fatalf("expected policy error, got %v", err)
	}
	if !strings.Contains(policyErr.Error(), "not allowed") {
		t.Fatalf("unexpected error: %v", policyErr)
	}
}

func TestPrepareMigratesBlockBinariesToDenyRules(t *testing.T) {
	t.Parallel()

	cfg := testExecConfig(config.ExecCommandRule{ID: "allow-echo", Decision: DecisionAllow, Pattern: []string{"echo"}})
	cfg.BlockBinaries = []string{"echo"}
	_, err := Prepare(context.Background(), cfg, PrepareRequest{Command: "echo hi", Workdir: t.TempDir(), Context: ContextCLI})
	var policyErr *PolicyError
	if !errors.As(err, &policyErr) {
		t.Fatalf("expected policy error, got %v", err)
	}
	if policyErr.PolicyID != "blockBinaries:echo" {
		t.Fatalf("policy id = %q, want blockBinaries:echo", policyErr.PolicyID)
	}
}

func TestPrepareRejectsPathLikeAndUnknownBinaries(t *testing.T) {
	t.Parallel()

	_, err := Prepare(context.Background(), testExecConfig(), PrepareRequest{Command: "/bin/echo", Workdir: t.TempDir(), Context: ContextCLI})
	var policyErr *PolicyError
	if !errors.As(err, &policyErr) {
		t.Fatalf("expected policy error for path-like binary, got %v", err)
	}

	_, err = Prepare(context.Background(), testExecConfig(config.ExecCommandRule{
		ID:       "allow-missing",
		Decision: DecisionAllow,
		Pattern:  []string{"definitely-not-a-manifold-command"},
	}), PrepareRequest{Command: "definitely-not-a-manifold-command", Workdir: t.TempDir(), Context: ContextCLI})
	if !errors.As(err, &policyErr) || !strings.Contains(policyErr.Error(), "not available") {
		t.Fatalf("expected unknown command policy error, got %v", err)
	}
}

type approvalRequester struct {
	choiceIDs []string
	called    bool
}

func (r *approvalRequester) RequestInfo(ctx context.Context, req inputrequest.Request) (inputrequest.Response, error) {
	r.called = true
	return inputrequest.Response{RequestID: req.ID, ChoiceIDs: r.choiceIDs, RespondedAt: time.Now().UTC()}, nil
}

func TestPrepareAskPromptsOnlyInteractiveContext(t *testing.T) {
	t.Parallel()

	cfg := testExecConfig(config.ExecCommandRule{
		ID:            "ask-echo",
		Decision:      DecisionAsk,
		Pattern:       []string{"echo"},
		Justification: "test approval",
	})
	_, err := Prepare(context.Background(), cfg, PrepareRequest{Command: "echo hi", Workdir: t.TempDir(), Context: ContextCLI})
	var policyErr *PolicyError
	if !errors.As(err, &policyErr) {
		t.Fatalf("expected policy error, got %v", err)
	}
	if !policyErr.RequiresApproval || policyErr.Decision != DecisionAsk {
		t.Fatalf("unexpected noninteractive ask error: %#v", policyErr)
	}

	requester := &approvalRequester{choiceIDs: []string{"approve"}}
	ctx := inputrequest.WithRequester(context.Background(), requester)
	prepared, err := Prepare(ctx, cfg, PrepareRequest{Command: "echo hi", Workdir: t.TempDir(), Context: ContextCLI})
	if err != nil {
		t.Fatalf("interactive Prepare error: %v", err)
	}
	if !requester.called || prepared.Decision != DecisionAsk || prepared.PolicyID != "ask-echo" {
		t.Fatalf("approval not reflected in prepared command: called=%v prepared=%#v", requester.called, prepared)
	}
}
