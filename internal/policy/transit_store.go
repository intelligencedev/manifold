package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"manifold/internal/transit"
)

type TransitService interface {
	ListKeys(ctx context.Context, tenantID int64, req transit.ListRequest) ([]transit.Metadata, error)
	GetMemory(ctx context.Context, tenantID int64, keys []string) ([]transit.Record, error)
}

type TransitEnforcer struct {
	Service TransitService
}

func NewTransitEnforcer(service TransitService) TransitEnforcer {
	return TransitEnforcer{Service: service}
}

func ConstraintProjectKey(projectID, constraintID string) string {
	return "constraint/project/" + strings.TrimSpace(projectID) + "/" + strings.TrimSpace(constraintID)
}

func ConstraintOrgKey(tenantID int64, constraintID string) string {
	return fmt.Sprintf("constraint/org/%d/%s", tenantID, strings.TrimSpace(constraintID))
}

func PolicyProjectRoleKey(projectID, roleName, policyID string) string {
	return "policy/project/" + strings.TrimSpace(projectID) + "/role/" + strings.TrimSpace(roleName) + "/" + strings.TrimSpace(policyID)
}

func PolicyOrgKey(tenantID int64, policyID string) string {
	return fmt.Sprintf("policy/org/%d/%s", tenantID, strings.TrimSpace(policyID))
}

func (e TransitEnforcer) Evaluate(ctx context.Context, req EvaluationRequest) (Decision, error) {
	if e.Service == nil {
		return Decision{Allowed: true}, nil
	}
	records, err := e.loadRecords(ctx, req)
	if err != nil {
		return Decision{Allowed: true}, err
	}
	return NewStaticEnforcer(records).Evaluate(ctx, req)
}

func (e TransitEnforcer) PromptContext(ctx context.Context, req EvaluationRequest) ([]Record, error) {
	if e.Service == nil {
		return nil, nil
	}
	records, err := e.loadRecords(ctx, req)
	if err != nil {
		return nil, err
	}
	out := make([]Record, 0, len(records))
	for _, record := range records {
		record = normalizeRecord(record)
		if record.Severity == SeveritySoft && record.ApprovalState == ApprovalApproved && matchesPromptScope(record, req) {
			out = append(out, record)
		}
	}
	return out, nil
}

func matchesPromptScope(record Record, req EvaluationRequest) bool {
	if record.TenantID != 0 && record.TenantID != req.TenantID {
		return false
	}
	if record.Scope == ScopeProject && record.ProjectID != "" && record.ProjectID != strings.TrimSpace(req.ProjectID) {
		return false
	}
	return matchesAny(record.Targets.Roles, req.Role)
}

func (e TransitEnforcer) loadRecords(ctx context.Context, req EvaluationRequest) ([]Record, error) {
	prefixes := transitPrefixes(req)
	keys := make([]string, 0)
	seen := map[string]bool{}
	for _, prefix := range prefixes {
		items, err := e.Service.ListKeys(ctx, req.TenantID, transit.ListRequest{Prefix: prefix, Limit: 100})
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if strings.TrimSpace(item.KeyName) == "" || seen[item.KeyName] {
				continue
			}
			seen[item.KeyName] = true
			keys = append(keys, item.KeyName)
		}
	}
	if len(keys) == 0 {
		return nil, nil
	}
	items, err := e.Service.GetMemory(ctx, req.TenantID, keys)
	if err != nil {
		return nil, err
	}
	out := make([]Record, 0, len(items))
	for _, item := range items {
		record, ok := DecodeTransitRecord(item)
		if !ok {
			continue
		}
		out = append(out, record)
	}
	return out, nil
}

func transitPrefixes(req EvaluationRequest) []string {
	projectID := strings.TrimSpace(req.ProjectID)
	role := strings.TrimSpace(req.Role)
	prefixes := []string{fmt.Sprintf("constraint/org/%d/", req.TenantID), fmt.Sprintf("policy/org/%d/", req.TenantID)}
	if projectID != "" {
		prefixes = append(prefixes, "constraint/project/"+projectID+"/")
		if role != "" {
			prefixes = append(prefixes, "policy/project/"+projectID+"/role/"+role+"/")
		}
	}
	return prefixes
}

func DecodeTransitRecord(item transit.Record) (Record, bool) {
	var record Record
	if err := json.Unmarshal([]byte(item.Value), &record); err != nil {
		return Record{}, false
	}
	if record.ID == "" {
		record.ID = item.KeyName
	}
	if record.TenantID == 0 {
		record.TenantID = item.TenantID
	}
	return record, true
}
