package flow

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

type DiagnosticSeverity string

const (
	DiagnosticSeverityError   DiagnosticSeverity = "error"
	DiagnosticSeverityWarning DiagnosticSeverity = "warning"
)

const legacyExpressionPrefix = "$" + "{A."

type Diagnostic struct {
	Severity DiagnosticSeverity `json:"severity"`
	Code     string             `json:"code"`
	Message  string             `json:"message"`
	Path     string             `json:"path,omitempty"`
}

type Plan struct {
	WorkflowID string            `json:"workflow_id"`
	NodeOrder  []string          `json:"node_order"`
	Incoming   map[string][]Edge `json:"incoming"`
	Outgoing   map[string][]Edge `json:"outgoing"`
	Indegree   map[string]int    `json:"indegree"`
}

// ValidateWorkflow validates a workflow definition and returns diagnostics.
func ValidateWorkflow(wf Workflow) []Diagnostic {
	collector := diagnosticCollector{}
	validateWorkflowMetadata(wf, &collector)
	nodeByID := validateWorkflowNodes(wf, &collector)
	adj, indegree := validateWorkflowEdges(wf, nodeByID, &collector)
	validateWorkflowAcyclic(wf, adj, indegree, &collector)
	return collector.diags
}

type diagnosticCollector struct {
	diags []Diagnostic
}

func (c *diagnosticCollector) add(sev DiagnosticSeverity, code, msg, path string) {
	c.diags = append(c.diags, Diagnostic{
		Severity: sev,
		Code:     code,
		Message:  msg,
		Path:     path,
	})
}

func validateWorkflowMetadata(wf Workflow, collector *diagnosticCollector) {
	if strings.TrimSpace(wf.ID) == "" {
		collector.add(DiagnosticSeverityError, "workflow.id.required", "workflow id is required", "workflow.id")
	}
	if strings.TrimSpace(wf.Name) == "" {
		collector.add(DiagnosticSeverityError, "workflow.name.required", "workflow name is required", "workflow.name")
	}
	if !slices.Contains(validTriggerTypes(), wf.Trigger.Type) {
		collector.add(
			DiagnosticSeverityError,
			"workflow.trigger.invalid_type",
			fmt.Sprintf("trigger type must be one of %q", validTriggerTypes()),
			"workflow.trigger.type",
		)
	}
	validateWorkflowTrigger(wf.Trigger, collector)
	if len(wf.Nodes) == 0 {
		collector.add(
			DiagnosticSeverityError,
			"workflow.nodes.required",
			"workflow must contain at least one node",
			"workflow.nodes",
		)
	}
}

func validateWorkflowTrigger(trigger Trigger, collector *diagnosticCollector) {
	switch trigger.Type {
	case TriggerTypeSchedule:
		if trigger.Schedule == nil || strings.TrimSpace(trigger.Schedule.Cron) == "" {
			collector.add(
				DiagnosticSeverityError,
				"workflow.trigger.schedule.required",
				"schedule trigger requires schedule.cron",
				"workflow.trigger.schedule.cron",
			)
		}
	case TriggerTypeWebhook:
		if trigger.Webhook == nil {
			collector.add(
				DiagnosticSeverityError,
				"workflow.trigger.webhook.required",
				"webhook trigger requires webhook config",
				"workflow.trigger.webhook",
			)
		} else {
			if strings.TrimSpace(trigger.Webhook.Method) == "" {
				collector.add(
					DiagnosticSeverityError,
					"workflow.trigger.webhook.method.required",
					"webhook trigger requires method",
					"workflow.trigger.webhook.method",
				)
			}
			if strings.TrimSpace(trigger.Webhook.Path) == "" {
				collector.add(
					DiagnosticSeverityError,
					"workflow.trigger.webhook.path.required",
					"webhook trigger requires path",
					"workflow.trigger.webhook.path",
				)
			}
		}
	case TriggerTypeEvent:
		if trigger.Event == nil || strings.TrimSpace(trigger.Event.Name) == "" {
			collector.add(
				DiagnosticSeverityError,
				"workflow.trigger.event.required",
				"event trigger requires event.name",
				"workflow.trigger.event.name",
			)
		}
	}
}

func validateWorkflowNodes(wf Workflow, collector *diagnosticCollector) map[string]Node {
	nodeByID := map[string]Node{}
	for i, n := range wf.Nodes {
		validateWorkflowNode(i, n, nodeByID, collector)
	}
	return nodeByID
}

func validateWorkflowNode(i int, n Node, nodeByID map[string]Node, collector *diagnosticCollector) {
	idxPath := fmt.Sprintf("workflow.nodes[%d]", i)
	validateWorkflowNodeIdentity(n, idxPath, nodeByID, collector)
	validateWorkflowNodeExecution(n, idxPath, collector)
	validateWorkflowNodeInputs(n, idxPath, collector)
}

func validateWorkflowNodeIdentity(n Node, idxPath string, nodeByID map[string]Node, collector *diagnosticCollector) {
	if strings.TrimSpace(n.ID) == "" {
		collector.add(DiagnosticSeverityError, "node.id.required", "node id is required", idxPath+".id")
	} else {
		if _, exists := nodeByID[n.ID]; exists {
			collector.add(DiagnosticSeverityError, "node.id.duplicate", "node id must be unique", idxPath+".id")
		}
		nodeByID[n.ID] = n
	}
	if strings.TrimSpace(n.Name) == "" {
		collector.add(DiagnosticSeverityError, "node.name.required", "node name is required", idxPath+".name")
	}
	if !slices.Contains(validNodeKinds(), n.Kind) {
		collector.add(DiagnosticSeverityError, "node.kind.invalid", fmt.Sprintf("node kind must be one of %q", validNodeKinds()), idxPath+".kind")
	}
	if strings.TrimSpace(n.Type) == "" {
		collector.add(DiagnosticSeverityError, "node.type.required", "node type is required", idxPath+".type")
	}
	if n.Type == "tool" && strings.TrimSpace(n.Tool) == "" {
		collector.add(DiagnosticSeverityError, "node.tool.required", "tool node requires tool name", idxPath+".tool")
	}
}

func validateWorkflowNodeExecution(n Node, idxPath string, collector *diagnosticCollector) {
	if !slices.Contains(validErrorStrategies(), n.Execution.OnError) {
		collector.add(
			DiagnosticSeverityError,
			"node.execution.on_error.invalid",
			fmt.Sprintf("on_error must be one of %q", validErrorStrategies()),
			idxPath+".execution.on_error",
		)
	}
	if n.Execution.Retries.Max < 0 {
		collector.add(DiagnosticSeverityError, "node.execution.retries.invalid_max", "retries.max must be >= 0", idxPath+".execution.retries.max")
	}
	if !slices.Contains(validBackoffStrategies(), n.Execution.Retries.Backoff) {
		collector.add(
			DiagnosticSeverityError,
			"node.execution.retries.invalid_backoff",
			fmt.Sprintf("retries.backoff must be one of %q", validBackoffStrategies()),
			idxPath+".execution.retries.backoff",
		)
	}
}

func validateWorkflowNodeInputs(n Node, idxPath string, collector *diagnosticCollector) {
	for key, binding := range n.Inputs {
		path := idxPath + ".inputs." + key
		hasExpr := strings.TrimSpace(binding.Expression) != ""
		hasLiteral := binding.Literal != nil
		if hasExpr == hasLiteral {
			collector.add(DiagnosticSeverityError, "node.input.binding.exclusive", "input binding must set exactly one of expression or literal", path)
			continue
		}
		if hasExpr && strings.Contains(binding.Expression, legacyExpressionPrefix) {
			collector.add(DiagnosticSeverityWarning, "node.input.expression.legacy", "legacy ${A.*} expression detected; use $node/$run expressions", path+".expression")
		}
	}
}

func validateWorkflowEdges(wf Workflow, nodeByID map[string]Node, collector *diagnosticCollector) (map[string][]string, map[string]int) {
	edgeByID := map[string]struct{}{}
	adj := map[string][]string{}
	indegree := map[string]int{}
	for _, n := range wf.Nodes {
		indegree[n.ID] = 0
	}
	for i, e := range wf.Edges {
		srcKnown, dstKnown := validateWorkflowEdge(i, e, edgeByID, nodeByID, collector)
		if srcKnown && dstKnown {
			adj[e.Source.NodeID] = append(adj[e.Source.NodeID], e.Target.NodeID)
			indegree[e.Target.NodeID]++
		}
	}
	return adj, indegree
}

func validateWorkflowEdge(
	i int,
	e Edge,
	edgeByID map[string]struct{},
	nodeByID map[string]Node,
	collector *diagnosticCollector,
) (bool, bool) {
	idxPath := fmt.Sprintf("workflow.edges[%d]", i)
	if strings.TrimSpace(e.ID) != "" {
		if _, exists := edgeByID[e.ID]; exists {
			collector.add(DiagnosticSeverityError, "edge.id.duplicate", "edge id must be unique", idxPath+".id")
		}
		edgeByID[e.ID] = struct{}{}
	}
	srcKnown := validateWorkflowEdgeEndpoint("source", e.Source, idxPath, nodeByID, collector)
	dstKnown := validateWorkflowEdgeEndpoint("target", e.Target, idxPath, nodeByID, collector)
	if e.Source.NodeID != "" && e.Source.NodeID == e.Target.NodeID {
		collector.add(DiagnosticSeverityError, "edge.self_loop", "self-loop edges are not supported", idxPath)
	}
	for j, m := range e.Mapping {
		mp := fmt.Sprintf("%s.mapping[%d]", idxPath, j)
		if strings.TrimSpace(m.From) == "" {
			collector.add(DiagnosticSeverityError, "edge.mapping.from.required", "mapping.from is required", mp+".from")
		}
		if strings.TrimSpace(m.To) == "" {
			collector.add(DiagnosticSeverityError, "edge.mapping.to.required", "mapping.to is required", mp+".to")
		}
	}
	return srcKnown, dstKnown
}

func validateWorkflowEdgeEndpoint(
	label string,
	endpoint PortRef,
	idxPath string,
	nodeByID map[string]Node,
	collector *diagnosticCollector,
) bool {
	path := idxPath + "." + label
	if strings.TrimSpace(endpoint.NodeID) == "" {
		collector.add(DiagnosticSeverityError, "edge."+label+".node_id.required", "edge "+label+" node_id is required", path+".node_id")
		return false
	}
	if _, ok := nodeByID[endpoint.NodeID]; !ok {
		collector.add(DiagnosticSeverityError, "edge."+label+".node_id.unknown", "edge "+label+" node_id does not exist", path+".node_id")
		return false
	}
	if strings.TrimSpace(endpoint.Port) == "" {
		collector.add(DiagnosticSeverityError, "edge."+label+".port.required", "edge "+label+" port is required", path+".port")
	}
	return true
}

func validateWorkflowAcyclic(
	wf Workflow,
	adj map[string][]string,
	indegree map[string]int,
	collector *diagnosticCollector,
) {
	if hasError(collector.diags) {
		return
	}
	remaining := map[string]int{}
	maps.Copy(remaining, indegree)
	queue := make([]string, 0)
	for id, d := range remaining {
		if d == 0 {
			queue = append(queue, id)
		}
	}
	visited := 0
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		visited++
		for _, to := range adj[id] {
			remaining[to]--
			if remaining[to] == 0 {
				queue = append(queue, to)
			}
		}
	}
	if visited != len(wf.Nodes) {
		collector.add(DiagnosticSeverityError, "workflow.graph.cycle", "workflow graph contains at least one cycle", "workflow.edges")
	}
}

func validTriggerTypes() []TriggerType {
	return []TriggerType{TriggerTypeManual, TriggerTypeSchedule, TriggerTypeWebhook, TriggerTypeEvent}
}

func validNodeKinds() []NodeKind {
	return []NodeKind{NodeKindAction, NodeKindLogic, NodeKindData}
}

func validErrorStrategies() []ErrorStrategy {
	return []ErrorStrategy{"", ErrorStrategyFail, ErrorStrategyContinue}
}

func validBackoffStrategies() []BackoffStrategy {
	return []BackoffStrategy{BackoffNone, BackoffFixed, BackoffExponential}
}

// CompileWorkflow validates and compiles a workflow to an execution plan.
func CompileWorkflow(wf Workflow) (*Plan, []Diagnostic) {
	diags := ValidateWorkflow(wf)
	if hasError(diags) {
		return nil, diags
	}

	incoming := make(map[string][]Edge, len(wf.Nodes))
	outgoing := make(map[string][]Edge, len(wf.Nodes))
	indegree := make(map[string]int, len(wf.Nodes))
	for _, n := range wf.Nodes {
		indegree[n.ID] = 0
		incoming[n.ID] = []Edge{}
		outgoing[n.ID] = []Edge{}
	}
	for _, e := range wf.Edges {
		incoming[e.Target.NodeID] = append(incoming[e.Target.NodeID], e)
		outgoing[e.Source.NodeID] = append(outgoing[e.Source.NodeID], e)
		indegree[e.Target.NodeID]++
	}
	indegreeSnapshot := make(map[string]int, len(indegree))
	maps.Copy(indegreeSnapshot, indegree)

	// Preserve deterministic order by using node declaration order as tie-breaker.
	nodeIndex := make(map[string]int, len(wf.Nodes))
	for i, n := range wf.Nodes {
		nodeIndex[n.ID] = i
	}
	queue := make([]string, 0, len(wf.Nodes))
	for _, n := range wf.Nodes {
		if indegree[n.ID] == 0 {
			queue = append(queue, n.ID)
		}
	}

	order := make([]string, 0, len(wf.Nodes))
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		order = append(order, id)
		for _, e := range outgoing[id] {
			indegree[e.Target.NodeID]--
			if indegree[e.Target.NodeID] == 0 {
				queue = append(queue, e.Target.NodeID)
			}
		}
		slices.SortFunc(queue, func(a, b string) int {
			if nodeIndex[a] < nodeIndex[b] {
				return -1
			}
			if nodeIndex[a] > nodeIndex[b] {
				return 1
			}
			return 0
		})
	}

	plan := &Plan{
		WorkflowID: wf.ID,
		NodeOrder:  order,
		Incoming:   incoming,
		Outgoing:   outgoing,
		Indegree:   make(map[string]int, len(indegreeSnapshot)),
	}
	maps.Copy(plan.Indegree, indegreeSnapshot)
	return plan, diags
}

func hasError(diags []Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == DiagnosticSeverityError {
			return true
		}
	}
	return false
}
