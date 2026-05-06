package policy

import (
	"context"
	"encoding/json"
	"testing"

	"manifold/internal/transit"
)

type fakeTransitService struct {
	records map[string]transit.Record
}

func (s fakeTransitService) ListKeys(_ context.Context, tenantID int64, req transit.ListRequest) ([]transit.Metadata, error) {
	out := make([]transit.Metadata, 0)
	for key, record := range s.records {
		if record.TenantID != tenantID || !stringsHasPrefix(key, req.Prefix) {
			continue
		}
		out = append(out, transit.Metadata{KeyName: key})
	}
	return out, nil
}

func (s fakeTransitService) GetMemory(_ context.Context, tenantID int64, keys []string) ([]transit.Record, error) {
	out := make([]transit.Record, 0, len(keys))
	for _, key := range keys {
		if record, ok := s.records[key]; ok && record.TenantID == tenantID {
			out = append(out, record)
		}
	}
	return out, nil
}

func TestTransitEnforcerLoadsConstraintRecords(t *testing.T) {
	t.Parallel()

	value, err := json.Marshal(Record{
		ID:            "block-delete",
		Scope:         ScopeProject,
		TenantID:      7,
		ProjectID:     "project-1",
		Severity:      SeverityHard,
		Statement:     "Do not delete generated artifacts.",
		ApprovalState: ApprovalApproved,
		Targets:       TargetSelector{Tools: []string{"delete_file"}, PathPrefixes: []string{"artifacts"}},
	})
	if err != nil {
		t.Fatalf("marshal record: %v", err)
	}
	enforcer := NewTransitEnforcer(fakeTransitService{records: map[string]transit.Record{
		ConstraintProjectKey("project-1", "block-delete"): {TenantID: 7, KeyName: ConstraintProjectKey("project-1", "block-delete"), Value: string(value)},
	}})

	decision, err := enforcer.Evaluate(context.Background(), EvaluationRequest{TenantID: 7, ProjectID: "project-1", ToolName: "delete_file", Args: json.RawMessage(`{"path":"artifacts/out.txt"}`)})
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if decision.Allowed || decision.RecordID != "block-delete" {
		t.Fatalf("expected Transit hard constraint to block, got %+v", decision)
	}
}

func stringsHasPrefix(value, prefix string) bool {
	return len(value) >= len(prefix) && value[:len(prefix)] == prefix
}
