package flow

import "testing"

func TestValidateWorkflow(t *testing.T) {
	t.Parallel()

	wf := validWorkflow()
	diags := ValidateWorkflow(wf)
	if countSeverity(diags, DiagnosticSeverityError) != 0 {
		t.Fatalf("expected no errors, got: %#v", diags)
	}

	wf.Nodes[0].Inputs = map[string]InputBinding{
		"query": {
			Literal:    "news",
			Expression: "={{$run.input.query}}",
		},
	}
	diags = ValidateWorkflow(wf)
	if !hasCode(diags, "node.input.binding.exclusive") {
		t.Fatalf("expected exclusive binding error, got: %#v", diags)
	}
}

func TestCompileWorkflow(t *testing.T) {
	t.Parallel()

	t.Run("compiles valid workflow in topological order", func(t *testing.T) {
		t.Parallel()

		plan, diags := CompileWorkflow(validWorkflow())
		if countSeverity(diags, DiagnosticSeverityError) != 0 {
			t.Fatalf("expected no errors, got: %#v", diags)
		}
		if plan == nil {
			t.Fatal("expected non-nil plan")
		}
		want := []string{"search", "fetch", "summarize"}
		if len(plan.NodeOrder) != len(want) {
			t.Fatalf("unexpected node order length: got=%d want=%d", len(plan.NodeOrder), len(want))
		}
		for i := range want {
			if plan.NodeOrder[i] != want[i] {
				t.Fatalf("unexpected node order at %d: got=%q want=%q", i, plan.NodeOrder[i], want[i])
			}
		}
	})

	t.Run("fails for unknown edge target", func(t *testing.T) {
		t.Parallel()

		wf := validWorkflow()
		wf.Edges[1].Target.NodeID = "missing"
		plan, diags := CompileWorkflow(wf)
		if plan != nil {
			t.Fatal("expected nil plan")
		}
		if !hasCode(diags, "edge.target.node_id.unknown") {
			t.Fatalf("expected unknown target diagnostic, got: %#v", diags)
		}
	})

	t.Run("fails for cycle", func(t *testing.T) {
		t.Parallel()

		wf := validWorkflow()
		wf.Edges = append(wf.Edges, Edge{
			Source: PortRef{NodeID: "summarize", Port: "done"},
			Target: PortRef{NodeID: "search", Port: "input"},
		})
		plan, diags := CompileWorkflow(wf)
		if plan != nil {
			t.Fatal("expected nil plan")
		}
		if !hasCode(diags, "workflow.graph.cycle") {
			t.Fatalf("expected cycle diagnostic, got: %#v", diags)
		}
	})

	t.Run("keeps warnings but still compiles", func(t *testing.T) {
		t.Parallel()

		wf := validWorkflow()
		wf.Nodes[2].Inputs["text"] = InputBinding{
			Expression: "${A.fetch.first_source.markdown}",
		}
		plan, diags := CompileWorkflow(wf)
		if plan == nil {
			t.Fatal("expected non-nil plan")
		}
		if !hasCode(diags, "node.input.expression.legacy") {
			t.Fatalf("expected legacy expression warning, got: %#v", diags)
		}
	})
}

func validWorkflow() Workflow {
	return Workflow{
		ID:   "flow_research",
		Name: "Research Flow",
		Trigger: Trigger{
			Type: TriggerTypeManual,
		},
		Nodes: []Node{
			{
				ID:   "search",
				Name: "Search",
				Kind: NodeKindAction,
				Type: "tool",
				Tool: "web_search",
				Inputs: map[string]InputBinding{
					"query": {Expression: "={{$run.input.query}}"},
				},
			},
			{
				ID:   "fetch",
				Name: "Fetch",
				Kind: NodeKindAction,
				Type: "tool",
				Tool: "web_fetch",
				Inputs: map[string]InputBinding{
					"urls": {Expression: "={{$node.search.output.urls}}"},
				},
			},
			{
				ID:   "summarize",
				Name: "Summarize",
				Kind: NodeKindAction,
				Type: "tool",
				Tool: "llm_transform",
				Inputs: map[string]InputBinding{
					"text": {Expression: "={{$node.fetch.output.markdown}}"},
				},
				Execution: NodeExecution{
					Retries: RetryPolicy{
						Max:     2,
						Backoff: BackoffExponential,
					},
					OnError: ErrorStrategyFail,
				},
			},
		},
		Edges: []Edge{
			{
				ID:     "edge_search_fetch",
				Source: PortRef{NodeID: "search", Port: "result"},
				Target: PortRef{NodeID: "fetch", Port: "input"},
			},
			{
				ID:     "edge_fetch_summarize",
				Source: PortRef{NodeID: "fetch", Port: "result"},
				Target: PortRef{NodeID: "summarize", Port: "input"},
			},
		},
	}
}

func hasCode(diags []Diagnostic, code string) bool {
	for _, d := range diags {
		if d.Code == code {
			return true
		}
	}
	return false
}

func countSeverity(diags []Diagnostic, severity DiagnosticSeverity) int {
	count := 0
	for _, d := range diags {
		if d.Severity == severity {
			count++
		}
	}
	return count
}
