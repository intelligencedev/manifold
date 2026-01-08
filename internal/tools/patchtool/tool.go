package patchtool

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"manifold/internal/observability"
	"manifold/internal/sandbox"
)

type Tool struct {
	Workdir           string
	MaxTotalBytes     int
	MaxFiles          int
	AllowCreate       bool
	RequireContextHit bool
}

type callArgs struct {
	Patch   string   `json:"patch"`
	Patches []string `json:"patches"`
	DryRun  bool     `json:"dry_run"`
}

type callResult struct {
	OK       bool          `json:"ok"`
	DryRun   bool          `json:"dry_run,omitempty"`
	Files    []string      `json:"files,omitempty"`
	Added    []string      `json:"added,omitempty"`
	Modified []string      `json:"modified,omitempty"`
	Deleted  []string      `json:"deleted,omitempty"`
	Moves    []moveSummary `json:"moves,omitempty"`
	Error    string        `json:"error,omitempty"`
}

func New(workdir string) *Tool {
	return &Tool{
		Workdir:           workdir,
		MaxTotalBytes:     512_000,
		MaxFiles:          64,
		AllowCreate:       true,
		RequireContextHit: true,
	}
}

func (t *Tool) Name() string { return "apply_patch" }

func (t *Tool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": "OpenAI models only. Apply one or more unified diff patches to files under the locked WORKDIR. Supports add, delete, update, and move semantics.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"patch":   map[string]any{"type": "string", "description": "Single patch body"},
				"patches": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Array of patch bodies"},
				"dry_run": map[string]any{"type": "boolean", "description": "Validate without modifying files"},
			},
		},
	}
}

func (t *Tool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args callArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}

	patches := make([]string, 0, len(args.Patches)+1)
	patches = append(patches, args.Patches...)
	if args.Patch != "" {
		patches = append(patches, args.Patch)
	}
	if len(patches) == 0 {
		return callResult{OK: false, Error: "no patch provided"}, nil
	}

	totalBytes := 0
	for _, p := range patches {
		totalBytes += len(p)
		if totalBytes > t.MaxTotalBytes {
			return callResult{OK: false, Error: fmt.Sprintf("patch size exceeds limit (%d > %d)", totalBytes, t.MaxTotalBytes)}, nil
		}
	}

	// Resolve per-request base directory from context, defaulting to configured workdir
	base := sandbox.ResolveBaseDir(ctx, t.Workdir)
	state := newApplyState(base)
	touched := make(map[string]struct{})

	for idx, body := range patches {
		parsed, err := ParsePatch(body)
		if err != nil {
			return callResult{OK: false, Error: err.Error()}, nil
		}
		if len(parsed.Hunks) == 0 {
			continue
		}
		if err := t.applyParsedPatch(ctx, state, parsed, touched); err != nil {
			return callResult{OK: false, Error: fmt.Sprintf("patch %d: %v", idx+1, err)}, nil
		}
	}

	if len(touched) > t.MaxFiles {
		return callResult{OK: false, Error: fmt.Sprintf("too many files modified (%d > %d)", len(touched), t.MaxFiles)}, nil
	}

	added, modified, deleted, moves := state.summarize()
	files := collectFiles(added, modified, deleted, moves)

	if args.DryRun {
		observability.LoggerWithTrace(ctx).Debug().Int("files", len(files)).Bool("dry_run", true).Msg("apply_patch_dry_run")
		return callResult{OK: true, DryRun: true, Files: files, Added: added, Modified: modified, Deleted: deleted, Moves: moves}, nil
	}

	if err := state.writeToDisk(); err != nil {
		return callResult{OK: false, Error: err.Error()}, nil
	}

	observability.LoggerWithTrace(ctx).Debug().Int("files", len(files)).Msg("apply_patch")
	return callResult{OK: true, Files: files, Added: added, Modified: modified, Deleted: deleted, Moves: moves}, nil
}

func (t *Tool) applyParsedPatch(ctx context.Context, state *applyState, patch *Patch, touched map[string]struct{}) error {
	for _, h := range patch.Hunks {
		switch h.Kind {
		case hunkAdd:
			path, err := t.normalizePath(h.Path)
			if err != nil {
				return err
			}
			if !t.AllowCreate {
				return fmt.Errorf("file creation disabled: %s", path)
			}
			if err := state.addFile(path, h.Contents); err != nil {
				return err
			}
			touched[path] = struct{}{}
		case hunkDelete:
			path, err := t.normalizePath(h.Path)
			if err != nil {
				return err
			}
			if err := state.deleteFile(path, true); err != nil {
				return err
			}
			touched[path] = struct{}{}
		case hunkUpdate:
			path, err := t.normalizePath(h.Path)
			if err != nil {
				return err
			}
			movePath := ""
			if h.MovePath != "" {
				movePath, err = t.normalizePath(h.MovePath)
				if err != nil {
					return err
				}
			}
			if err := state.updateFile(path, movePath, h.Chunks); err != nil {
				return err
			}
			touched[path] = struct{}{}
			if movePath != "" && movePath != path {
				touched[movePath] = struct{}{}
			}
		default:
			return fmt.Errorf("unsupported hunk kind")
		}
	}
	return nil
}

func (t *Tool) normalizePath(path string) (string, error) {
	candidate := strings.TrimSpace(path)
	if candidate == "" {
		return "", fmt.Errorf("empty path")
	}
	if strings.HasPrefix(candidate, "a/") || strings.HasPrefix(candidate, "b/") {
		candidate = candidate[2:]
	}
	candidate = strings.TrimPrefix(candidate, "./")

	if filepath.IsAbs(candidate) {
		abs := filepath.Clean(candidate)
		rel, err := filepath.Rel(t.Workdir, abs)
		if err != nil {
			return "", fmt.Errorf("path %s outside workdir", path)
		}
		if strings.HasPrefix(rel, "..") || rel == "." && abs != t.Workdir {
			return "", fmt.Errorf("path %s escapes workdir", path)
		}
		candidate = rel
	}

	cleaned := filepath.Clean(candidate)
	rel, err := sandbox.SanitizeArg(t.Workdir, cleaned)
	if err != nil {
		return "", err
	}
	rel = filepath.Clean(rel)
	rel = filepath.FromSlash(rel)
	if rel == "." {
		return "", fmt.Errorf("path resolves to workdir root")
	}
	return rel, nil
}

func collectFiles(added, modified, deleted []string, moves []moveSummary) []string {
	set := make(map[string]struct{})
	for _, v := range added {
		set[v] = struct{}{}
	}
	for _, v := range modified {
		set[v] = struct{}{}
	}
	for _, v := range deleted {
		set[v] = struct{}{}
	}
	for _, mv := range moves {
		set[mv.From] = struct{}{}
		set[mv.To] = struct{}{}
	}
	files := make([]string, 0, len(set))
	for k := range set {
		files = append(files, k)
	}
	sort.Strings(files)
	return files
}
