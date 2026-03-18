package flow

import (
	"fmt"
	"slices"
	"strings"
)

type DiagnosticSeverity string

const (
	DiagnosticSeverityError   DiagnosticSeverity = "error"
	DiagnosticSeverityWarning DiagnosticSeverity = "warning"
)

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
	diags := make([]Diagnostic, 0)
	add := func(sev DiagnosticSeverity, code, msg, path string) {
		diags = append(diags, Diagnostic{
			Severity: sev,
			Code:     code,
			Message:  msg,
			Path:     path,
		})
	}
	validTriggerTypes := []TriggerType{
		TriggerTypeManual,
		TriggerTypeSchedule,
		TriggerTypeWebhook,
		TriggerTypeEvent,
	}
	validNodeKinds := []NodeKind{
		NodeKindAction,
		NodeKindLogic,
		NodeKindData,
	}
	validOnError := []ErrorStrategy{
		"",
		ErrorStrategyFail,
		ErrorStrategyContinue,
	}
	validBackoff := []BackoffStrategy{
		BackoffNone,
		BackoffFixed,
		BackoffExponential,
	}

	if strings.TrimSpace(wf.ID) == "" {
		add(DiagnosticSeverityError, "workflow.id.required", "workflow id is required", "workflow.id")
	}
	if strings.TrimSpace(wf.Name) == "" {
		add(DiagnosticSeverityError, "workflow.name.required", "workflow name is required", "workflow.name")
	}
	if !slices.Contains(validTriggerTypes, wf.Trigger.Type) {
		add(
			DiagnosticSeverityError,
			"workflow.trigger.invalid_type",
			fmt.Sprintf("trigger type must be one of %q", validTriggerTypes),
			"workflow.trigger.type",
		)
	}
	switch wf.Trigger.Type {
	case TriggerTypeSchedule:
		if wf.Trigger.Schedule == nil || strings.TrimSpace(wf.Trigger.Schedule.Cron) == "" {
			add(
				DiagnosticSeverityError,
				"workflow.trigger.schedule.required",
				"schedule trigger requires schedule.cron",
				"workflow.trigger.schedule.cron",
			)
		}
	case TriggerTypeWebhook:
		if wf.Trigger.Webhook == nil {
			add(
				DiagnosticSeverityError,
				"workflow.trigger.webhook.required",
				"webhook trigger requires webhook config",
				"workflow.trigger.webhook",
			)
		} else {
			if strings.TrimSpace(wf.Trigger.Webhook.Method) == "" {
				add(
					DiagnosticSeverityError,
					"workflow.trigger.webhook.method.required",
					"webhook trigger requires method",
					"workflow.trigger.webhook.method",
				)
			}
			if strings.TrimSpace(wf.Trigger.Webhook.Path) == "" {
				add(
					DiagnosticSeverityError,
					"workflow.trigger.webhook.path.required",
					"webhook trigger requires path",
					"workflow.trigger.webhook.path",
				)
			}
		}
	case TriggerTypeEvent:
		if wf.Trigger.Event == nil || strings.TrimSpace(wf.Trigger.Event.Name) == "" {
			add(
				DiagnosticSeverityError,
				"workflow.trigger.event.required",
				"event trigger requires event.name",
				"workflow.trigger.event.name",
			)
		}
	}

	if len(wf.Nodes) == 0 {
		add(
			DiagnosticSeverityError,
			"workflow.nodes.required",
			"workflow must contain at least one node",
			"workflow.nodes",
		)
	}

	nodeByID := map[string]Node{}
	for i, n := range wf.Nodes {
		idxPath := fmt.Sprintf("workflow.nodes[%d]", i)
		if strings.TrimSpace(n.ID) == "" {
			add(DiagnosticSeverityError, "node.id.required", "node id is required", idxPath+".id")
		} else {
			if _, exists := nodeByID[n.ID]; exists {
				add(DiagnosticSeverityError, "node.id.duplicate", "node id must be unique", idxPath+".id")
			}
			nodeByID[n.ID] = n
		}
		if strings.TrimSpace(n.Name) == "" {
			add(DiagnosticSeverityError, "node.name.required", "node name is required", idxPath+".name")
		}
		if !slices.Contains(validNodeKinds, n.Kind) {
			add(
				DiagnosticSeverityError,
				"node.kind.invalid",
				fmt.Sprintf("node kind must be one of %q", validNodeKinds),
				idxPath+".kind",
			)
		}
		if strings.TrimSpace(n.Type) == "" {
			add(DiagnosticSeverityError, "node.type.required", "node type is required", idxPath+".type")
		}
		if n.Type == "tool" && strings.TrimSpace(n.Tool) == "" {
			add(DiagnosticSeverityError, "node.tool.required", "tool node requires tool name", idxPath+".tool")
		}
		if !slices.Contains(validOnError, n.Execution.OnError) {
			add(
				DiagnosticSeverityError,
				"node.execution.on_error.invalid",
				fmt.Sprintf("on_error must be one of %q", validOnError),
				idxPath+".execution.on_error",
			)
		}
		if n.Execution.Retries.Max < 0 {
			add(
				DiagnosticSeverityError,
				"node.execution.retries.invalid_max",
				"retries.max must be >= 0",
				idxPath+".execution.retries.max",
			)
		}
		if !slices.Contains(validBackoff, n.Execution.Retries.Backoff) {
			add(
				DiagnosticSeverityError,
				"node.execution.retries.invalid_backoff",
				fmt.Sprintf("retries.backoff must be one of %q", validBackoff),
				idxPath+".execution.retries.backoff",
			)
		}
		for key, binding := range n.Inputs {
			path := idxPath + ".inputs." + key
			hasExpr := strings.TrimSpace(binding.Expression) != ""
			hasLiteral := binding.Literal != nil
			if hasExpr == hasLiteral {
				add(
					DiagnosticSeverityError,
					"node.input.binding.exclusive",
					"input binding must set exactly one of expression or literal",
					path,
				)
				continue
			}
			if hasExpr && strings.Contains(binding.Expression, "${A.") {
				add(
					DiagnosticSeverityWarning,
					"node.input.expression.legacy",
					"legacy ${A.*} expression detected; use $node/$run expressions",
					path+".expression",
				)
			}
		}
	}

	edgeByID := map[string]struct{}{}
	adj := map[string][]string{}
	indegree := map[string]int{}
	for _, n := range wf.Nodes {
		indegree[n.ID] = 0
	}
	for i, e := range wf.Edges {
		idxPath := fmt.Sprintf("workflow.edges[%d]", i)
		if strings.TrimSpace(e.ID) != "" {
			if _, exists := edgeByID[e.ID]; exists {
				add(DiagnosticSeverityError, "edge.id.duplicate", "edge id must be unique", idxPath+".id")
			}
			edgeByID[e.ID] = struct{}{}
		}
		if strings.TrimSpace(e.Source.NodeID) == "" {
			add(DiagnosticSeverityError, "edge.source.node_id.required", "edge source node_id is required", idxPath+".source.node_id")
		} else if _, ok := nodeByID[e.Source.NodeID]; !ok {
			add(DiagnosticSeverityError, "edge.source.node_id.unknown", "edge source node_id does not exist", idxPath+".source.node_id")
		}
		if strings.TrimSpace(e.Target.NodeID) == "" {
			add(DiagnosticSeverityError, "edge.target.node_id.required", "edge target node_id is required", idxPath+".target.node_id")
		} else if _, ok := nodeByID[e.Target.NodeID]; !ok {
			add(DiagnosticSeverityError, "edge.target.node_id.unknown", "edge target node_id does not exist", idxPath+".target.node_id")
		}
		if strings.TrimSpace(e.Source.Port) == "" {
			add(DiagnosticSeverityError, "edge.source.port.required", "edge source port is required", idxPath+".source.port")
		}
		if strings.TrimSpace(e.Target.Port) == "" {
			add(DiagnosticSeverityError, "edge.target.port.required", "edge target port is required", idxPath+".target.port")
		}
		if e.Source.NodeID != "" && e.Source.NodeID == e.Target.NodeID {
			add(DiagnosticSeverityError, "edge.self_loop", "self-loop edges are not supported", idxPath)
		}
		for j, m := range e.Mapping {
			mp := fmt.Sprintf("%s.mapping[%d]", idxPath, j)
			if strings.TrimSpace(m.From) == "" {
				add(DiagnosticSeverityError, "edge.mapping.from.required", "mapping.from is required", mp+".from")
			}
			if strings.TrimSpace(m.To) == "" {
				add(DiagnosticSeverityError, "edge.mapping.to.required", "mapping.to is required", mp+".to")
			}
		}
		srcKnown := e.Source.NodeID != "" && nodeByID[e.Source.NodeID].ID != ""
		dstKnown := e.Target.NodeID != "" && nodeByID[e.Target.NodeID].ID != ""
		if srcKnown && dstKnown {
			adj[e.Source.NodeID] = append(adj[e.Source.NodeID], e.Target.NodeID)
			indegree[e.Target.NodeID]++
		}
	}

	if !hasError(diags) {
		remaining := map[string]int{}
		for id, d := range indegree {
			remaining[id] = d
		}
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
			add(
				DiagnosticSeverityError,
				"workflow.graph.cycle",
				"workflow graph contains at least one cycle",
				"workflow.edges",
			)
		}
	}

	return diags
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
	for id, degree := range indegree {
		indegreeSnapshot[id] = degree
	}

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
	for id, degree := range indegreeSnapshot {
		plan.Indegree[id] = degree
	}
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
