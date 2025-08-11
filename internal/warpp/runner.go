package warpp

import (
    "context"
    "encoding/json"
    "fmt"
    "strings"

    "gptagent/internal/tools"
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
            if strings.Contains(u, strings.ToLower(kw)) { score++ }
        }
        if score > bestScore {
            bestScore = score
            best = w.Intent
        }
    }
    if best == "" { best = "cli_echo" }
    return best
}

// Personalize infers basic attributes from the utterance and tool outputs.
func (r *Runner) Personalize(ctx context.Context, w Workflow, A Attrs) (Workflow, tools.Registry, Attrs, error) {
    if A == nil { A = Attrs{} }
    // Basic attributes: echo utterance and use as a query
    if _, ok := A["utter"]; !ok { A["utter"] = A["query"] }
    A["query"] = A["utter"]

    // TRIM: static pruning by guards and record referenced tools
    keepTools := map[string]bool{}
    out := Workflow{Intent: w.Intent, Description: w.Description, Keywords: w.Keywords}
    for _, s := range w.Steps {
        if s.Guard != "" && !EvalGuard(s.Guard, A) { continue }
        step := s
        if step.Tool != nil { keepTools[step.Tool.Name] = true }
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
    for _, s := range w.Steps {
        if s.Tool == nil { continue }
        if !allowed[s.Tool.Name] { return "", fmt.Errorf("tool not permitted: %s", s.Tool.Name) }
        args := renderArgs(s.Tool.Args, A)
        raw, _ := json.Marshal(args)
        payload, err := r.Tools.Dispatch(ctx, s.Tool.Name, raw)
        if err != nil {
            return "", err
        }
        fmt.Fprintf(&summary, "- %s\n", s.Text)
        // Opportunistically capture first_url from web_search result for later steps
        if s.Tool.Name == "web_search" && (A["first_url"] == nil || A["first_url"] == "") {
            var resp struct{
                OK bool `json:"ok"`
                Results []struct{ URL string `json:"url"` } `json:"results"`
            }
            _ = json.Unmarshal(payload, &resp)
            if len(resp.Results) > 0 {
                A["first_url"] = resp.Results[0].URL
            }
        }
        if s.Tool.Name == "web_fetch" {
            var resp struct{
                OK        bool   `json:"ok"`
                Title     string `json:"title"`
                Markdown  string `json:"markdown"`
                FinalURL  string `json:"final_url"`
                InputURL  string `json:"input_url"`
                UsedRead  bool   `json:"used_readable"`
            }
            _ = json.Unmarshal(payload, &resp)
            title := strings.TrimSpace(resp.Title)
            if title == "" { title = "Research Report" }
            // Build a simple deep-research style report
            var b strings.Builder
            fmt.Fprintf(&b, "# %s\n\n", title)
            if resp.FinalURL != "" {
                fmt.Fprintf(&b, "Source: %s\n\n", resp.FinalURL)
            } else if resp.InputURL != "" {
                fmt.Fprintf(&b, "Source: %s\n\n", resp.InputURL)
            }
            b.WriteString("## Executive Summary\n\n")
            b.WriteString("This report summarizes findings from the fetched source.\n\n")
            b.WriteString("## Detailed Findings\n\n")
            if resp.Markdown != "" { b.WriteString(resp.Markdown) }
            A["report_md"] = b.String()
        }
    }
    return summary.String(), nil
}

// renderArgs replaces ${A.key} placeholders in string values within args.
func renderArgs(args map[string]any, A Attrs) map[string]any {
    if args == nil { return nil }
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
        if start == -1 { break }
        end := strings.Index(s[start:], "}")
        if end == -1 { break }
        end += start
        key := strings.TrimPrefix(s[start+2:start+2+len("A.")], "A.")
        // above was wrong, recompute key properly
        inner := s[start+2 : end]
        key = strings.TrimPrefix(inner, "A.")
        val := fmt.Sprintf("%v", A[key])
        s = s[:start] + val + s[end+1:]
    }
    return s
}
