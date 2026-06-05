package commandexec

import (
	"context"
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
}

func (e *PolicyError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

type PreparedCommand struct {
	Command     string
	Args        []string
	ExecCommand string
	ExecArgs    []string
	Dir         string
	Env         []string
	Decision    string
	PolicyID    string
	Sandboxed   bool
}

type PrepareRequest struct {
	Command string
	Args    []string
	Workdir string
	Context string
}

type evaluation struct {
	decision      string
	policyID      string
	justification string
}

func Prepare(ctx context.Context, cfg config.ExecConfig, req PrepareRequest) (PreparedCommand, error) {
	config.ApplyExecDefaults(&cfg)
	command, args, err := NormalizeCommandArgs(req.Command, req.Args)
	if err != nil {
		return PreparedCommand{}, err
	}
	if command == "" {
		return PreparedCommand{}, errors.New("command is required")
	}
	if err := validateBinaryName(command); err != nil {
		return PreparedCommand{}, policyDeny(err.Error(), "", false)
	}

	base := req.Workdir
	if strings.TrimSpace(base) == "" {
		base = sandbox.ResolveBaseDir(ctx, "")
	}
	if strings.TrimSpace(base) == "" {
		return PreparedCommand{}, errors.New("workdir is required")
	}
	base, err = filepath.Abs(base)
	if err != nil {
		return PreparedCommand{}, fmt.Errorf("resolve workdir: %w", err)
	}

	safeArgs := make([]string, 0, len(args))
	for _, arg := range args {
		safeArg, err := sandbox.SanitizeArg(base, arg)
		if err != nil {
			return PreparedCommand{}, err
		}
		safeArgs = append(safeArgs, safeArg)
	}

	argv := append([]string{command}, safeArgs...)
	result, err := evaluatePolicy(ctx, cfg, req.Context, argv)
	if err != nil {
		return PreparedCommand{}, err
	}
	if result.decision == DecisionAsk {
		if err := requestApproval(ctx, result, argv); err != nil {
			return PreparedCommand{}, err
		}
	}

	if err := ensureRuntimeDirs(base); err != nil {
		return PreparedCommand{}, err
	}
	env := secureEnv(base)
	resolved, err := lookPath(command, pathFromEnv(env))
	if err != nil {
		return PreparedCommand{}, policyDeny(fmt.Sprintf("command is not available: %q", command), result.policyID, false)
	}

	execCommand, execArgs, sandboxed, err := wrapSandbox(cfg, base, resolved, safeArgs, env)
	if err != nil {
		var policyErr *PolicyError
		if errors.As(err, &policyErr) {
			return PreparedCommand{}, policyErr
		}
		return PreparedCommand{}, err
	}
	return PreparedCommand{
		Command:     command,
		Args:        safeArgs,
		ExecCommand: execCommand,
		ExecArgs:    execArgs,
		Dir:         base,
		Env:         env,
		Decision:    result.decision,
		PolicyID:    result.policyID,
		Sandboxed:   sandboxed,
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

func requestApproval(ctx context.Context, result evaluation, argv []string) error {
	requester := inputrequest.RequesterFromContext(ctx)
	if requester == nil {
		return &PolicyError{
			Message:          fmt.Sprintf("command requires approval and cannot prompt in this context: %s", strings.Join(argv, " ")),
			Decision:         DecisionAsk,
			PolicyID:         result.policyID,
			RequiresApproval: true,
		}
	}
	meta := inputrequest.RunMetadataFromContext(ctx)
	req := inputrequest.Request{
		ID:       fmt.Sprintf("command-approval-%d", time.Now().UnixNano()),
		Question: fmt.Sprintf("Approve one-time command execution: %s", strings.Join(argv, " ")),
		Reason:   strings.TrimSpace(result.justification),
		Choices: []inputrequest.Choice{
			{ID: "approve", Label: "Approve once", Description: "Run this command one time without changing policy."},
			{ID: "deny", Label: "Deny", Description: "Do not run this command."},
		},
		AllowFreeText: false,
		Multiple:      false,
		Agent:         meta.Agent,
		Model:         meta.Model,
		CallID:        meta.CallID,
		ParentCallID:  meta.ParentCallID,
		ToolID:        meta.ToolID,
		Depth:         meta.Depth,
		CreatedAt:     time.Now().UTC(),
	}
	resp, err := requester.RequestInfo(ctx, req)
	if err != nil {
		return &PolicyError{
			Message:          fmt.Sprintf("command approval failed: %v", err),
			Decision:         DecisionAsk,
			PolicyID:         result.policyID,
			RequiresApproval: true,
		}
	}
	for _, choice := range resp.ChoiceIDs {
		if strings.EqualFold(choice, "approve") {
			return nil
		}
	}
	if strings.EqualFold(strings.TrimSpace(resp.Answer), "approve") {
		return nil
	}
	return &PolicyError{
		Message:          fmt.Sprintf("command approval denied: %s", strings.Join(argv, " ")),
		Decision:         DecisionDeny,
		PolicyID:         result.policyID,
		RequiresApproval: false,
	}
}

func policyDeny(message, policyID string, requiresApproval bool) *PolicyError {
	return &PolicyError{Message: message, Decision: DecisionDeny, PolicyID: policyID, RequiresApproval: requiresApproval}
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
