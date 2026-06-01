package agentd

import (
	"context"
	"encoding/json"
	"strings"

	"manifold/internal/agent/memory/belief"
	"manifold/internal/policy"
	transitdomain "manifold/internal/transit"
)

type beliefPolicySink struct {
	service *transitdomain.Service
}

func (s beliefPolicySink) UpsertPolicyForBelief(ctx context.Context, item belief.Belief, promotion belief.Promotion) error {
	if s.service == nil {
		return nil
	}
	record, key := policyRecordForBelief(item, promotion)
	if strings.TrimSpace(key) == "" {
		return nil
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	description := "Autonomous belief-memory policy for belief " + item.ID
	created, err := s.service.CreateMemory(ctx, item.TenantID, item.TenantID, []transitdomain.CreateMemoryItem{{
		KeyName:     key,
		Description: description,
		Value:       string(data),
		EmbedSource: "value",
	}})
	if err == nil || len(created) > 0 {
		return err
	}
	_, err = s.service.UpdateMemory(ctx, item.TenantID, item.TenantID, transitdomain.UpdateMemoryRequest{
		KeyName:     key,
		Value:       string(data),
		EmbedSource: "value",
	})
	return err
}

func policyRecordForBelief(item belief.Belief, promotion belief.Promotion) (policy.Record, string) {
	projectID := beliefMetadataString(item.Metadata, "projectId")
	if projectID == "" {
		projectID = beliefMetadataString(item.Metadata, "projectID")
	}
	if projectID == "" {
		return policy.Record{}, ""
	}
	role := strings.ToLower(strings.TrimSpace(beliefMetadataString(item.Metadata, "agentRole")))
	if role == "" {
		role = "orchestrator"
	}
	severity := policy.SeveritySoft
	key := policy.PolicyProjectRoleKey(projectID, role, item.ID)
	if item.Enforcement == belief.EnforcementHardConstraint {
		severity = policy.SeverityHard
		key = policy.ConstraintProjectKey(projectID, item.ID)
	}
	return policy.Record{
		ID:            key,
		Kind:          "belief_memory",
		Scope:         policy.ScopeProject,
		TenantID:      item.TenantID,
		ProjectID:     projectID,
		Severity:      severity,
		Statement:     item.Statement,
		Targets:       policy.TargetSelector{Roles: []string{role}},
		ApprovalState: policy.ApprovalApproved,
		ActorUserID:   item.TenantID,
		Metadata: map[string]any{
			"source":                "belief_memory",
			"beliefId":              item.ID,
			"beliefStatementHash":   item.StatementHash,
			"beliefConfidence":      item.Confidence,
			"beliefEvidenceFor":     item.EvidenceFor,
			"beliefEvidenceAgainst": item.EvidenceAgainst,
			"beliefKind":            item.Kind,
			"beliefEnforcement":     item.Enforcement,
			"promotionId":           promotion.ID,
		},
	}, key
}

func beliefMetadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	value, _ := metadata[key].(string)
	return strings.TrimSpace(value)
}
