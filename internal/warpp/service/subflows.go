package service

import (
	"context"
	"fmt"
	"strings"

	"manifold/internal/warpp"
)

func (s *Service) registerSubflowRunners(ctx context.Context, userID int64, runners map[string]warpp.NodeRunner) {
	if s == nil || s.deps.State == nil {
		return
	}
	summaries, err := s.deps.State.ListWorkflowSummaries(ctx, userID)
	if err != nil {
		return
	}
	for _, summary := range summaries {
		workflowID := summary.ID
		runners["flow."+workflowID] = s.subflowRunner(userID, workflowID)
	}
}

func (s *Service) subflowRunner(userID int64, workflowID string) warpp.NodeRunner {
	return func(ctx context.Context, _ warpp.RunnerCtx, input warpp.NodeInputs) (map[string]warpp.Value, error) {
		if s == nil || s.deps.State == nil {
			return nil, fmt.Errorf("subflow %s unavailable", workflowID)
		}
		doc, _, found, err := s.deps.State.GetWorkflow(ctx, userID, workflowID)
		if err != nil || !found {
			return nil, fmt.Errorf("subflow %s unavailable", workflowID)
		}
		values := make(map[string]any)
		for _, port := range doc.Inputs {
			if value, ok := input.Values[port.Name]; ok {
				values[port.Name] = value.Data
			}
		}
		result, _, err := s.RunSync(ctx, userID, doc, values)
		if err != nil {
			return nil, err
		}
		if result.Status == warpp.StatusFailed || result.Status == warpp.StatusCancelled {
			return nil, fmt.Errorf("subflow %s finished with status %s", workflowID, result.Status)
		}
		manifest, _ := warpp.WorkflowManifest(doc, BaseResolver())
		outputs := make(map[string]warpp.Value)
		for _, port := range manifest.Outputs {
			raw, ok := result.Outputs[port.Name]
			if !ok {
				continue
			}
			parsed, parseErr := warpp.ParseType(port.Type)
			if parseErr == nil {
				if value, coerceErr := warpp.CoerceRaw(raw, parsed); coerceErr == nil {
					outputs[port.Name] = value
					continue
				}
			}
			outputs[port.Name] = warpp.Value{Type: warpp.InferLiteral(raw), Data: raw}
		}
		return outputs, nil
	}
}

// SubflowRunner returns a node runner for a saved workflow.
func (s *Service) SubflowRunner(userID int64, workflowID string) warpp.NodeRunner {
	return s.subflowRunner(userID, workflowID)
}

// SubflowResolver returns the resolver used by a workflow editor or validator.
func (s *Service) SubflowResolver(ctx context.Context, userID int64) warpp.Resolver {
	return s.Resolver(ctx, userID)
}

// PublishedWorkflowManifests returns manifests for saved workflows.
func (s *Service) PublishedWorkflowManifests(ctx context.Context, userID int64) []warpp.Manifest {
	if s == nil || s.deps.State == nil {
		return nil
	}
	summaries, err := s.deps.State.ListWorkflowSummaries(ctx, userID)
	if err != nil {
		return nil
	}
	base := BaseResolver()
	manifests := make([]warpp.Manifest, 0, len(summaries))
	for _, summary := range summaries {
		doc, _, found, err := s.deps.State.GetWorkflow(ctx, userID, summary.ID)
		if err != nil || !found {
			continue
		}
		manifest, _ := warpp.WorkflowManifest(doc, base)
		manifests = append(manifests, manifest)
	}
	return manifests
}

// CheckSubflowCycles reports whether a document recursively includes itself.
func (s *Service) CheckSubflowCycles(ctx context.Context, userID int64, doc warpp.Document) []warpp.Diagnostic {
	if s == nil || s.deps.State == nil {
		return nil
	}
	seen := make(map[string]bool)
	var reaches func(string) bool
	reaches = func(id string) bool {
		if id == doc.ID {
			return true
		}
		if seen[id] {
			return false
		}
		seen[id] = true
		child, _, found, err := s.deps.State.GetWorkflow(ctx, userID, id)
		if err != nil || !found {
			return false
		}
		for _, ref := range SubflowRefs(child) {
			if reaches(ref) {
				return true
			}
		}
		return false
	}
	for _, ref := range SubflowRefs(doc) {
		if reaches(ref) {
			return []warpp.Diagnostic{{Severity: warpp.SeverityError, Code: "workflow.subflow.cycle", Message: "subflow inclusion forms a cycle"}}
		}
	}
	return nil
}

// SubflowRefs returns workflow IDs referenced by flow.* nodes, including
// nested block bodies.
func SubflowRefs(doc warpp.Document) []string {
	var ids []string
	var scan func([]warpp.Node)
	scan = func(nodes []warpp.Node) {
		for _, node := range nodes {
			if strings.HasPrefix(node.Type, "flow.") {
				ids = append(ids, strings.TrimPrefix(node.Type, "flow."))
			}
			if node.Body != nil {
				scan(node.Body.Nodes)
			}
		}
	}
	scan(doc.Nodes)
	return ids
}
