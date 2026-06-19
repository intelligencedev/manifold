package commandexec

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
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

func TestPrepareAllowedDomainsRequiresSandbox(t *testing.T) {
	t.Parallel()

	cfg := testExecConfig(config.ExecCommandRule{ID: "allow-echo", Decision: DecisionAllow, Pattern: []string{"echo"}})
	cfg.Sandbox.Enabled = testBool(false)
	cfg.Sandbox.Network.Enabled = testBool(true)
	cfg.Sandbox.Network.AllowedDomains = []string{"example.com"}
	_, err := Prepare(context.Background(), cfg, PrepareRequest{Command: "echo hi", Workdir: t.TempDir(), Context: ContextCLI})
	var policyErr *PolicyError
	if !errors.As(err, &policyErr) {
		t.Fatalf("expected policy error, got %v", err)
	}
	if !strings.Contains(policyErr.Error(), "allowedDomains requires") {
		t.Fatalf("unexpected policy error: %v", policyErr)
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
	called           bool
	rule             config.ExecCommandRule
	sessionAllowAll  map[string]bool
	sessionSetScope  CommandSessionScope
	sessionSetCalled bool
}

func (c *approvalController) PersistCommandAllowRule(ctx context.Context, rule config.ExecCommandRule) (config.ExecCommandRule, error) {
	c.called = true
	c.rule = rule
	return rule, nil
}

func (c *approvalController) SessionAllowAllCommands(ctx context.Context, scope CommandSessionScope) (bool, error) {
	return c.sessionAllowAll[sessionScopeTestKey(scope)], nil
}

func (c *approvalController) SetSessionAllowAllCommands(ctx context.Context, scope CommandSessionScope, allow bool) error {
	c.sessionSetCalled = true
	c.sessionSetScope = scope
	if c.sessionAllowAll == nil {
		c.sessionAllowAll = map[string]bool{}
	}
	c.sessionAllowAll[sessionScopeTestKey(scope)] = allow
	return nil
}

func sessionScopeTestKey(scope CommandSessionScope) string {
	return strconv.FormatInt(scope.UserID, 10) + "\x00" + scope.SessionID
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

func TestPrepareSessionScopedApprovalChoice(t *testing.T) {
	t.Parallel()

	cfg := testExecConfig(config.ExecCommandRule{ID: "allow-go", Decision: DecisionAllow, Pattern: []string{"go"}})
	requester := &approvalRequester{choiceIDs: []string{"approve_once"}}
	controller := &approvalController{}
	ctx := inputrequest.WithRequester(context.Background(), requester)
	ctx = WithApprovalController(ctx, controller)
	ctx = WithCommandSessionScope(ctx, CommandSessionScope{UserID: 7, SessionID: "session-a"})

	if _, err := Prepare(ctx, cfg, PrepareRequest{Command: "echo hi", Workdir: t.TempDir(), Context: ContextCLI}); err != nil {
		t.Fatalf("Prepare error: %v", err)
	}
	if len(requester.request.Choices) != 4 {
		t.Fatalf("expected session-scoped approval choice, got %#v", requester.request.Choices)
	}
	var found bool
	for _, choice := range requester.request.Choices {
		if choice.ID == "allow_all_session" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("allow_all_session choice missing: %#v", requester.request.Choices)
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

func TestPrepareUnmatchedInteractiveCanAllowAllForSession(t *testing.T) {
	t.Parallel()

	cfg := testExecConfig(config.ExecCommandRule{ID: "allow-go", Decision: DecisionAllow, Pattern: []string{"go"}})
	requester := &approvalRequester{choiceIDs: []string{"allow_all_session"}}
	controller := &approvalController{}
	scope := CommandSessionScope{UserID: 7, SessionID: "session-a"}
	ctx := inputrequest.WithRequester(context.Background(), requester)
	ctx = WithApprovalController(ctx, controller)
	ctx = WithCommandSessionScope(ctx, scope)

	prepared, err := Prepare(ctx, cfg, PrepareRequest{Command: "echo hi", Workdir: t.TempDir(), Context: ContextCLI})
	if err != nil {
		t.Fatalf("Prepare error: %v", err)
	}
	if !controller.sessionSetCalled || controller.sessionSetScope != scope {
		t.Fatalf("expected session allow-all to persist, called=%v scope=%#v", controller.sessionSetCalled, controller.sessionSetScope)
	}
	if prepared.PolicyID != "session:session-a:allow-all" {
		t.Fatalf("policy id = %q, want session allow-all", prepared.PolicyID)
	}
}

func TestPrepareSessionAllowAllSkipsPromptsOnlyForSameSession(t *testing.T) {
	t.Parallel()

	cfg := testExecConfig(config.ExecCommandRule{ID: "allow-go", Decision: DecisionAllow, Pattern: []string{"go"}})
	controller := &approvalController{sessionAllowAll: map[string]bool{
		sessionScopeTestKey(CommandSessionScope{UserID: 7, SessionID: "session-a"}): true,
	}}
	ctx := WithApprovalController(context.Background(), controller)
	ctx = WithCommandSessionScope(ctx, CommandSessionScope{UserID: 7, SessionID: "session-a"})
	prepared, err := Prepare(ctx, cfg, PrepareRequest{Command: "echo hi", Workdir: t.TempDir(), Context: ContextCLI})
	if err != nil {
		t.Fatalf("Prepare same session error: %v", err)
	}
	if prepared.PolicyID != "session:session-a:allow-all" || prepared.Decision != DecisionAllow {
		t.Fatalf("unexpected same-session prepared command: %#v", prepared)
	}

	otherCtx := WithApprovalController(context.Background(), controller)
	otherCtx = WithCommandSessionScope(otherCtx, CommandSessionScope{UserID: 7, SessionID: "session-b"})
	_, err = Prepare(otherCtx, cfg, PrepareRequest{Command: "echo hi", Workdir: t.TempDir(), Context: ContextCLI})
	var policyErr *PolicyError
	if !errors.As(err, &policyErr) || policyErr.Decision != DecisionAsk {
		t.Fatalf("expected different session to require approval, got %v", err)
	}
}

func TestPrepareSessionAllowAllDoesNotBypassDenyOrAvailability(t *testing.T) {
	t.Parallel()

	scope := CommandSessionScope{UserID: 7, SessionID: "session-a"}
	controller := &approvalController{sessionAllowAll: map[string]bool{sessionScopeTestKey(scope): true}}
	ctx := WithApprovalController(context.Background(), controller)
	ctx = WithCommandSessionScope(ctx, scope)

	_, err := Prepare(ctx, testExecConfig(config.ExecCommandRule{ID: "deny-echo", Decision: DecisionDeny, Pattern: []string{"echo"}}), PrepareRequest{Command: "echo hi", Workdir: t.TempDir(), Context: ContextCLI})
	var policyErr *PolicyError
	if !errors.As(err, &policyErr) || policyErr.PolicyID != "deny-echo" {
		t.Fatalf("expected explicit deny to win, got %v", err)
	}

	_, err = Prepare(ctx, testExecConfig(config.ExecCommandRule{ID: "allow-go", Decision: DecisionAllow, Pattern: []string{"go"}}), PrepareRequest{Command: "definitely-not-a-manifold-command", Workdir: t.TempDir(), Context: ContextCLI})
	if !errors.As(err, &policyErr) || !strings.Contains(policyErr.Error(), "not available") {
		t.Fatalf("expected unavailable binary to fail, got %v", err)
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

func TestSecurePathIncludesResolvedGoBin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("securePath uses ambient PATH on Windows")
	}
	binDir := t.TempDir()
	goPath := filepath.Join(binDir, "go")
	if err := os.WriteFile(goPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write fake go binary: %v", err)
	}
	t.Setenv("PATH", binDir)
	want := binDir
	if resolved, err := filepath.EvalSymlinks(binDir); err == nil {
		want = resolved
	}

	got := filepath.SplitList(securePath())
	for _, dir := range got {
		if dir == want {
			return
		}
	}
	t.Fatalf("securePath() = %#v, want %q", got, want)
}
