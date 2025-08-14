package warpp

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"

	"singularityio/internal/tools"
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
	// Basic attributes: echo utterance and use as a query
	if _, ok := A["utter"]; !ok {
		A["utter"] = A["query"]
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
func (r *Runner) Execute(ctx context.Context, w Workflow, allowed map[string]bool, A Attrs) (string, error) {
	var summary strings.Builder
	fmt.Fprintf(&summary, "WARPP: executing intent %s\n", w.Intent)
	steps := 0
	if A["sources"] == nil {
		A["sources"] = []map[string]string{}
	}
	for _, s := range w.Steps {
		if s.Tool == nil {
			continue
		}
		if !allowed[s.Tool.Name] {
			return "", fmt.Errorf("tool not permitted: %s", s.Tool.Name)
		}
		args := renderArgs(s.Tool.Args, A)
		raw, _ := json.Marshal(args)
		payload, err := r.Tools.Dispatch(ctx, s.Tool.Name, raw)
		if err != nil {
			return "", err
		}
		// record payloads in attributes
		A["last_payload"] = string(payload)
		if ps, ok := A["payloads"].([]string); ok {
			A["payloads"] = append(ps, string(payload))
		} else {
			A["payloads"] = []string{string(payload)}
		}
		steps++
		fmt.Fprintf(&summary, "- %s\n", s.Text)
		// Opportunistically capture first_url from web_search result for later steps
		if s.Tool.Name == "web_search" {
			var resp struct {
				OK      bool `json:"ok"`
				Results []struct {
					URL   string `json:"url"`
					Title string `json:"title"`
				} `json:"results"`
			}
			_ = json.Unmarshal(payload, &resp)
			if len(resp.Results) > 0 {
				A["first_url"] = resp.Results[0].URL
			}
			if len(resp.Results) > 1 {
				A["second_url"] = resp.Results[1].URL
			}
			// store urls slice too
			urls := make([]string, 0, len(resp.Results))
			for _, r := range resp.Results {
				urls = append(urls, r.URL)
			}
			A["urls"] = urls
		}
		if s.Tool.Name == "web_fetch" {
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
			sources := A["sources"].([]map[string]string)
			sources = append(sources, map[string]string{"title": title, "url": srcURL, "markdown": resp.Markdown})
			A["sources"] = sources
		}
		if s.Tool.Name == "llm_transform" {
			var resp struct {
				OK     bool   `json:"ok"`
				Output string `json:"output"`
			}
			_ = json.Unmarshal(payload, &resp)
			if resp.Output != "" {
				A["llm_output"] = resp.Output
				A["report_md"] = resp.Output
			}
		}
		if s.Tool.Name == "write_file" {
			// If not already built, synthesize a report from collected sources
			rep, _ := A["report_md"].(string)
			if rep == "" {
				q := fmt.Sprintf("%v", A["query"])
				if q == "" {
					q = fmt.Sprintf("%v", A["utter"])
				}
				title := q
				var b strings.Builder
				fmt.Fprintf(&b, "# Deep Research Report: %s\n\n", title)
				// Sources
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
				A["report_md"] = b.String()
			}
			// Re-dispatch write_file with the potentially updated content
			args = renderArgs(s.Tool.Args, A)
			raw, _ = json.Marshal(args)
			if _, err := r.Tools.Dispatch(ctx, s.Tool.Name, raw); err != nil {
				return "", err
			}
		}
	}
	fmt.Fprintf(&summary, "\nObjective complete: report written to report.md (steps=%d).\n", steps)
	return summary.String(), nil
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
