package commandexec

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"manifold/internal/agent/inputrequest"
	"manifold/internal/config"
	"manifold/internal/durable"
	"manifold/internal/sandbox"
)

const (
	DecisionAllow = "allow"
	DecisionAsk   = "ask"
	DecisionDeny  = "deny"

	ContextCLI         = "cli"
	ContextTerminal    = "terminal"
	ContextRunCLI      = "run_cli"
	ContextTerminalRun = "terminal_start"
	ContextChat        = "chat"
	ContextInteractive = "interactive"
	ContextBackground  = "background"
)

type PolicyError struct {
	Message          string
	Decision         string
	PolicyID         string
	RequiresApproval bool
	UserInstructions string
}

func (e *PolicyError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

type PreparedCommand struct {
	Command          string
	Args             []string
	ExecCommand      string
	ExecArgs         []string
	Dir              string
	Env              []string
	Cleanup          func()
	Decision         string
	PolicyID         string
	Sandboxed        bool
	UserInstructions string
}

type PrepareRequest struct {
	Command string
	Args    []string
	Workdir string
	Context string
}

type preparedLaunch struct {
	execCommand string
	execArgs    []string
	env         []string
	cleanup     func()
	sandboxed   bool
}

type evaluation struct {
	decision      string
	policyID      string
	justification string
}

type approvalResult struct {
	userInstructions string
	policyID         string
}

type CommandSessionScope struct {
	UserID    int64
	SessionID string
}

type ApprovalController interface {
	PersistCommandAllowRule(ctx context.Context, rule config.ExecCommandRule) (config.ExecCommandRule, error)
	SessionAllowAllCommands(ctx context.Context, scope CommandSessionScope) (bool, error)
	SetSessionAllowAllCommands(ctx context.Context, scope CommandSessionScope, allow bool) error
}

type approvalControllerContextKey struct{}
type commandSessionScopeContextKey struct{}

func WithApprovalController(ctx context.Context, controller ApprovalController) context.Context {
	if ctx == nil || controller == nil {
		return ctx
	}
	return context.WithValue(ctx, approvalControllerContextKey{}, controller)
}

func ApprovalControllerFromContext(ctx context.Context) ApprovalController {
	if ctx == nil {
		return nil
	}
	controller, _ := ctx.Value(approvalControllerContextKey{}).(ApprovalController)
	return controller
}

func WithCommandSessionScope(ctx context.Context, scope CommandSessionScope) context.Context {
	scope.SessionID = strings.TrimSpace(scope.SessionID)
	if ctx == nil {
		ctx = context.Background()
	}
	if scope.SessionID == "" {
		return ctx
	}
	return context.WithValue(ctx, commandSessionScopeContextKey{}, scope)
}

func CommandSessionScopeFromContext(ctx context.Context) (CommandSessionScope, bool) {
	if ctx == nil {
		return CommandSessionScope{}, false
	}
	if scope, ok := ctx.Value(commandSessionScopeContextKey{}).(CommandSessionScope); ok && strings.TrimSpace(scope.SessionID) != "" {
		scope.SessionID = strings.TrimSpace(scope.SessionID)
		return scope, true
	}
	sessionID, ok := sandbox.SessionIDFromContext(ctx)
	if !ok {
		return CommandSessionScope{}, false
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return CommandSessionScope{}, false
	}
	return CommandSessionScope{SessionID: sessionID}, true
}

func Prepare(ctx context.Context, cfg config.ExecConfig, req PrepareRequest) (PreparedCommand, error) {
	config.ApplyExecDefaults(&cfg)
	command, safeArgs, base, err := normalizePrepareRequest(ctx, req)
	if err != nil {
		return PreparedCommand{}, err
	}

	argv := append([]string{command}, safeArgs...)
	result, approval, err := evaluateAndApprove(ctx, cfg, req.Context, argv)
	if err != nil {
		return PreparedCommand{}, err
	}

	launch, err := prepareLaunch(cfg, base, command, safeArgs, result.policyID)
	if err != nil {
		return PreparedCommand{}, err
	}
	return PreparedCommand{
		Command:          command,
		Args:             safeArgs,
		ExecCommand:      launch.execCommand,
		ExecArgs:         launch.execArgs,
		Dir:              base,
		Env:              launch.env,
		Cleanup:          launch.cleanup,
		Decision:         result.decision,
		PolicyID:         firstNonEmpty(approval.policyID, result.policyID),
		Sandboxed:        launch.sandboxed,
		UserInstructions: approval.userInstructions,
	}, nil
}

func normalizePrepareRequest(ctx context.Context, req PrepareRequest) (string, []string, string, error) {
	command, args, err := NormalizeCommandArgs(req.Command, req.Args)
	if err != nil {
		return "", nil, "", err
	}
	if command == "" {
		return "", nil, "", errors.New("command is required")
	}
	if err := validateBinaryName(command); err != nil {
		return "", nil, "", policyDeny(err.Error(), "", false)
	}

	base := req.Workdir
	if strings.TrimSpace(base) == "" {
		base = sandbox.ResolveBaseDir(ctx, "")
	}
	if strings.TrimSpace(base) == "" {
		return "", nil, "", errors.New("workdir is required")
	}
	base, err = filepath.Abs(base)
	if err != nil {
		return "", nil, "", fmt.Errorf("resolve workdir: %w", err)
	}

	safeArgs := make([]string, 0, len(args))
	for _, arg := range args {
		safeArg, err := sandbox.SanitizeArg(base, arg)
		if err != nil {
			return "", nil, "", err
		}
		safeArgs = append(safeArgs, safeArg)
	}
	return command, safeArgs, base, nil
}

func evaluateAndApprove(ctx context.Context, cfg config.ExecConfig, execContext string, argv []string) (evaluation, approvalResult, error) {
	result, err := evaluatePolicy(ctx, cfg, execContext, argv)
	if err != nil {
		return evaluation{}, approvalResult{}, err
	}
	approval := approvalResult{policyID: result.policyID}
	if result.decision == DecisionAsk {
		if sessionResult, ok := sessionAllowAllEvaluation(ctx); ok {
			return sessionResult, approvalResult{policyID: sessionResult.policyID}, nil
		}
		approval, err = requestApproval(ctx, result, argv, execContext)
		if err != nil {
			return evaluation{}, approvalResult{}, err
		}
	}
	return result, approval, nil
}

func prepareLaunch(cfg config.ExecConfig, base, command string, safeArgs []string, policyID string) (preparedLaunch, error) {
	if err := ensureRuntimeDirs(base); err != nil {
		return preparedLaunch{}, err
	}
	env := secureEnv(base)
	network, env, err := prepareSandboxNetwork(cfg, base, env)
	if err != nil {
		return preparedLaunch{}, err
	}
	success := false
	defer func() {
		if !success {
			runCleanup(network.cleanup)
		}
	}()
	resolved, err := lookPath(command, pathFromEnv(env))
	if err != nil {
		return preparedLaunch{}, policyDeny(fmt.Sprintf("command is not available: %q", command), policyID, false)
	}

	execCommand, execArgs, sandboxed, err := wrapSandbox(cfg, base, resolved, safeArgs, env, &network)
	if err != nil {
		var policyErr *PolicyError
		if errors.As(err, &policyErr) {
			return preparedLaunch{}, policyErr
		}
		return preparedLaunch{}, err
	}
	success = true
	return preparedLaunch{
		execCommand: execCommand,
		execArgs:    execArgs,
		env:         env,
		cleanup:     network.cleanup,
		sandboxed:   sandboxed,
	}, nil
}

func NormalizeCommandArgs(command string, args []string) (string, []string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", args, nil
	}
	if hasInlineShellSyntax(command) {
		return "", nil, errors.New("inline command must be plain whitespace-separated argv; pass complex arguments in args")
	}
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return "", args, nil
	}
	if len(parts) == 1 {
		return parts[0], append([]string(nil), args...), nil
	}
	merged := make([]string, 0, len(parts)-1+len(args))
	merged = append(merged, parts[1:]...)
	merged = append(merged, args...)
	return parts[0], merged, nil
}

func hasInlineShellSyntax(command string) bool {
	return strings.ContainsAny(command, "'\"`$;&|<>(){}[]*?\n\r")
}

func validateBinaryName(command string) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return errors.New("command is required")
	}
	if command == "." || command == ".." || strings.HasPrefix(command, "-") {
		return fmt.Errorf("binary is blocked or invalid: %q", command)
	}
	if strings.Contains(command, "/") || strings.Contains(command, "\\") {
		return fmt.Errorf("binary is blocked or invalid: %q", command)
	}
	if filepath.IsAbs(command) {
		return fmt.Errorf("binary is blocked or invalid: %q", command)
	}
	return nil
}

func evaluatePolicy(ctx context.Context, cfg config.ExecConfig, execContext string, argv []string) (evaluation, error) {
	rules := effectiveRules(cfg)
	activeContexts := activeContextSet(ctx, execContext)
	var best evaluation
	bestRank := 0
	for i, rule := range rules {
		normalized, err := normalizeRule(rule, i)
		if err != nil {
			return evaluation{}, policyDeny(err.Error(), ruleID(rule, i), false)
		}
		if !contextsMatch(normalized.Contexts, activeContexts) {
			continue
		}
		if !prefixMatch(argv, normalized.Pattern) {
			continue
		}
		rank := decisionRank(normalized.Decision)
		if rank > bestRank {
			bestRank = rank
			best = evaluation{
				decision:      normalized.Decision,
				policyID:      ruleID(normalized, i),
				justification: normalized.Justification,
			}
		}
	}
	if bestRank == 0 {
		if canPromptCommandApproval(ctx) || canUseSessionCommandPolicy(ctx) {
			return evaluation{
				decision:      DecisionAsk,
				policyID:      "unmatched",
				justification: "No command policy rule matched this command.",
			}, nil
		}
		return evaluation{}, policyDeny(fmt.Sprintf("command is not allowed by policy: %s", strings.Join(argv, " ")), "", false)
	}
	if best.decision == DecisionDeny {
		return evaluation{}, policyDeny(fmt.Sprintf("command is denied by policy: %s", strings.Join(argv, " ")), best.policyID, false)
	}
	return best, nil
}

func effectiveRules(cfg config.ExecConfig) []config.ExecCommandRule {
	rules := append([]config.ExecCommandRule(nil), cfg.CommandRules...)
	for _, binary := range cfg.BlockBinaries {
		binary = strings.TrimSpace(binary)
		if binary == "" {
			continue
		}
		rules = append(rules, config.ExecCommandRule{
			ID:            "blockBinaries:" + binary,
			Decision:      DecisionDeny,
			Pattern:       []string{binary},
			Justification: "legacy exec.blockBinaries migration deny rule",
		})
	}
	return rules
}

func normalizeRule(rule config.ExecCommandRule, index int) (config.ExecCommandRule, error) {
	rule.Decision = strings.ToLower(strings.TrimSpace(rule.Decision))
	switch rule.Decision {
	case DecisionAllow, DecisionAsk, DecisionDeny:
	default:
		return rule, fmt.Errorf("invalid command policy decision for %s: %q", ruleID(rule, index), rule.Decision)
	}
	if len(rule.Pattern) == 0 {
		return rule, fmt.Errorf("command policy rule %s has empty pattern", ruleID(rule, index))
	}
	for i := range rule.Pattern {
		rule.Pattern[i] = strings.TrimSpace(rule.Pattern[i])
		if rule.Pattern[i] == "" {
			return rule, fmt.Errorf("command policy rule %s has an empty pattern token", ruleID(rule, index))
		}
	}
	if err := validateBinaryName(rule.Pattern[0]); err != nil {
		return rule, fmt.Errorf("command policy rule %s has invalid binary: %w", ruleID(rule, index), err)
	}
	for i := range rule.Contexts {
		rule.Contexts[i] = strings.ToLower(strings.TrimSpace(rule.Contexts[i]))
	}
	return rule, nil
}

func ruleID(rule config.ExecCommandRule, index int) string {
	if strings.TrimSpace(rule.ID) != "" {
		return strings.TrimSpace(rule.ID)
	}
	return fmt.Sprintf("exec.commandRules[%d]", index)
}

func prefixMatch(argv, pattern []string) bool {
	if len(pattern) == 0 || len(pattern) > len(argv) {
		return false
	}
	for i := range pattern {
		if argv[i] != pattern[i] {
			return false
		}
	}
	return true
}

func contextsMatch(ruleContexts []string, active map[string]struct{}) bool {
	if len(ruleContexts) == 0 {
		return true
	}
	for _, contextName := range ruleContexts {
		contextName = strings.ToLower(strings.TrimSpace(contextName))
		if contextName == "" {
			continue
		}
		if _, ok := active[contextName]; ok {
			return true
		}
	}
	return false
}

func activeContextSet(ctx context.Context, execContext string) map[string]struct{} {
	out := make(map[string]struct{}, 5)
	execContext = strings.ToLower(strings.TrimSpace(execContext))
	if execContext != "" {
		out[execContext] = struct{}{}
	}
	switch execContext {
	case ContextCLI:
		out[ContextRunCLI] = struct{}{}
	case ContextTerminal:
		out[ContextTerminalRun] = struct{}{}
	}
	if inputrequest.RequesterFromContext(ctx) != nil {
		out[ContextChat] = struct{}{}
		out[ContextInteractive] = struct{}{}
	} else {
		out[ContextBackground] = struct{}{}
	}
	return out
}

func decisionRank(decision string) int {
	switch decision {
	case DecisionDeny:
		return 3
	case DecisionAsk:
		return 2
	case DecisionAllow:
		return 1
	default:
		return 0
	}
}

func canPromptCommandApproval(ctx context.Context) bool {
	return inputrequest.RequesterFromContext(ctx) != nil && ApprovalControllerFromContext(ctx) != nil
}

func canUseSessionCommandPolicy(ctx context.Context) bool {
	if ApprovalControllerFromContext(ctx) == nil {
		return false
	}
	_, ok := CommandSessionScopeFromContext(ctx)
	return ok
}

func sessionAllowAllEvaluation(ctx context.Context) (evaluation, bool) {
	controller := ApprovalControllerFromContext(ctx)
	if controller == nil {
		return evaluation{}, false
	}
	scope, ok := CommandSessionScopeFromContext(ctx)
	if !ok {
		return evaluation{}, false
	}
	allow, err := controller.SessionAllowAllCommands(ctx, scope)
	if err != nil || !allow {
		return evaluation{}, false
	}
	return evaluation{
		decision:      DecisionAllow,
		policyID:      sessionAllowAllPolicyID(scope),
		justification: "Session command policy allows all commands.",
	}, true
}

func sessionAllowAllPolicyID(scope CommandSessionScope) string {
	return "session:" + strings.TrimSpace(scope.SessionID) + ":allow-all"
}

func requestApproval(ctx context.Context, result evaluation, argv []string, execContext string) (approvalResult, error) {
	requester := inputrequest.RequesterFromContext(ctx)
	controller := ApprovalControllerFromContext(ctx)
	if requester == nil || controller == nil {
		return approvalResult{}, commandApprovalUnavailable(result, argv)
	}
	req := commandApprovalRequest(ctx, result, argv)
	resp, err := requester.RequestInfo(ctx, req)
	if err != nil {
		if errors.Is(err, durable.ErrSuspended) {
			return approvalResult{}, err
		}
		return approvalResult{}, commandApprovalPolicyError(result, fmt.Sprintf("command approval failed: %v", err), "")
	}
	userInstructions := strings.TrimSpace(resp.Answer)
	if approvalResponseHasChoice(resp, "deny") {
		return approvalResult{}, commandApprovalDenied(result, argv, userInstructions)
	}
	for _, choice := range resp.ChoiceIDs {
		approval, handled, err := handleCommandApprovalChoice(ctx, controller, result, argv, execContext, userInstructions, choice)
		if handled || err != nil {
			return approval, err
		}
	}
	if strings.EqualFold(strings.TrimSpace(resp.Answer), "approve") {
		return approvalResult{userInstructions: userInstructions, policyID: result.policyID}, nil
	}
	return approvalResult{}, commandApprovalDenied(result, argv, userInstructions)
}

func commandApprovalUnavailable(result evaluation, argv []string) *PolicyError {
	return commandApprovalPolicyError(
		result,
		fmt.Sprintf("command requires approval and cannot prompt in this context: %s", strings.Join(argv, " ")),
		"",
	)
}

func commandApprovalRequest(ctx context.Context, result evaluation, argv []string) inputrequest.Request {
	meta := inputrequest.RunMetadataFromContext(ctx)
	return inputrequest.Request{
		ID:            fmt.Sprintf("command-approval-%d", time.Now().UnixNano()),
		Question:      fmt.Sprintf("Approve command execution: %s", strings.Join(argv, " ")),
		Reason:        strings.TrimSpace(result.justification),
		Choices:       commandApprovalChoices(ctx),
		AllowFreeText: true,
		Multiple:      false,
		Agent:         meta.Agent,
		Model:         meta.Model,
		CallID:        meta.CallID,
		ParentCallID:  meta.ParentCallID,
		ToolID:        meta.ToolID,
		Depth:         meta.Depth,
		CreatedAt:     time.Now().UTC(),
	}
}

func approvalResponseHasChoice(resp inputrequest.Response, id string) bool {
	for _, choice := range resp.ChoiceIDs {
		if strings.EqualFold(strings.TrimSpace(choice), id) {
			return true
		}
	}
	return false
}

func handleCommandApprovalChoice(
	ctx context.Context,
	controller ApprovalController,
	result evaluation,
	argv []string,
	execContext string,
	userInstructions string,
	choice string,
) (approvalResult, bool, error) {
	switch strings.ToLower(strings.TrimSpace(choice)) {
	case "approve_once", "approve":
		return approvalResult{userInstructions: userInstructions, policyID: result.policyID}, true, nil
	case "always_allow":
		approval, err := persistCommandAllowApproval(ctx, controller, result, argv, execContext, userInstructions)
		return approval, true, err
	case "allow_all_session":
		approval, err := persistSessionAllowAllApproval(ctx, controller, result, userInstructions)
		return approval, true, err
	default:
		return approvalResult{}, false, nil
	}
}

func persistCommandAllowApproval(
	ctx context.Context,
	controller ApprovalController,
	result evaluation,
	argv []string,
	execContext string,
	userInstructions string,
) (approvalResult, error) {
	rule := commandApprovalAllowRule(execContext, argv)
	persisted, err := controller.PersistCommandAllowRule(ctx, rule)
	if err != nil {
		return approvalResult{}, commandApprovalPolicyError(result, fmt.Sprintf("persist command allow rule: %v", err), userInstructions)
	}
	return approvalResult{
		userInstructions: userInstructions,
		policyID:         firstNonEmpty(strings.TrimSpace(persisted.ID), result.policyID),
	}, nil
}

func persistSessionAllowAllApproval(
	ctx context.Context,
	controller ApprovalController,
	result evaluation,
	userInstructions string,
) (approvalResult, error) {
	scope, ok := CommandSessionScopeFromContext(ctx)
	if !ok {
		return approvalResult{}, commandApprovalPolicyError(result, "persist session command policy: session scope is unavailable", userInstructions)
	}
	if err := controller.SetSessionAllowAllCommands(ctx, scope, true); err != nil {
		return approvalResult{}, commandApprovalPolicyError(result, fmt.Sprintf("persist session command policy: %v", err), userInstructions)
	}
	return approvalResult{userInstructions: userInstructions, policyID: sessionAllowAllPolicyID(scope)}, nil
}

func commandApprovalChoices(ctx context.Context) []inputrequest.Choice {
	choices := []inputrequest.Choice{
		{ID: "approve_once", Label: "Approve once", Description: "Run this command one time without changing policy."},
		{ID: "always_allow", Label: "Always allow", Description: "Add this exact argv prefix to persistent command policy."},
	}
	if _, ok := CommandSessionScopeFromContext(ctx); ok {
		choices = append(choices, inputrequest.Choice{
			ID:          "allow_all_session",
			Label:       "Allow all in this session",
			Description: "Skip command prompts for this chat session. Explicit denies and sandbox rules still apply.",
		})
	}
	return append(choices, inputrequest.Choice{ID: "deny", Label: "Deny", Description: "Do not run this command."})
}

func commandApprovalDenied(result evaluation, argv []string, userInstructions string) *PolicyError {
	return &PolicyError{
		Message:          fmt.Sprintf("command approval denied: %s", strings.Join(argv, " ")),
		Decision:         DecisionDeny,
		PolicyID:         result.policyID,
		RequiresApproval: false,
		UserInstructions: userInstructions,
	}
}

func commandApprovalPolicyError(result evaluation, message string, userInstructions string) *PolicyError {
	return &PolicyError{
		Message:          message,
		Decision:         DecisionAsk,
		PolicyID:         result.policyID,
		RequiresApproval: true,
		UserInstructions: userInstructions,
	}
}

func policyDeny(message, policyID string, requiresApproval bool) *PolicyError {
	return &PolicyError{Message: message, Decision: DecisionDeny, PolicyID: policyID, RequiresApproval: requiresApproval}
}

func commandApprovalAllowRule(execContext string, argv []string) config.ExecCommandRule {
	contextName := strings.ToLower(strings.TrimSpace(execContext))
	pattern := append([]string(nil), argv...)
	h := sha256.Sum256([]byte(contextName + "\x00" + strings.Join(pattern, "\x00")))
	contexts := []string(nil)
	if contextName != "" {
		contexts = []string{contextName}
	}
	return config.ExecCommandRule{
		ID:            "approved:" + firstNonEmpty(contextName, "command") + ":" + hex.EncodeToString(h[:])[:12],
		Decision:      DecisionAllow,
		Pattern:       pattern,
		Contexts:      contexts,
		Justification: "Approved from chat command approval prompt.",
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func secureEnv(workdir string) []string {
	path := securePath()
	tmpDir := filepath.Join(workdir, ".tmp")
	cacheDir := filepath.Join(workdir, ".cache")
	env := []string{
		"PATH=" + path,
		"HOME=" + workdir,
		"TMPDIR=" + tmpDir,
		"TEMP=" + tmpDir,
		"TMP=" + tmpDir,
		"GOTOOLCHAIN=local",
		"GO111MODULE=on",
		"GOTELEMETRY=off",
		"GOCACHE=" + filepath.Join(cacheDir, "go-build"),
		"GOMODCACHE=" + filepath.Join(cacheDir, "go-mod"),
		"TERM=xterm-256color",
	}
	for _, key := range []string{"LANG", "LC_ALL", "LC_CTYPE"} {
		if value := os.Getenv(key); strings.TrimSpace(value) != "" {
			env = append(env, key+"="+value)
		}
	}
	return env
}

func ensureRuntimeDirs(workdir string) error {
	for _, rel := range []string{".tmp", ".cache"} {
		if err := os.MkdirAll(filepath.Join(workdir, rel), 0o700); err != nil {
			return fmt.Errorf("create command runtime dir %s: %w", rel, err)
		}
	}
	return nil
}

func pathFromEnv(env []string) string {
	for _, item := range env {
		if strings.HasPrefix(item, "PATH=") {
			return strings.TrimPrefix(item, "PATH=")
		}
	}
	return securePath()
}

func securePath() string {
	if runtime.GOOS == "windows" {
		return os.Getenv("PATH")
	}
	paths := []string{"/usr/local/go/bin", "/opt/homebrew/bin", "/usr/local/bin", "/usr/bin", "/bin", "/usr/sbin", "/sbin"}
	return strings.Join(paths, string(os.PathListSeparator))
}

func lookPath(command, pathValue string) (string, error) {
	if runtime.GOOS == "windows" {
		return exec.LookPath(command)
	}
	for _, dir := range filepath.SplitList(pathValue) {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, command)
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		if info.Mode()&0111 != 0 {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%s not found on secure PATH", command)
}
