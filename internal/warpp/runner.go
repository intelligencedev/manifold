package warpp

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"time"

	"manifold/internal/tools"
)

// Runner executes the WARPP protocol using an in-process workflow registry and the
// configured tool registry. It performs minimal attribute inference and pruning.
type Runner struct {
	Workflows *Registry
	Tools     tools.Registry
}

// DetectIntent selects a workflow intent for the utterance based on keyword
// matches. If multiple match, the first is chosen. If none match, picks any.
func (r *Runner) DetectIntent(ctx context.Context, utter string) string {
	u := strings.ToLower(utter)
	best := ""
	bestScore := -1
	for _, w := range r.Workflows.All() {
		score := 0
		for _, kw := range w.Keywords {
			if strings.Contains(u, strings.ToLower(kw)) {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			best = w.Intent
		}
	}
	if best == "" {
		best = "cli_echo"
	}
	return best
}

// Personalize infers basic attributes from the utterance and tool outputs.
func (r *Runner) Personalize(ctx context.Context, w Workflow, A Attrs) (Workflow, tools.Registry, Attrs, error) {
	if A == nil {
		A = Attrs{}
	}
	// Basic attributes: prefer an explicit "utter", then fall back to
	// "echo" (used by some callers), then to an existing "query".
	if _, ok := A["utter"]; !ok {
		if v, ok2 := A["echo"]; ok2 {
			A["utter"] = v
		} else if v2, ok3 := A["query"]; ok3 {
			A["utter"] = v2
		}
	}
	A["query"] = A["utter"]
	A["os"] = runtime.GOOS

	// TRIM: static pruning by guards and record referenced tools
	keepTools := map[string]bool{}
	out := Workflow{Intent: w.Intent, Description: w.Description, Keywords: w.Keywords}
	for _, s := range w.Steps {
		if s.Guard != "" && !EvalGuard(s.Guard, A) {
			continue
		}
		step := s
		if step.Tool != nil {
			keepTools[step.Tool.Name] = true
		}
		out.Steps = append(out.Steps, step)
	}

	// Filter tools registry to only referenced tools
	// We return the original Tools; enforcement is done at Execute with the
	// keepTools allowlist.
	return out, r.Tools, A, nil
}

// Execute runs the personalized workflow, performing simple template substitution
// on string arguments of the form ${A.key} using attributes.
// StepPublisher is a function called when a step has a result to publish.
// It should be best-effort: failures will be logged by callers but do not
// necessarily abort workflow execution.
type StepPublisher func(ctx context.Context, stepID string, payload []byte) error

func (r *Runner) Execute(ctx context.Context, w Workflow, allowed map[string]bool, A Attrs, publish StepPublisher) (string, error) {
	summary, _, err := r.executeInternal(ctx, w, allowed, A, publish, false)
	return summary, err
}

// ExecuteWithTrace mirrors Execute but also returns a per-step trace capturing
// rendered arguments and payloads. The trace slice is ordered by execution
// (respecting DAG topology where applicable).
func (r *Runner) ExecuteWithTrace(ctx context.Context, w Workflow, allowed map[string]bool, A Attrs, publish StepPublisher) (string, []StepTrace, error) {
	return r.executeInternal(ctx, w, allowed, A, publish, true)
}

func (r *Runner) executeInternal(ctx context.Context, w Workflow, allowed map[string]bool, A Attrs, publish StepPublisher, collect bool) (string, []StepTrace, error) {
	var summary strings.Builder
	fmt.Fprintf(&summary, "WARPP: executing intent %s\n", w.Intent)
	if A == nil {
		A = Attrs{}
	}
	if A["sources"] == nil {
		A["sources"] = []map[string]string{}
	}
	var traces []StepTrace
	if collect {
		traces = make([]StepTrace, 0, len(w.Steps))
	}
	type nodeResult struct {
		id          string
		topo        int
		stepIdx     int
		payload     []byte
		delta       Attrs
		publishIt   bool
		publishMode string
		text        string
		args        map[string]any
		status      string
		err         string
	}
	cloneArgs := func(args map[string]any) map[string]any {
		if args == nil {
			return nil
		}
		cloned := cloneAttrs(Attrs(args))
		return map[string]any(cloned)
	}
	cloneDelta := func(delta Attrs) Attrs {
		if delta == nil {
			return nil
		}
		return cloneAttrs(delta)
	}
	clonePayload := func(payload []byte) json.RawMessage {
		if len(payload) == 0 {
			return nil
		}
		cp := make([]byte, len(payload))
		copy(cp, payload)
		return json.RawMessage(cp)
	}
	makeTrace := func(step Step, status string, args map[string]any, delta Attrs, payload []byte) StepTrace {
		trace := StepTrace{StepID: step.ID, Text: step.Text, Status: status}
		if v := cloneArgs(args); v != nil {
			trace.RenderedArgs = v
		}
		if v := cloneDelta(delta); v != nil {
			trace.Delta = v
		}
		if v := clonePayload(payload); v != nil {
			trace.Payload = v
		}
		return trace
	}
	makeTraceFromResult := func(res nodeResult) StepTrace {
		status := res.status
		if status == "" {
			status = "completed"
		}
		trace := StepTrace{StepID: res.id, Text: res.text, Status: status}
		if v := cloneArgs(res.args); v != nil {
			trace.RenderedArgs = v
		}
		if v := cloneDelta(res.delta); v != nil {
			trace.Delta = v
		}
		if v := clonePayload(res.payload); v != nil {
			trace.Payload = v
		}
		if res.err != "" {
			trace.Error = res.err
		}
		return trace
	}
	// If no DAG edges are present, retain sequential behavior for compatibility.
	dagPresent := false
	for _, s := range w.Steps {
		if len(s.DependsOn) > 0 {
			dagPresent = true
			break
		}
	}
	if !dagPresent {
		// Fallback to legacy sequential execution
		steps := 0
		for _, s := range w.Steps {
			if s.Tool == nil {
				if collect {
					traces = append(traces, StepTrace{StepID: s.ID, Text: s.Text, Status: "noop"})
				}
				continue
			}
			if s.Guard != "" && !EvalGuard(s.Guard, A) {
				if collect {
					traces = append(traces, StepTrace{StepID: s.ID, Text: s.Text, Status: "skipped"})
				}
				continue
			}
			if !allowed[s.Tool.Name] {
				err := fmt.Errorf("tool not permitted: %s", s.Tool.Name)
				if collect {
					traces = append(traces, StepTrace{StepID: s.ID, Text: s.Text, Status: "error", Error: err.Error()})
				}
				return "", traces, err
			}
			ai := cloneAttrs(A)
			payload, delta, args, err := r.runStep(ctx, s, ai)
			if err != nil {
				if collect {
					traces = append(traces, StepTrace{StepID: s.ID, Text: s.Text, Status: "error", Error: err.Error(), RenderedArgs: cloneArgs(args)})
				}
				return "", traces, err
			}
			mergeDelta(A, delta, 0, 0, nil)
			steps++
			fmt.Fprintf(&summary, "- %s\n", s.Text)
			if collect {
				traces = append(traces, makeTrace(s, "completed", args, delta, payload))
			}
			if s.PublishResult && publish != nil {
				if perr := publish(ctx, s.ID, payload); perr != nil {
					fmt.Printf("step result publish failed (step=%s): %v\n", s.ID, perr)
				}
			}
		}
		fmt.Fprintf(&summary, "\nObjective complete. (steps=%d).\n", steps)
		return summary.String(), traces, nil
	}

	// DAG scheduling path

	// Build indices
	idToStep := make(map[string]Step, len(w.Steps))
	stepIndex := make(map[string]int, len(w.Steps))
	indegree := make(map[string]int, len(w.Steps))
	adj := make(map[string][]string, len(w.Steps))
	for i, s := range w.Steps {
		idToStep[s.ID] = s
		stepIndex[s.ID] = i
		indegree[s.ID] = 0
	}
	for _, s := range w.Steps {
		for _, dep := range s.DependsOn {
			indegree[s.ID]++
			adj[dep] = append(adj[dep], s.ID)
		}
	}
	// Deterministic ready set: start with all indegree 0 sorted by step order
	ready := make([]string, 0)
	for id, d := range indegree {
		if d == 0 {
			ready = append(ready, id)
		}
	}
	sortByStepIndex(ready, stepIndex)

	// Concurrency limit: 0 means unlimited
	limit := w.MaxConcurrency

	// Topological index assignment
	topoIndex := 0
	completed := 0
	total := len(w.Steps)
	// Provenance for deterministic merges
	prov := make(map[string][2]int)

	// Channels for scheduling
	type queued struct {
		id   string
		topo int
	}
	queueCh := make(chan queued)
	resCh := make(chan nodeResult)
	errCh := make(chan error, 1)
	doneCh := make(chan struct{})

	// Semaphore (nil when unlimited)
	var sem chan struct{}
	if limit > 0 {
		sem = make(chan struct{}, limit)
	}

	// Context cancellation handling for fail-fast
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Publisher buffering for topo mode
	var topoBuffer []nodeResult

	// Scheduler goroutine: feeds ready nodes to workers
	go func() {
		defer close(doneCh)
		for len(ready) > 0 || completed < total {
			// admit as many as possible under concurrency limit
			for len(ready) > 0 && (limit == 0 || len(sem) < limit) {
				id := ready[0]
				ready = ready[1:]
				if sem != nil {
					sem <- struct{}{}
				}
				qi := topoIndex
				queueCh <- queued{id: id, topo: qi}
				topoIndex++
			}
			select {
			case res := <-resCh:
				// Merge delta deterministically using provenance
				mergeDeltaWithProv(A, res.delta, res.topo, res.stepIdx, prov)
				completed++
				fmt.Fprintf(&summary, "- %s\n", res.text)
				if collect {
					traces = append(traces, makeTraceFromResult(res))
				}
				if res.publishIt && publish != nil {
					if res.publishMode == "topo" {
						topoBuffer = append(topoBuffer, res)
					} else {
						_ = publish(ctx, res.id, res.payload) // best-effort
					}
				}
				// Update dependents
				for _, m := range adj[res.id] {
					indegree[m]--
					if indegree[m] == 0 {
						ready = append(ready, m)
					}
				}
				sortByStepIndex(ready, stepIndex)
			case err := <-errCh:
				// fail-fast
				cancel()
				_ = err // not used here; handled by return below
				return
			}
		}
	}()

	// Worker launcher
	go func() {
		for q := range queueCh {
			s := idToStep[q.id]
			// Skip nodes with false guards: treat as no-op success
			if s.Guard != "" && !EvalGuard(s.Guard, A) {
				if sem != nil {
					<-sem
				}
				resCh <- nodeResult{id: s.ID, topo: q.topo, stepIdx: stepIndex[s.ID], delta: Attrs{}, payload: nil, publishIt: false, text: s.Text, status: "skipped"}
				continue
			}
			// Launch worker
			go func(st Step, topo int) {
				defer func() {
					if sem != nil {
						<-sem
					}
				}()
				// Disallow missing or unauthorized tools
				if st.Tool == nil {
					resCh <- nodeResult{id: st.ID, topo: topo, stepIdx: stepIndex[st.ID], delta: Attrs{}, payload: nil, publishIt: false, text: st.Text, status: "noop"}
					return
				}
				if !allowed[st.Tool.Name] {
					errCh <- fmt.Errorf("tool not permitted: %s", st.Tool.Name)
					return
				}
				// Snapshot A
				ai := cloneAttrs(A)
				// Per-step timeout
				cctx := ctx
				if d := parseDurationSafe(st.Timeout); d > 0 {
					var cancel2 context.CancelFunc
					cctx, cancel2 = context.WithTimeout(ctx, d)
					defer cancel2()
				}
				payload, delta, args, err := r.runStep(cctx, st, ai)
				if err != nil {
					if st.ContinueOnError {
						// Soft-fail: report as completion without merge
						resCh <- nodeResult{id: st.ID, topo: topo, stepIdx: stepIndex[st.ID], delta: Attrs{}, payload: payload, publishIt: false, text: st.Text, status: "error", err: err.Error(), args: args}
						return
					}
					errCh <- err
					return
				}
				resCh <- nodeResult{id: st.ID, topo: topo, stepIdx: stepIndex[st.ID], delta: delta, payload: payload, publishIt: st.PublishResult, publishMode: st.PublishMode, text: st.Text, args: args, status: "completed"}
			}(s, q.topo)
		}
	}()

	// Wait for scheduler to complete or error
	<-doneCh
	select {
	case err := <-errCh:
		return "", traces, err
	default:
	}

	// Flush topo-ordered publications
	if len(topoBuffer) > 0 && publish != nil {
		sort.Slice(topoBuffer, func(i, j int) bool {
			if topoBuffer[i].topo == topoBuffer[j].topo {
				return topoBuffer[i].stepIdx < topoBuffer[j].stepIdx
			}
			return topoBuffer[i].topo < topoBuffer[j].topo
		})
		for _, res := range topoBuffer {
			_ = publish(ctx, res.id, res.payload)
		}
	}
	fmt.Fprintf(&summary, "\nObjective complete. (steps=%d).\n", completed)
	return summary.String(), traces, nil
}

// renderArgs replaces ${A.key} placeholders in string values within args.
func renderArgs(args map[string]any, A Attrs) map[string]any {
	if args == nil {
		return nil
	}
	out := make(map[string]any, len(args))
	for k, v := range args {
		switch vv := v.(type) {
		case string:
			out[k] = substitute(vv, A)
		case []any:
			out[k] = renderSlice(vv, A)
		case map[string]any:
			out[k] = renderArgs(vv, A)
		default:
			out[k] = v
		}
	}
	return out
}

func renderSlice(xs []any, A Attrs) []any {
	out := make([]any, len(xs))
	for i, v := range xs {
		switch vv := v.(type) {
		case string:
			out[i] = substitute(vv, A)
		case []any:
			out[i] = renderSlice(vv, A)
		case map[string]any:
			out[i] = renderArgs(vv, A)
		default:
			out[i] = v
		}
	}
	return out
}

func substitute(s string, A Attrs) string {
	// ${A.key}
	for {
		start := strings.Index(s, "${A.")
		if start == -1 {
			break
		}
		end := strings.Index(s[start:], "}")
		if end == -1 {
			break
		}
		end += start
		inner := s[start+2 : end]
		key := strings.TrimPrefix(inner, "A.")
		val := fmt.Sprintf("%v", A[key])
		s = s[:start] + val + s[end+1:]
	}
	return s
}

// --- helpers for DAG execution ---

func cloneAttrs(A Attrs) Attrs {
	out := Attrs{}
	for k, v := range A {
		out[k] = v
	}
	return out
}

func parseDurationSafe(s string) time.Duration {
	if s == "" {
		return 0
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0
	}
	return d
}

func sortByStepIndex(ids []string, idx map[string]int) {
	sort.Slice(ids, func(i, j int) bool { return idx[ids[i]] < idx[ids[j]] })
}

// (max helper removed; concurrency handled via optional semaphore)

// mergeDelta merges a delta into A with simple rules (used for sequential fallback).
func mergeDelta(A Attrs, delta Attrs, topo, stepIdx int, prov map[string][2]int) {
	mergeDeltaWithProv(A, delta, topo, stepIdx, prov)
}

// mergeDeltaWithProv merges respecting deterministic precedence using provenance per key.
func mergeDeltaWithProv(A Attrs, delta Attrs, topo, stepIdx int, prov map[string][2]int) {
	if delta == nil {
		return
	}
	for k, v := range delta {
		switch k {
		case "payloads":
			// append
			var exist []string
			if xs, ok := A[k].([]string); ok {
				exist = xs
			}
			exist = append(exist, v.([]string)...)
			A[k] = exist
		case "sources":
			// append slice of maps
			var exist []map[string]string
			if xs, ok := A[k].([]map[string]string); ok {
				exist = xs
			}
			exist = append(exist, v.([]map[string]string)...)
			A[k] = exist
		case "urls":
			// union
			seen := map[string]bool{}
			var exist []string
			if xs, ok := A[k].([]string); ok {
				for _, u := range xs {
					seen[u] = true
				}
				exist = append(exist, xs...)
			}
			for _, u := range v.([]string) {
				if !seen[u] {
					seen[u] = true
					exist = append(exist, u)
				}
			}
			A[k] = exist
		default:
			// scalar or map fallback with precedence
			if prov != nil {
				if p, ok := prov[k]; ok {
					if p[0] > topo || (p[0] == topo && p[1] > stepIdx) {
						// existing has higher precedence; skip
						continue
					}
				}
				prov[k] = [2]int{topo, stepIdx}
			}
			A[k] = v
		}
	}
}

// runStep executes a step against Tools and returns payload, attribute delta,
// and the rendered argument map used for invocation.
func (r *Runner) runStep(ctx context.Context, s Step, A Attrs) ([]byte, Attrs, map[string]any, error) {
	if s.Tool == nil {
		return nil, nil, nil, nil
	}
	args := renderArgs(s.Tool.Args, A)
	raw, _ := json.Marshal(args)
	payload, err := r.Tools.Dispatch(ctx, s.Tool.Name, raw)
	if err != nil {
		return nil, nil, args, err
	}
	delta := Attrs{}
	ps := string(payload)
	// Common payload recording
	delta["last_payload"] = ps
	delta["payloads"] = []string{ps}

	switch s.Tool.Name {
	case "web_search":
		var resp struct {
			OK      bool `json:"ok"`
			Results []struct {
				URL   string `json:"url"`
				Title string `json:"title"`
			} `json:"results"`
		}
		_ = json.Unmarshal(payload, &resp)
		if len(resp.Results) > 0 {
			delta["first_url"] = resp.Results[0].URL
		}
		if len(resp.Results) > 1 {
			delta["second_url"] = resp.Results[1].URL
		}
		// urls slice
		urls := make([]string, 0, len(resp.Results))
		for _, r := range resp.Results {
			urls = append(urls, r.URL)
		}
		delta["urls"] = urls
	case "web_fetch":
		var resp struct {
			OK       bool   `json:"ok"`
			Title    string `json:"title"`
			Markdown string `json:"markdown"`
			FinalURL string `json:"final_url"`
			InputURL string `json:"input_url"`
			UsedRead bool   `json:"used_readable"`
		}
		_ = json.Unmarshal(payload, &resp)
		title := strings.TrimSpace(resp.Title)
		if title == "" {
			title = "Research Report"
		}
		srcURL := resp.FinalURL
		if srcURL == "" {
			srcURL = resp.InputURL
		}
		delta["sources"] = []map[string]string{{"title": title, "url": srcURL, "markdown": resp.Markdown}}
	case "llm_transform":
		var resp struct {
			OK     bool   `json:"ok"`
			Output string `json:"output"`
		}
		_ = json.Unmarshal(payload, &resp)
		if resp.Output != "" {
			delta["llm_output"] = resp.Output
			delta["report_md"] = resp.Output
		}
	case "utility_textbox":
		var resp struct {
			Text       string `json:"text"`
			OutputAttr string `json:"output_attr"`
		}
		_ = json.Unmarshal(payload, &resp)
		attrKey := strings.TrimSpace(resp.OutputAttr)
		if attrKey == "" {
			attrKey = s.ID + "_text"
		}
		delta[attrKey] = resp.Text
	case "write_file":
		// If not already built, synthesize a report from collected sources (from snapshot A)
		rep, _ := A["report_md"].(string)
		if rep == "" {
			q := fmt.Sprintf("%v", A["query"])
			if q == "" {
				q = fmt.Sprintf("%v", A["utter"])
			}
			title := q
			var b strings.Builder
			fmt.Fprintf(&b, "# Deep Research Report: %s\n\n", title)
			sources, _ := A["sources"].([]map[string]string)
			if len(sources) > 0 {
				b.WriteString("## Sources\n\n")
				for i, s := range sources {
					fmt.Fprintf(&b, "%d. %s\n   %s\n\n", i+1, s["url"], s["title"])
				}
			}
			b.WriteString("## Executive Summary\n\n")
			b.WriteString("This report synthesizes information from multiple web sources.\n\n")
			b.WriteString("## Detailed Findings\n\n")
			for _, s := range sources {
				if t := strings.TrimSpace(s["title"]); t != "" {
					fmt.Fprintf(&b, "### %s\n\n", t)
				}
				if m := s["markdown"]; m != "" {
					b.WriteString(m)
					b.WriteString("\n\n")
				}
			}
			rep = b.String()
			delta["report_md"] = rep
			// re-render args with updated content
			args = renderArgs(s.Tool.Args, mergePreview(A, delta))
			raw, _ = json.Marshal(args)
			if _, err := r.Tools.Dispatch(ctx, s.Tool.Name, raw); err != nil {
				return nil, nil, args, err
			}
		}
	}
	return payload, delta, args, nil
}

func mergePreview(A Attrs, delta Attrs) Attrs {
	out := cloneAttrs(A)
	for k, v := range delta {
		out[k] = v
	}
	return out
}
