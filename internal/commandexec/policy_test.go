package commandexec

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"manifold/internal/agent/inputrequest"
	"manifold/internal/config"
	"manifold/internal/durable"
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
	answer    string
	request   inputrequest.Request
	called    bool
}

func (r *approvalRequester) RequestInfo(ctx context.Context, req inputrequest.Request) (inputrequest.Response, error) {
	r.called = true
	r.request = req
	return inputrequest.Response{RequestID: req.ID, Answer: r.answer, ChoiceIDs: r.choiceIDs, RespondedAt: time.Now().UTC()}, nil
}

type approvalController struct {
	called bool
	rule   config.ExecCommandRule
}

func (c *approvalController) PersistCommandAllowRule(ctx context.Context, rule config.ExecCommandRule) (config.ExecCommandRule, error) {
	c.called = true
	c.rule = rule
	return rule, nil
}

type suspendingApprovalRequester struct{}

func (r suspendingApprovalRequester) RequestInfo(ctx context.Context, req inputrequest.Request) (inputrequest.Response, error) {
	return inputrequest.Response{}, durable.ErrSuspended
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

	requester := &approvalRequester{choiceIDs: []string{"approve_once"}}
	controller := &approvalController{}
	ctx := inputrequest.WithRequester(context.Background(), requester)
	ctx = WithApprovalController(ctx, controller)
	prepared, err := Prepare(ctx, cfg, PrepareRequest{Command: "echo hi", Workdir: t.TempDir(), Context: ContextCLI})
	if err != nil {
		t.Fatalf("interactive Prepare error: %v", err)
	}
	if !requester.called || prepared.Decision != DecisionAsk || prepared.PolicyID != "ask-echo" {
		t.Fatalf("approval not reflected in prepared command: called=%v prepared=%#v", requester.called, prepared)
	}
	if !requester.request.AllowFreeText || len(requester.request.Choices) != 3 {
		t.Fatalf("approval request did not expose expected frontend controls: %#v", requester.request)
	}
}

func TestPrepareUnmatchedInteractiveCanApproveOnceWithInstructions(t *testing.T) {
	t.Parallel()

	cfg := testExecConfig(config.ExecCommandRule{ID: "allow-go", Decision: DecisionAllow, Pattern: []string{"go"}})
	requester := &approvalRequester{choiceIDs: []string{"approve_once"}, answer: "Run this and then summarize the version."}
	controller := &approvalController{}
	ctx := inputrequest.WithRequester(context.Background(), requester)
	ctx = WithApprovalController(ctx, controller)

	prepared, err := Prepare(ctx, cfg, PrepareRequest{Command: "echo hi", Workdir: t.TempDir(), Context: ContextCLI})
	if err != nil {
		t.Fatalf("Prepare error: %v", err)
	}
	if prepared.PolicyID != "unmatched" || prepared.UserInstructions != requester.answer {
		t.Fatalf("unexpected prepared command: %#v", prepared)
	}
	if controller.called {
		t.Fatalf("approve once should not persist a policy rule")
	}
}

func TestPrepareUnmatchedInteractiveCanPersistAlwaysAllow(t *testing.T) {
	t.Parallel()

	cfg := testExecConfig(config.ExecCommandRule{ID: "allow-go", Decision: DecisionAllow, Pattern: []string{"go"}})
	requester := &approvalRequester{choiceIDs: []string{"always_allow"}}
	controller := &approvalController{}
	ctx := inputrequest.WithRequester(context.Background(), requester)
	ctx = WithApprovalController(ctx, controller)

	prepared, err := Prepare(ctx, cfg, PrepareRequest{Command: "echo hi", Workdir: t.TempDir(), Context: ContextCLI})
	if err != nil {
		t.Fatalf("Prepare error: %v", err)
	}
	if !controller.called {
		t.Fatal("expected always_allow to persist a policy rule")
	}
	if controller.rule.Decision != DecisionAllow || len(controller.rule.Pattern) != 2 || controller.rule.Pattern[0] != "echo" || controller.rule.Pattern[1] != "hi" {
		t.Fatalf("unexpected persisted rule: %#v", controller.rule)
	}
	if len(controller.rule.Contexts) != 1 || controller.rule.Contexts[0] != ContextCLI {
		t.Fatalf("unexpected persisted contexts: %#v", controller.rule.Contexts)
	}
	if prepared.PolicyID != controller.rule.ID {
		t.Fatalf("prepared policy id = %q, want persisted rule id %q", prepared.PolicyID, controller.rule.ID)
	}
}

func TestPrepareApprovalPropagatesDurableSuspension(t *testing.T) {
	t.Parallel()

	cfg := testExecConfig(config.ExecCommandRule{ID: "allow-go", Decision: DecisionAllow, Pattern: []string{"go"}})
	ctx := inputrequest.WithRequester(context.Background(), suspendingApprovalRequester{})
	ctx = WithApprovalController(ctx, &approvalController{})

	_, err := Prepare(ctx, cfg, PrepareRequest{Command: "echo hi", Workdir: t.TempDir(), Context: ContextCLI})
	if !errors.Is(err, durable.ErrSuspended) {
		t.Fatalf("expected durable suspension to propagate, got %v", err)
	}
}
