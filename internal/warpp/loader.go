package warpp

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	persist "manifold/internal/persistence"
)

// Registry provides access to workflows by intent.
type Registry struct {
	byIntent     map[string]Workflow
	pathByIntent map[string]string
}

// defaultStore is an optional, package-level WarppWorkflowStore used by
// defaultWorkflows() to source seed workflows from a database instead of
// hard-coded values.
var defaultStore persist.WarppWorkflowStore

// SetDefaultStore configures the store used by defaultWorkflows(). It is safe
// to pass nil to disable DB-backed defaults.
func SetDefaultStore(s persist.WarppWorkflowStore) { defaultStore = s }

// LoadFromDir loads all .json workflows from a directory. If the directory is
// missing or empty, it returns a registry with built-in defaults.
func LoadFromDir(dir string) (*Registry, error) {
	r := &Registry{byIntent: map[string]Workflow{}, pathByIntent: map[string]string{}}
	if dir != "" {
		_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if filepath.Ext(path) != ".json" {
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			var w Workflow
			if err := json.Unmarshal(b, &w); err != nil {
				return nil
			}
			if err := ValidateWorkflow(w); err != nil {
				// Skip invalid workflows; could log in a higher layer
				return nil
			}
			if w.Intent != "" {
				r.byIntent[w.Intent] = w
				r.pathByIntent[w.Intent] = path
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
	if !ok {
		return Workflow{}, errors.New("workflow not found")
	}
	return w, nil
}

func (r *Registry) All() []Workflow {
	out := make([]Workflow, 0, len(r.byIntent))
	for _, w := range r.byIntent {
		out = append(out, w)
	}
	return out
}

// Upsert stores or updates the workflow for the given intent and records the
// source file path when provided.
func (r *Registry) Upsert(w Workflow, path string) {
	if r == nil || w.Intent == "" {
		return
	}
	if r.byIntent == nil {
		r.byIntent = map[string]Workflow{}
	}
	if r.pathByIntent == nil {
		r.pathByIntent = map[string]string{}
	}
	r.byIntent[w.Intent] = w
	if path != "" {
		r.pathByIntent[w.Intent] = path
	}
}

// Remove deletes a workflow from the registry maps.
func (r *Registry) Remove(intent string) {
	if r == nil {
		return
	}
	delete(r.byIntent, intent)
	if r.pathByIntent != nil {
		delete(r.pathByIntent, intent)
	}
}

// Path returns the on-disk location for a workflow if known.
func (r *Registry) Path(intent string) string {
	if r == nil || r.pathByIntent == nil {
		return ""
	}
	return r.pathByIntent[intent]
}

// SaveWorkflow writes the workflow JSON to dir, returning the resulting path.
func SaveWorkflow(dir string, w Workflow) (string, error) {
	if w.Intent == "" {
		return "", errors.New("workflow intent required")
	}
	if dir == "" {
		return "", errors.New("workflow dir required")
	}
	if err := ValidateWorkflow(w); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	filename := sanitizeIntent(w.Intent) + ".json"
	path := filepath.Join(dir, filename)
	if err := SaveWorkflowToPath(path, w); err != nil {
		return "", err
	}
	return path, nil
}

// SaveWorkflowToPath writes the workflow JSON to an explicit path.
func SaveWorkflowToPath(path string, w Workflow) error {
	if path == "" {
		return errors.New("workflow path required")
	}
	if err := ValidateWorkflow(w); err != nil {
		return err
	}
	data, err := json.MarshalIndent(w, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func sanitizeIntent(intent string) string {
	var b strings.Builder
	b.Grow(len(intent))
	for _, r := range intent {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "workflow"
	}
	return b.String()
}

func defaultWorkflows() []Workflow {
	if defaultStore == nil {
		return nil
	}
	_ = defaultStore.Init(context.Background())
	list, err := defaultStore.ListWorkflows(context.Background())
	if err != nil || len(list) == 0 {
		return nil
	}
	out := make([]Workflow, 0, len(list))
	for _, pw := range list {
		b, err := json.Marshal(pw)
		if err != nil {
			continue
		}
		var w Workflow
		if err := json.Unmarshal(b, &w); err != nil {
			continue
		}
		if err := ValidateWorkflow(w); err != nil {
			continue
		}
		if w.Intent != "" {
			out = append(out, w)
		}
	}
	return out
}

// ValidateWorkflow checks IDs, references, and acyclicity of a workflow DAG.
func ValidateWorkflow(w Workflow) error {
	// Unique step IDs
	ids := make(map[string]struct{}, len(w.Steps))
	for _, s := range w.Steps {
		if s.ID == "" {
			return errors.New("step id required")
		}
		if _, dup := ids[s.ID]; dup {
			return errors.New("duplicate step id: " + s.ID)
		}
		ids[s.ID] = struct{}{}
	}
	// DependsOn references must exist
	indegree := make(map[string]int, len(w.Steps))
	adj := make(map[string][]string, len(w.Steps))
	for _, s := range w.Steps {
		indegree[s.ID] = 0
	}
	for _, s := range w.Steps {
		for _, dep := range s.DependsOn {
			if _, ok := ids[dep]; !ok {
				return errors.New("unknown depends_on reference: " + dep + " -> " + s.ID)
			}
			indegree[s.ID]++
			adj[dep] = append(adj[dep], s.ID)
		}
	}
	// Kahn's algorithm for cycle detection
	queue := make([]string, 0)
	for id, d := range indegree {
		if d == 0 {
			queue = append(queue, id)
		}
	}
	visited := 0
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		visited++
		for _, m := range adj[n] {
			indegree[m]--
			if indegree[m] == 0 {
				queue = append(queue, m)
			}
		}
	}
	if visited != len(w.Steps) {
		return errors.New("workflow has a cycle or unreachable dependency")
	}
	return nil
}

// LoadFromStore loads workflows from a persistence.WarppWorkflowStore. If the
// provided store is nil or an error occurs, the registry will fall back to the
// built-in defaults.
func LoadFromStore(ctx context.Context, store persist.WarppWorkflowStore) (*Registry, error) {
	r := &Registry{byIntent: map[string]Workflow{}, pathByIntent: map[string]string{}}
	if store == nil {
		// seed defaults
		for _, w := range defaultWorkflows() {
			r.byIntent[w.Intent] = w
		}
		return r, nil
	}
	// best-effort init
	_ = store.Init(ctx)
	wfs, err := store.ListWorkflows(ctx)
	if err != nil {
		return nil, err
	}
	for _, pw := range wfs {
		b, err := json.Marshal(pw)
		if err != nil {
			continue
		}
		var w Workflow
		if err := json.Unmarshal(b, &w); err != nil {
			continue
		}
		if err := ValidateWorkflow(w); err != nil {
			continue
		}
		if w.Intent != "" {
			r.byIntent[w.Intent] = w
		}
	}
	if len(r.byIntent) == 0 {
		for _, w := range defaultWorkflows() {
			r.byIntent[w.Intent] = w
		}
	}
	return r, nil
}
