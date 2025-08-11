package warpp

import (
    "encoding/json"
    "errors"
    "io/fs"
    "os"
    "path/filepath"
)

// Registry provides access to workflows by intent.
type Registry struct {
    byIntent map[string]Workflow
}

// LoadFromDir loads all .json workflows from a directory. If the directory is
// missing or empty, it returns a registry with built-in defaults.
func LoadFromDir(dir string) (*Registry, error) {
    r := &Registry{byIntent: map[string]Workflow{}}
    if dir != "" {
        _ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
            if err != nil || d.IsDir() { return nil }
            if filepath.Ext(path) != ".json" { return nil }
            b, err := os.ReadFile(path)
            if err != nil { return nil }
            var w Workflow
            if err := json.Unmarshal(b, &w); err != nil { return nil }
            if w.Intent != "" {
                r.byIntent[w.Intent] = w
            }
            return nil
        })
    }
    if len(r.byIntent) == 0 {
        // seed with defaults
        for _, w := range defaultWorkflows() {
            r.byIntent[w.Intent] = w
        }
    }
    return r, nil
}

func (r *Registry) Get(intent string) (Workflow, error) {
    w, ok := r.byIntent[intent]
    if !ok { return Workflow{}, errors.New("workflow not found") }
    return w, nil
}

func (r *Registry) All() []Workflow {
    out := make([]Workflow, 0, len(r.byIntent))
    for _, w := range r.byIntent { out = append(out, w) }
    return out
}

func defaultWorkflows() []Workflow {
    return []Workflow{
        {
            Intent:      "web_research",
            Description: "Perform web search and fetch readable content from a promising URL",
            Keywords:    []string{"web", "search", "http", "url", "research"},
            Steps: []Step{
                {ID: "s1", Text: "Search the web for the topic", Tool: &ToolRef{Name: "web_search", Args: map[string]any{"query": "${A.query}", "max_results": 5, "category": "general"}}},
                {ID: "s2", Text: "Fetch the first result's content", Tool: &ToolRef{Name: "web_fetch", Args: map[string]any{"url": "${A.first_url}", "prefer_readable": true}}},
            },
        },
        {
            Intent:      "deep_web_report",
            Description: "Research a topic on the web and write a deep research style report to report.md",
            Keywords:    []string{"report", "report.md", "write", "deep research"},
            Steps: []Step{
                {ID: "s1", Text: "Search the web for the topic", Tool: &ToolRef{Name: "web_search", Args: map[string]any{"query": "${A.query}", "max_results": 5, "category": "general"}}},
                {ID: "s2", Text: "Fetch the first result's content", Tool: &ToolRef{Name: "web_fetch", Args: map[string]any{"url": "${A.first_url}", "prefer_readable": true}}},
                {ID: "s3", Text: "Write report to report.md", Tool: &ToolRef{Name: "write_file", Args: map[string]any{"path": "report.md", "content": "${A.report_md}"}}},
            },
        },
        {
            Intent:      "cli_echo",
            Description: "Echo the user's input via run_cli tool",
            Keywords:    []string{"echo", "print", "say"},
            Steps: []Step{
                {ID: "s1", Text: "Echo the utterance", Tool: &ToolRef{Name: "run_cli", Args: map[string]any{"command": "echo", "args": []any{"You said:", "${A.utter}"}}}},
            },
        },
    }
}
