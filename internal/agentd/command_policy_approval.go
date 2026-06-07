package agentd

import (
	"context"
	"errors"
	"slices"
	"strings"

	"manifold/internal/commandexec"
	"manifold/internal/config"
)

type commandPolicyApprovalController struct {
	app *app
}

func (c commandPolicyApprovalController) PersistCommandAllowRule(ctx context.Context, rule config.ExecCommandRule) (config.ExecCommandRule, error) {
	if err := ctx.Err(); err != nil {
		return config.ExecCommandRule{}, err
	}
	if c.app == nil || c.app.cfg == nil {
		return config.ExecCommandRule{}, errors.New("command policy config is unavailable")
	}
	if c.app.commandPolicyStore == nil {
		return config.ExecCommandRule{}, errors.New("command policy store is unavailable")
	}
	rule = normalizeCommandApprovalRule(rule)
	if err := validateCommandRulesSettings([]config.ExecCommandRule{rule}); err != nil {
		return config.ExecCommandRule{}, err
	}

	c.app.commandPolicyMu.Lock()
	defer c.app.commandPolicyMu.Unlock()

	if existing, ok := findEquivalentCommandRule(c.app.cfg.Exec.CommandRules, rule); ok {
		c.app.updateLiveCommandPolicy(existing)
		return existing, nil
	}

	stored, err := c.app.commandPolicyStore.UpsertRule(ctx, systemUserID, rule)
	if err != nil {
		return config.ExecCommandRule{}, err
	}
	c.app.cfg.Exec.CommandRules = append(c.app.cfg.Exec.CommandRules, cloneCommandApprovalRule(stored))
	c.app.updateLiveCommandPolicy(stored)
	return stored, nil
}

func (c commandPolicyApprovalController) SessionAllowAllCommands(ctx context.Context, scope commandexec.CommandSessionScope) (bool, error) {
	if c.app == nil || c.app.commandPolicyStore == nil {
		return false, errors.New("command policy store is unavailable")
	}
	override, ok, err := c.app.commandPolicyStore.GetSessionOverride(ctx, scope.UserID, scope.SessionID)
	if err != nil || !ok {
		return false, err
	}
	return override.AllowAllCommands, nil
}

func (c commandPolicyApprovalController) SetSessionAllowAllCommands(ctx context.Context, scope commandexec.CommandSessionScope, allow bool) error {
	if c.app == nil || c.app.commandPolicyStore == nil {
		return errors.New("command policy store is unavailable")
	}
	return c.app.commandPolicyStore.SetSessionAllowAll(ctx, scope.UserID, scope.SessionID, allow)
}

func commandPolicySessionScope(userID *int64, sessionID string) commandexec.CommandSessionScope {
	scope := commandexec.CommandSessionScope{
		UserID:    commandPolicyUserID(userID),
		SessionID: strings.TrimSpace(sessionID),
	}
	return scope
}

func (a *app) updateLiveCommandPolicy(rule config.ExecCommandRule) {
	if a.cliExecutor != nil {
		a.cliExecutor.AddCommandRule(rule)
	}
	if a.terminalManager != nil {
		a.terminalManager.AddCommandRule(rule)
	}
}

func findEquivalentCommandRule(rules []config.ExecCommandRule, rule config.ExecCommandRule) (config.ExecCommandRule, bool) {
	for _, existing := range rules {
		normalizedExisting := normalizeCommandApprovalRule(existing)
		if normalizedExisting.ID != "" && normalizedExisting.ID == rule.ID {
			return normalizedExisting, true
		}
		if normalizedExisting.Decision == rule.Decision &&
			slices.Equal(normalizedExisting.Pattern, rule.Pattern) &&
			slices.Equal(normalizedExisting.Contexts, rule.Contexts) {
			return normalizedExisting, true
		}
	}
	return config.ExecCommandRule{}, false
}

func normalizeCommandApprovalRule(rule config.ExecCommandRule) config.ExecCommandRule {
	rule.ID = strings.TrimSpace(rule.ID)
	rule.Decision = strings.ToLower(strings.TrimSpace(rule.Decision))
	rule.Justification = strings.TrimSpace(rule.Justification)
	rule.Pattern = trimStringSlice(rule.Pattern, false)
	rule.Contexts = trimStringSlice(rule.Contexts, true)
	return rule
}

func trimStringSlice(values []string, lower bool) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if lower {
			value = strings.ToLower(value)
		}
		out = append(out, value)
	}
	return out
}

func cloneCommandRules(rules []config.ExecCommandRule) []config.ExecCommandRule {
	out := make([]config.ExecCommandRule, 0, len(rules))
	for _, rule := range rules {
		out = append(out, cloneCommandApprovalRule(rule))
	}
	return out
}

func cloneCommandApprovalRule(rule config.ExecCommandRule) config.ExecCommandRule {
	out := rule
	out.Pattern = append([]string(nil), rule.Pattern...)
	out.Contexts = append([]string(nil), rule.Contexts...)
	return out
}
