package warpp

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"sort"
	"strconv"
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
		if s.Guard != "" && !safeEvalGuard(s.Guard, A) {
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

	// Preprocess workflow to emulate UI convenience wiring:
	// - Rewrite short aliases like ${A.first_url} to explicit producer-scoped
	//   placeholders ${A.<stepID>.first_url} when a prior step is known to
	//   produce that alias.
	// - Normalize write-file paths that are absolute into a safe relative tmp/ path.
	w = preprocessWorkflow(w, A)
	if A["sources"] == nil {
		A["sources"] = []map[string]string{}
	} else {
		A["sources"] = toSourceSlice(A["sources"]) // normalize shape
	}
	if A["urls"] != nil {
		A["urls"] = toStringSlice(A["urls"]) // normalize shape
	}
	if A["payloads"] != nil {
		A["payloads"] = toStringSlice(A["payloads"]) // normalize shape
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
			if s.Guard != "" && !safeEvalGuard(s.Guard, A) {
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
			// Record step-scoped outputs for cross-step addressing (A.<stepID>.*)
			recordStepResult(A, s.ID, payload, delta, args)
			steps++
			fmt.Fprintf(&summary, "- %s\n", s.Text)
			if collect {
				traces = append(traces, makeTrace(s, "completed", args, delta, payload))
			}
			if s.PublishResult && publish != nil {
				if perr := safePublish(ctx, s.ID, payload, publish); perr != nil {
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
				// Also record step-scoped outputs for cross-step addressing
				recordStepResult(A, res.id, res.payload, res.delta, res.args)
				completed++
				fmt.Fprintf(&summary, "- %s\n", res.text)
				if collect {
					traces = append(traces, makeTraceFromResult(res))
				}
				if res.publishIt && publish != nil {
					if res.publishMode == "topo" {
						topoBuffer = append(topoBuffer, res)
					} else {
						_ = safePublish(ctx, res.id, res.payload, publish) // best-effort
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
			if s.Guard != "" && !safeEvalGuard(s.Guard, A) {
				if sem != nil {
					<-sem
				}
				resCh <- nodeResult{id: s.ID, topo: q.topo, stepIdx: stepIndex[s.ID], delta: Attrs{}, payload: nil, publishIt: false, text: s.Text, status: "skipped"}
				continue
			}
			// Launch worker
			go func(st Step, topo int) {
				defer func() {
					if rec := recover(); rec != nil {
						if st.ContinueOnError {
							resCh <- nodeResult{id: st.ID, topo: topo, stepIdx: stepIndex[st.ID], delta: Attrs{}, payload: nil, publishIt: false, text: st.Text, status: "error", err: fmt.Sprintf("panic: %v", rec)}
						} else {
							errCh <- fmt.Errorf("panic in step %s: %v", st.ID, rec)
						}
					}
				}()
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
			_ = safePublish(ctx, res.id, res.payload, publish)
		}
	}
	fmt.Fprintf(&summary, "\nObjective complete. (steps=%d).\n", completed)
	return summary.String(), traces, nil
}

// preprocessWorkflow applies UI-like convenience wiring so headless runs
// behave like the Flow editor. It currently implements two features:
//   - auto-aliasing of per-step produced keys (e.g., first_url) to
//     fully-qualified attributes when a producing step exists in the plan;
//   - normalizing write-file targets to a safe relative path under tmp/.
func preprocessWorkflow(w Workflow, A Attrs) Workflow {
	// Build a map of produced short aliases -> producer step id by scanning
	// known tool types and their conventional outputs.
	producers := map[string]string{}
	for _, s := range w.Steps {
		if s.Tool == nil {
			continue
		}
		switch s.Tool.Name {
		case "web_search":
			// web_search produces first_url, second_url, urls
			producers["first_url"] = s.ID
			producers["second_url"] = s.ID
			producers["urls"] = s.ID
		case "web_fetch":
			// web_fetch adds sources (with url/title/markdown) and final_url
			producers["first_source"] = s.ID
			producers["final_url"] = s.ID
		case "llm_transform":
			producers["report_md"] = s.ID
			producers["llm_output"] = s.ID
		case "utility_textbox":
			// arbitrary output_attr may be specified; handled at render time
		}
	}

	// Walk steps and rewrite literal string args containing ${A.key} where
	// key is a known short alias into ${A.<step>.<key>}.
	for si := range w.Steps {
		s := &w.Steps[si]
		if s.Tool == nil || s.Tool.Args == nil {
			continue
		}
		// mutate args in-place
		for ak, av := range s.Tool.Args {
			switch tv := av.(type) {
			case string:
				newv := tv
				for short := range producers {
					placeholder := "${A." + short + "}"
					if strings.Contains(newv, placeholder) {
						newv = strings.ReplaceAll(newv, placeholder, "${A."+producers[short]+"."+short+"}")
					}
				}
				s.Tool.Args[ak] = newv
			case []any:
				// simple slice walk
				for i, it := range tv {
					if str, ok := it.(string); ok {
						newv := str
						for short := range producers {
							placeholder := "${A." + short + "}"
							if strings.Contains(newv, placeholder) {
								newv = strings.ReplaceAll(newv, placeholder, "${A."+producers[short]+"."+short+"}")
							}
						}
						tv[i] = newv
					}
				}
				s.Tool.Args[ak] = tv
			case map[string]any:
				// recurse shallow
				for k2, v2 := range tv {
					if str, ok := v2.(string); ok {
						newv := str
						for short := range producers {
							placeholder := "${A." + short + "}"
							if strings.Contains(newv, placeholder) {
								newv = strings.ReplaceAll(newv, placeholder, "${A."+producers[short]+"."+short+"}")
							}
						}
						tv[k2] = newv
					}
				}
				s.Tool.Args[ak] = tv
			}
		}

		// Normalize write_file targets: rewrite absolute paths to tmp/
		if s.Tool.Name == "write_file" {
			if p, ok := s.Tool.Args["path"].(string); ok {
				if strings.HasPrefix(p, "/") || strings.Contains(p, ":\\") {
					// rewrite to tmp/<stepID>_<basename>
					base := p
					if idx := strings.LastIndexAny(p, "/\\"); idx != -1 {
						base = p[idx+1:]
					}
					safe := "tmp/" + s.ID + "_" + base
					s.Tool.Args["path"] = safe
				}
			}
		}
	}

	return w
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
	// Supports nested paths and array indices, e.g. ${A.step1.result.0.title}
	// Unknown paths are replaced with empty string.
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
		inner := s[start+2 : end] // like "A.foo.bar"
		path := strings.TrimPrefix(inner, "A.")
		// Resolve against the full attribute map
		if v, ok := selectFromData(A, path); ok {
			// marshal complex values to JSON; scalars to string
			switch vv := v.(type) {
			case string:
				s = s[:start] + vv + s[end+1:]
			case fmt.Stringer:
				s = s[:start] + vv.String() + s[end+1:]
			default:
				// Try JSON first for maps/slices
				if b, err := json.Marshal(v); err == nil {
					s = s[:start] + string(b) + s[end+1:]
				} else {
					s = s[:start] + fmt.Sprintf("%v", v) + s[end+1:]
				}
			}
		} else {
			// replace with empty string if not found
			s = s[:start] + "" + s[end+1:]
		}
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
			A[k] = append(toStringSlice(A[k]), toStringSlice(v)...)
		case "sources":
			A[k] = append(toSourceSlice(A[k]), toSourceSlice(v)...)
		case "urls":
			A[k] = unionStrings(toStringSlice(A[k]), toStringSlice(v))
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
func (r *Runner) runStep(ctx context.Context, s Step, A Attrs) (payload []byte, delta Attrs, args map[string]any, err error) {
	// Panic guard so a bad tool or arg shape never crashes the process
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("panic in runStep step=%s: %v", s.ID, rec)
		}
	}()
	if s.Tool == nil {
		return nil, nil, nil, nil
	}
	args = renderArgs(s.Tool.Args, A)
	if args == nil {
		args = map[string]any{}
	}
	// Auto-wire: if a web_search node precedes a web_fetch node, propagate URLs
	// from A["urls"] into the web_fetch call when neither url nor urls is set.
	if s.Tool.Name == "web_fetch" {
		if _, ok := args["url"]; !ok {
			if _, ok2 := args["urls"]; !ok2 {
				if v, ok3 := A["urls"]; ok3 {
					if us := toStringSlice(v); len(us) > 0 {
						args["urls"] = us
					}
				} else if fu, ok4 := A["first_url"].(string); ok4 && fu != "" {
					// Fallback for legacy flows
					args["url"] = fu
				}
			}
		}
	}
	raw, _ := json.Marshal(args)
	payload, err = r.Tools.Dispatch(ctx, s.Tool.Name, raw)
	if err != nil {
		return nil, nil, args, err
	}
	delta = Attrs{}
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
			_, err = r.Tools.Dispatch(ctx, s.Tool.Name, raw)
			if err != nil {
				return nil, nil, args, err
			}
		}
	}

	// Generic output attribute support for all nodes
	// If args specify an "output_attr", set that key in delta from either
	// args["output_value"], a selector in args["output_from"], or default no-op.
	if oa, ok := args["output_attr"].(string); ok {
		oa = strings.TrimSpace(oa)
		if oa != "" {
			var outVal any
			if v, ok := args["output_value"]; ok {
				// Already rendered by renderArgs
				outVal = v
			} else if sel, ok := args["output_from"].(string); ok {
				sel = strings.TrimSpace(sel)
				switch {
				case sel == "payload":
					outVal = ps
				case strings.HasPrefix(sel, "json."):
					if v, ok := selectFromJSON(payload, strings.TrimPrefix(sel, "json.")); ok {
						outVal = v
					}
				case strings.HasPrefix(sel, "args."):
					if v, ok := selectFromData(args, strings.TrimPrefix(sel, "args.")); ok {
						outVal = v
					}
				case strings.HasPrefix(sel, "delta."):
					if v, ok := selectFromData(delta, strings.TrimPrefix(sel, "delta.")); ok {
						outVal = v
					}
				default:
					// unknown selector -> leave nil
				}
			}
			if outVal != nil {
				// Explicit output_attr should win for this step's delta
				delta[oa] = outVal
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

func selectFromJSON(payload []byte, path string) (any, bool) {
	if len(payload) == 0 || path == "" {
		return nil, false
	}
	var data any
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, false
	}
	return navigatePath(data, strings.Split(path, "."))
}

func selectFromData(root any, path string) (any, bool) {
	if path == "" {
		if root == nil {
			return nil, false
		}
		return root, true
	}
	return navigatePath(root, strings.Split(path, "."))
}

func navigatePath(cur any, parts []string) (any, bool) {
	for _, part := range parts {
		if part == "" {
			continue
		}
		switch node := cur.(type) {
		case map[string]any:
			var ok bool
			cur, ok = node[part]
			if !ok {
				return nil, false
			}
		case Attrs:
			var ok bool
			cur, ok = node[part]
			if !ok {
				return nil, false
			}
		case map[string]string:
			val, ok := node[part]
			if !ok {
				return nil, false
			}
			cur = val
		case string:
			// If current node is a JSON-encoded string, attempt to parse it
			// and apply the same navigation step on the parsed value.
			var pj any
			if json.Unmarshal([]byte(node), &pj) == nil {
				// Re-run this part against the parsed JSON by emulating the
				// current step logic inline.
				switch inner := pj.(type) {
				case map[string]any:
					v, ok := inner[part]
					if !ok {
						return nil, false
					}
					cur = v
				case []any:
					idx, err := strconv.Atoi(part)
					if err != nil || idx < 0 || idx >= len(inner) {
						return nil, false
					}
					cur = inner[idx]
				case []map[string]any:
					idx, err := strconv.Atoi(part)
					if err != nil || idx < 0 || idx >= len(inner) {
						return nil, false
					}
					cur = inner[idx]
				case []string:
					idx, err := strconv.Atoi(part)
					if err != nil || idx < 0 || idx >= len(inner) {
						return nil, false
					}
					cur = inner[idx]
				default:
					return nil, false
				}
				// continue to next part
				continue
			}
			return nil, false
		case []any:
			idx, err := strconv.Atoi(part)
			if err != nil || idx < 0 || idx >= len(node) {
				return nil, false
			}
			cur = node[idx]
		case []string:
			idx, err := strconv.Atoi(part)
			if err != nil || idx < 0 || idx >= len(node) {
				return nil, false
			}
			cur = node[idx]
		case []map[string]any:
			idx, err := strconv.Atoi(part)
			if err != nil || idx < 0 || idx >= len(node) {
				return nil, false
			}
			cur = node[idx]
		case []map[string]string:
			idx, err := strconv.Atoi(part)
			if err != nil || idx < 0 || idx >= len(node) {
				return nil, false
			}
			cur = node[idx]
		default:
			return nil, false
		}
	}
	if cur == nil {
		return nil, false
	}
	return cur, true
}

// --- tolerant coercion helpers and safety wrappers ---

// toStringSlice attempts to coerce various inputs into a []string.
// Accepts: []string, []any, comma/space-separated string, scalar string; otherwise returns empty slice.
func toStringSlice(v any) []string {
	if v == nil {
		return []string{}
	}
	switch t := v.(type) {
	case []string:
		return append([]string(nil), t...)
	case []any:
		out := make([]string, 0, len(t))
		for _, it := range t {
			if s, ok := it.(string); ok {
				if s != "" {
					out = append(out, s)
				}
			}
		}
		return out
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return []string{}
		}
		if strings.Contains(s, ",") {
			parts := strings.Split(s, ",")
			out := make([]string, 0, len(parts))
			for _, p := range parts {
				ps := strings.TrimSpace(p)
				if ps != "" {
					out = append(out, ps)
				}
			}
			return out
		}
		return strings.Fields(s)
	default:
		return []string{}
	}
}

// toSourceSlice coerces v into []map[string]string if possible.
func toSourceSlice(v any) []map[string]string {
	if v == nil {
		return []map[string]string{}
	}
	switch t := v.(type) {
	case []map[string]string:
		out := make([]map[string]string, 0, len(t))
		for _, m := range t {
			c := make(map[string]string, len(m))
			for k, val := range m {
				c[k] = val
			}
			out = append(out, c)
		}
		return out
	case []map[string]any:
		out := make([]map[string]string, 0, len(t))
		for _, m := range t {
			c := map[string]string{}
			for k, val := range m {
				c[k] = fmt.Sprintf("%v", val)
			}
			out = append(out, c)
		}
		return out
	case []any:
		out := make([]map[string]string, 0, len(t))
		for _, it := range t {
			switch mm := it.(type) {
			case map[string]string:
				c := make(map[string]string, len(mm))
				for k, val := range mm {
					c[k] = val
				}
				out = append(out, c)
			case map[string]any:
				c := map[string]string{}
				for k, val := range mm {
					c[k] = fmt.Sprintf("%v", val)
				}
				out = append(out, c)
			}
		}
		return out
	default:
		return []map[string]string{}
	}
}

// unionStrings returns a union of a and b preserving order and uniqueness.
func unionStrings(a, b []string) []string {
	if len(a) == 0 && len(b) == 0 {
		return []string{}
	}
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, s := range a {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, s := range b {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// safeEvalGuard evaluates a guard and returns false if it panics.
func safeEvalGuard(guard string, A Attrs) (ok bool) {
	defer func() {
		if rec := recover(); rec != nil {
			ok = false
		}
	}()
	return EvalGuard(guard, A)
}

// safePublish calls the publisher and converts panics to errors.
func safePublish(ctx context.Context, stepID string, payload []byte, publish StepPublisher) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("panic in publish step=%s: %v", stepID, rec)
		}
	}()
	if publish == nil {
		return nil
	}
	return publish(ctx, stepID, payload)
}

// recordStepResult stores per-step outputs under A[stepID] for cross-step access.
// Layout:
//
//	A[stepID] = {
//	  <delta keys...>,
//	  "delta": <map[string]any>,
//	  "args": <map[string]any>,
//	  "payload": <string>,
//	  "json": <any parsed from payload, if JSON>
//	}
func recordStepResult(A Attrs, stepID string, payload []byte, delta Attrs, args map[string]any) {
	if stepID == "" {
		return
	}
	m := map[string]any{}
	// copy delta flat keys for convenience
	for k, v := range delta {
		m[k] = v
	}
	if delta != nil {
		// store a clone to avoid accidental external mutation
		dd := make(map[string]any, len(delta))
		for k, v := range delta {
			dd[k] = v
		}
		m["delta"] = dd
	}
	if args != nil {
		aa := make(map[string]any, len(args))
		for k, v := range args {
			aa[k] = v
		}
		m["args"] = aa
	}
	if len(payload) > 0 {
		m["payload"] = string(payload)
		var pj any
		if err := json.Unmarshal(payload, &pj); err == nil {
			m["json"] = pj
		}
	}
	A[stepID] = m
}
