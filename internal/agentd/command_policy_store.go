package agentd

import (
	"context"

	"manifold/internal/config"
	persist "manifold/internal/persistence"
)

func initializeCommandPolicy(ctx context.Context, cfg *config.Config, store persist.CommandPolicyStore) error {
	if cfg == nil || store == nil {
		return nil
	}

	rules, err := store.ListRules(ctx, systemUserID)
	if err != nil {
		return err
	}
	if len(rules) == 0 {
		rules, err = seedCommandPolicyRules(ctx, store, cfg.Exec.CommandRules)
		if err != nil {
			return err
		}
	}
	if len(rules) == 0 {
		return nil
	}
	if err := validateCommandRulesSettings(rules); err != nil {
		return err
	}
	cfg.Exec.CommandRules = cloneCommandRules(rules)
	return nil
}

func seedCommandPolicyRules(ctx context.Context, store persist.CommandPolicyStore, rules []config.ExecCommandRule) ([]config.ExecCommandRule, error) {
	seeded := make([]config.ExecCommandRule, 0, len(rules))
	for _, rule := range rules {
		rule = normalizeCommandApprovalRule(rule)
		if err := validateCommandRulesSettings([]config.ExecCommandRule{rule}); err != nil {
			return nil, err
		}
		stored, err := store.UpsertRule(ctx, systemUserID, rule)
		if err != nil {
			return nil, err
		}
		seeded = append(seeded, stored)
	}
	return seeded, nil
}
