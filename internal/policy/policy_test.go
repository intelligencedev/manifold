package policy

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestStaticEnforcerBlocksApprovedHardConstraint(t *testing.T) {
	t.Parallel()

	enforcer := NewStaticEnforcer([]Record{{
		ID:            "no-secrets",
		Scope:         ScopeProject,
		TenantID:      7,
		ProjectID:     "project-1",
		Severity:      SeverityHard,
		Statement:     "Do not write secrets.",
		ApprovalState: ApprovalApproved,
		Targets:       TargetSelector{Tools: []string{"write_file"}, PathPrefixes: []string{"secrets"}, Roles: []string{"orchestrator"}},
	}})
	args := json.RawMessage(`{"path":"secrets/token.txt"}`)
	decision, err := enforcer.Evaluate(context.Background(), EvaluationRequest{TenantID: 7, ProjectID: "project-1", Role: "orchestrator", ToolName: "write_file", Args: args})
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if decision.Allowed || decision.RecordID != "no-secrets" {
		t.Fatalf("expected hard block, got %+v", decision)
	}
}

func TestStaticEnforcerIgnoresProposedHardConstraint(t *testing.T) {
	t.Parallel()

	enforcer := NewStaticEnforcer([]Record{{
		ID:            "agent-proposed-hard-rule",
		Scope:         ScopeProject,
		TenantID:      7,
		ProjectID:     "project-1",
		Severity:      SeverityHard,
		Statement:     "This should not block until approved.",
		ApprovalState: ApprovalProposed,
		Targets:       TargetSelector{Tools: []string{"write_file"}},
	}})
	decision, err := enforcer.Evaluate(context.Background(), EvaluationRequest{TenantID: 7, ProjectID: "project-1", ToolName: "write_file"})
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("expected proposed hard constraint to be ignored, got %+v", decision)
	}
}

func TestStaticEnforcerReturnsSoftAnnotations(t *testing.T) {
	t.Parallel()

	enforcer := NewStaticEnforcer([]Record{{
		ID:            "prefer-review",
		Scope:         ScopeProject,
		TenantID:      7,
		ProjectID:     "project-1",
		Severity:      SeveritySoft,
		Statement:     "Prefer reviewer confirmation before deployment.",
		ApprovalState: ApprovalApproved,
		Targets:       TargetSelector{Tools: []string{"deploy"}},
	}})
	decision, err := enforcer.Evaluate(context.Background(), EvaluationRequest{TenantID: 7, ProjectID: "project-1", ToolName: "deploy"})
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if !decision.Allowed || len(decision.Annotations) != 1 {
		t.Fatalf("expected soft annotation without block, got %+v", decision)
	}
}

func TestBuildPromptSectionIncludesApprovedSoftPolicies(t *testing.T) {
	t.Parallel()

	section := BuildPromptSection([]Record{
		{ID: "soft", Severity: SeveritySoft, ApprovalState: ApprovalApproved, Statement: "Prefer small edits."},
		{ID: "hard", Severity: SeverityHard, ApprovalState: ApprovalApproved, Statement: "Hard rule."},
	})
	if section == "" || !strings.Contains(section, "Prefer small edits") || strings.Contains(section, "Hard rule") {
		t.Fatalf("unexpected policy prompt section %q", section)
	}
}
