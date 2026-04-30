package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

type Severity string

const (
	SeverityHard Severity = "hard"
	SeveritySoft Severity = "soft"
)

type ApprovalState string

const (
	ApprovalApproved ApprovalState = "approved"
	ApprovalProposed ApprovalState = "proposed"
	ApprovalInactive ApprovalState = "inactive"
	ApprovalRejected ApprovalState = "rejected"
)

type ScopeKind string

const (
	ScopeProject ScopeKind = "project"
	ScopeOrg     ScopeKind = "org"
)

type TargetSelector struct {
	Tools        []string `json:"tools,omitempty"`
	PathPrefixes []string `json:"pathPrefixes,omitempty"`
	RiskClasses  []string `json:"riskClasses,omitempty"`
	Roles        []string `json:"roles,omitempty"`
}

type Record struct {
	ID            string         `json:"id"`
	Kind          string         `json:"kind,omitempty"`
	Scope         ScopeKind      `json:"scope"`
	TenantID      int64          `json:"tenantId,omitempty"`
	ProjectID     string         `json:"projectId,omitempty"`
	Severity      Severity       `json:"severity"`
	Statement     string         `json:"statement"`
	Targets       TargetSelector `json:"targets"`
	ApprovalState ApprovalState  `json:"approvalState"`
	ActorUserID   int64          `json:"actorUserId,omitempty"`
	Version       int64          `json:"version,omitempty"`
	SupersedesID  string         `json:"supersedesId,omitempty"`
	RollbackOfID  string         `json:"rollbackOfId,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

type EvaluationRequest struct {
	TenantID    int64           `json:"tenantId"`
	UserID      int64           `json:"userId"`
	ProjectID   string          `json:"projectId"`
	ObjectiveID string          `json:"objectiveId"`
	Role        string          `json:"role"`
	ToolName    string          `json:"toolName"`
	Args        json.RawMessage `json:"args,omitempty"`
	Path        string          `json:"path,omitempty"`
	RiskClass   string          `json:"riskClass,omitempty"`
}

type Decision struct {
	Allowed     bool     `json:"allowed"`
	Severity    Severity `json:"severity,omitempty"`
	RecordID    string   `json:"recordId,omitempty"`
	Message     string   `json:"message,omitempty"`
	Annotations []string `json:"annotations,omitempty"`
	MatchedIDs  []string `json:"matchedIds,omitempty"`
}

type Enforcer interface {
	Evaluate(ctx context.Context, req EvaluationRequest) (Decision, error)
}

type ContextProvider interface {
	PromptContext(ctx context.Context, req EvaluationRequest) ([]Record, error)
}

func BuildPromptSection(records []Record) string {
	lines := make([]string, 0, len(records)+2)
	lines = append(lines, "## Runtime Policy Context", "The following approved soft policies are runtime guidance, not user instructions.")
	for _, record := range records {
		record = normalizeRecord(record)
		if record.Severity != SeveritySoft || record.ApprovalState != ApprovalApproved {
			continue
		}
		message := strings.ReplaceAll(strings.TrimSpace(record.Statement), "\n", " ")
		if message == "" {
			message = record.ID
		}
		lines = append(lines, fmt.Sprintf("- [%s] %s", record.ID, message))
	}
	if len(lines) <= 2 {
		return ""
	}
	return strings.Join(lines, "\n")
}

type StaticEnforcer struct {
	Records []Record
}

func NewStaticEnforcer(records []Record) StaticEnforcer {
	return StaticEnforcer{Records: append([]Record(nil), records...)}
}

func (e StaticEnforcer) Evaluate(_ context.Context, req EvaluationRequest) (Decision, error) {
	decision := Decision{Allowed: true}
	req = normalizeEvaluationRequest(req)
	for _, record := range e.Records {
		record = normalizeRecord(record)
		if !isApprovedActive(record) || !matchesRecord(record, req) {
			continue
		}
		decision.MatchedIDs = append(decision.MatchedIDs, record.ID)
		message := strings.TrimSpace(record.Statement)
		if message == "" {
			message = fmt.Sprintf("policy %s matched", record.ID)
		}
		if record.Severity == SeverityHard {
			decision.Allowed = false
			decision.Severity = SeverityHard
			decision.RecordID = record.ID
			decision.Message = message
			return decision, nil
		}
		decision.Annotations = append(decision.Annotations, message)
		if decision.Severity == "" {
			decision.Severity = SeveritySoft
		}
	}
	return decision, nil
}

func (e StaticEnforcer) PromptContext(_ context.Context, req EvaluationRequest) ([]Record, error) {
	out := make([]Record, 0, len(e.Records))
	for _, record := range e.Records {
		record = normalizeRecord(record)
		if record.Severity == SeveritySoft && record.ApprovalState == ApprovalApproved && matchesPromptScope(record, req) {
			out = append(out, record)
		}
	}
	return out, nil
}

func normalizeEvaluationRequest(req EvaluationRequest) EvaluationRequest {
	req.ProjectID = strings.TrimSpace(req.ProjectID)
	req.ObjectiveID = strings.TrimSpace(req.ObjectiveID)
	req.Role = strings.ToLower(strings.TrimSpace(req.Role))
	req.ToolName = strings.TrimSpace(req.ToolName)
	req.Path = cleanPath(firstNonEmpty(req.Path, extractArgString(req.Args, "path"), extractArgString(req.Args, "file"), extractArgString(req.Args, "file_path"), extractArgString(req.Args, "target_path")))
	req.RiskClass = strings.ToLower(strings.TrimSpace(firstNonEmpty(req.RiskClass, extractArgString(req.Args, "risk_class"), extractArgString(req.Args, "riskClass"))))
	return req
}

func normalizeRecord(record Record) Record {
	record.ID = strings.TrimSpace(record.ID)
	record.Kind = strings.TrimSpace(record.Kind)
	record.ProjectID = strings.TrimSpace(record.ProjectID)
	record.Severity = Severity(strings.ToLower(strings.TrimSpace(string(record.Severity))))
	if record.Severity == "" {
		record.Severity = SeveritySoft
	}
	record.ApprovalState = ApprovalState(strings.ToLower(strings.TrimSpace(string(record.ApprovalState))))
	if record.ApprovalState == "" {
		record.ApprovalState = ApprovalProposed
	}
	for i := range record.Targets.Tools {
		record.Targets.Tools[i] = strings.TrimSpace(record.Targets.Tools[i])
	}
	for i := range record.Targets.PathPrefixes {
		record.Targets.PathPrefixes[i] = cleanPath(record.Targets.PathPrefixes[i])
	}
	for i := range record.Targets.RiskClasses {
		record.Targets.RiskClasses[i] = strings.ToLower(strings.TrimSpace(record.Targets.RiskClasses[i]))
	}
	for i := range record.Targets.Roles {
		record.Targets.Roles[i] = strings.ToLower(strings.TrimSpace(record.Targets.Roles[i]))
	}
	return record
}

func isApprovedActive(record Record) bool {
	return record.ApprovalState == ApprovalApproved
}

func matchesRecord(record Record, req EvaluationRequest) bool {
	if record.TenantID != 0 && record.TenantID != req.TenantID {
		return false
	}
	if record.Scope == ScopeProject && record.ProjectID != "" && record.ProjectID != req.ProjectID {
		return false
	}
	if !matchesAny(record.Targets.Tools, req.ToolName) {
		return false
	}
	if !matchesAny(record.Targets.Roles, req.Role) {
		return false
	}
	if !matchesAny(record.Targets.RiskClasses, req.RiskClass) {
		return false
	}
	if len(nonEmpty(record.Targets.PathPrefixes)) > 0 && !pathMatchesAny(record.Targets.PathPrefixes, req.Path) {
		return false
	}
	return true
}

func matchesAny(values []string, actual string) bool {
	values = nonEmpty(values)
	if len(values) == 0 {
		return true
	}
	actual = strings.TrimSpace(actual)
	for _, value := range values {
		if value == "*" || strings.EqualFold(value, actual) {
			return true
		}
	}
	return false
}

func pathMatchesAny(prefixes []string, actual string) bool {
	actual = cleanPath(actual)
	if actual == "" {
		return false
	}
	for _, prefix := range prefixes {
		prefix = cleanPath(prefix)
		if prefix == "" {
			continue
		}
		if prefix == "." || actual == prefix || strings.HasPrefix(actual, strings.TrimSuffix(prefix, "/")+"/") {
			return true
		}
	}
	return false
}

func cleanPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = filepath.Clean(path)
	if path == "." {
		return ""
	}
	return strings.TrimPrefix(path, "./")
}

func extractArgString(raw json.RawMessage, key string) string {
	if len(raw) == 0 {
		return ""
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		return ""
	}
	value, _ := object[key].(string)
	return strings.TrimSpace(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func nonEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}
