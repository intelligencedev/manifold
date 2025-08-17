package fs

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	dmp "github.com/sergi/go-diff/diffmatchpatch"
	"singularityio/internal/observability"
	"singularityio/internal/sandbox"
)

// ApplyPatchTool applies a Codex-style V4A patch to files in WORKDIR.
// It supports three actions per file: Add, Update, Delete.
// For Update, it uses diff-match-patch to apply the unified patch hunks.
type ApplyPatchTool struct{ workdir string }

func NewApplyPatchTool(workdir string) *ApplyPatchTool { return &ApplyPatchTool{workdir: workdir} }

func (t *ApplyPatchTool) Name() string { return "apply_patch" }

func (t *ApplyPatchTool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": "Apply a Codex-style V4A patch to files within the locked working directory. Supports Add/Update/Delete. Always use relative paths.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"patch":            map[string]any{"type": "string", "description": "Patch text including *** Begin Patch and *** End Patch markers"},
				"dry_run":          map[string]any{"type": "boolean", "description": "If true, do not write changes to disk; just validate and preview.", "default": false},
				"strict":           map[string]any{"type": "boolean", "description": "Fail the whole operation if any file fails to apply.", "default": true},
				"context_fallback": map[string]any{"type": "boolean", "description": "When Update hunks fail, try fuzzy match on surrounding context.", "default": true},
			},
			"required": []string{"patch"},
		},
	}
}

type applyResult struct {
	Path    string `json:"path"`
	Action  string `json:"action"`
	Applied bool   `json:"applied"`
	Bytes   int    `json:"bytes"`
	Message string `json:"message"`
}

func (t *ApplyPatchTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct {
		Patch           string `json:"patch"`
		DryRun          bool   `json:"dry_run"`
		Strict          bool   `json:"strict"`
		ContextFallback bool   `json:"context_fallback"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}
	text, err := extractV4ABody(args.Patch)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	ops, err := parseV4A(text)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	results := make([]applyResult, 0, len(ops))
	okAll := true
	for _, op := range ops {
		rel, err := sandbox.SanitizeArg(t.workdir, op.Path)
		if err != nil {
			results = append(results, applyResult{Path: op.Path, Action: op.Action, Applied: false, Message: err.Error()})
			okAll = false
			continue
		}
		full := filepath.Join(t.workdir, rel)
		switch op.Action {
		case "Add":
			data := strings.Join(op.New, "\n")
			if args.DryRun {
				results = append(results, applyResult{Path: rel, Action: op.Action, Applied: true, Bytes: len(data)})
				continue
			}
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				results = append(results, applyResult{Path: rel, Action: op.Action, Applied: false, Message: err.Error()})
				okAll = false
				continue
			}
			if err := os.WriteFile(full, []byte(data), 0o644); err != nil {
				results = append(results, applyResult{Path: rel, Action: op.Action, Applied: false, Message: err.Error()})
				okAll = false
				continue
			}
			results = append(results, applyResult{Path: rel, Action: op.Action, Applied: true, Bytes: len(data)})
		case "Delete":
			if args.DryRun {
				results = append(results, applyResult{Path: rel, Action: op.Action, Applied: true})
				continue
			}
			if err := os.Remove(full); err != nil {
				results = append(results, applyResult{Path: rel, Action: op.Action, Applied: false, Message: err.Error()})
				okAll = false
				continue
			}
			results = append(results, applyResult{Path: rel, Action: op.Action, Applied: true})
		case "Update":
			// read existing content
			oldBytes, err := os.ReadFile(full)
			if err != nil {
				results = append(results, applyResult{Path: rel, Action: op.Action, Applied: false, Message: fmt.Sprintf("read: %v", err)})
				okAll = false
				continue
			}
			oldText := string(oldBytes)
			// Build a unified patch from hunks
			unified := buildUnifiedPatch(op.Hunks)
			d := dmp.New()
			patches, perr := d.PatchFromText(unified)
			if perr != nil {
				results = append(results, applyResult{Path: rel, Action: op.Action, Applied: false, Message: fmt.Sprintf("parse unified: %v", perr)})
				okAll = false
				continue
			}
			newText, applied := d.PatchApply(patches, oldText)
			// Preserve final newline if the original had one
			if strings.HasSuffix(oldText, "\n") && !strings.HasSuffix(newText, "\n") {
				newText += "\n"
			}
			appliedAll := allTrue(applied)
			if !appliedAll && args.ContextFallback {
				// try again with semantic cleanup by generating diffs from expected new content if provided
				expected := strings.Join(op.New, "\n")
				if strings.HasSuffix(oldText, "\n") && !strings.HasSuffix(expected, "\n") {
					expected += "\n"
				}
				diffs := d.DiffMain(oldText, expected, true)
				d.DiffCleanupSemantic(diffs)
				patches = d.PatchMake(oldText, diffs)
				newText, applied = d.PatchApply(patches, oldText)
				if strings.HasSuffix(oldText, "\n") && !strings.HasSuffix(newText, "\n") {
					newText += "\n"
				}
				appliedAll = allTrue(applied)
			}
			if !appliedAll {
				results = append(results, applyResult{Path: rel, Action: op.Action, Applied: false, Message: "failed to apply one or more hunks"})
				okAll = false
				continue
			}
			if args.DryRun {
				results = append(results, applyResult{Path: rel, Action: op.Action, Applied: true, Bytes: len(newText)})
				continue
			}
			if err := os.WriteFile(full, []byte(newText), 0o644); err != nil {
				results = append(results, applyResult{Path: rel, Action: op.Action, Applied: false, Message: err.Error()})
				okAll = false
				continue
			}
			results = append(results, applyResult{Path: rel, Action: op.Action, Applied: true, Bytes: len(newText)})
		default:
			results = append(results, applyResult{Path: rel, Action: op.Action, Applied: false, Message: "unknown action"})
			okAll = false
			continue
		}
	}
	return map[string]any{"ok": okAll, "results": results}, nil
}

// V4A parsing -----------------------------------------------------------------

type fileOp struct {
	Action string
	Path   string
	Hunks  []hunk
	New    []string // for Add/Update expected content (optional)
}

type hunk struct {
	lines []string // raw lines including +- prefix
}

func extractV4ABody(s string) (string, error) {
	// Find markers *** Begin Patch and *** End Patch
	start := strings.Index(s, "*** Begin Patch")
	if start == -1 {
		return "", errors.New("missing *** Begin Patch")
	}
	end := strings.LastIndex(s, "*** End Patch")
	if end == -1 || end <= start {
		return "", errors.New("missing *** End Patch")
	}
	body := s[start+len("*** Begin Patch") : end]
	return strings.TrimSpace(body), nil
}

func parseV4A(body string) ([]fileOp, error) {
	sc := bufio.NewScanner(strings.NewReader(body))
	sc.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	ops := []fileOp{}
	var cur *fileOp
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "*** ") && strings.Contains(line, " File: ") {
			// flush previous
			if cur != nil {
				ops = append(ops, *cur)
			}
			// parse header: *** Update File: path
			rest := strings.TrimPrefix(line, "*** ")
			parts := strings.SplitN(rest, " File: ", 2)
			if len(parts) != 2 {
				return nil, fmt.Errorf("bad header: %q", line)
			}
			action := strings.TrimSpace(parts[0])
			path := strings.TrimSpace(parts[1])
			cur = &fileOp{Action: action, Path: path}
			continue
		}
		if cur == nil {
			// skip preface lines
			continue
		}
		// hunk or content line.
		if len(line) == 0 {
			continue
		}
		first := line[0]
		switch first {
		case '+', '-', ' ':
			// Normalize optional single space after the marker to match diff-match-patch format.
			// Example: "- line" -> "-line", "  hello" -> " hello".
			var content string
			if len(line) > 1 {
				content = line[1:]
				if strings.HasPrefix(content, " ") {
					content = content[1:]
				}
			} else {
				content = ""
			}
			norm := string(first) + content
			if len(cur.Hunks) == 0 || len(cur.Hunks[len(cur.Hunks)-1].lines) == 0 {
				cur.Hunks = append(cur.Hunks, hunk{})
			}
			idx := len(cur.Hunks) - 1
			cur.Hunks[idx].lines = append(cur.Hunks[idx].lines, norm)
			if first == '+' || first == ' ' {
				cur.New = append(cur.New, content)
			}
		default:
			// Treat unprefixed lines as context equalities per V4A guidance.
			if strings.HasPrefix(line, "@@") {
				// start a new hunk
				cur.Hunks = append(cur.Hunks, hunk{})
				continue
			}
			if strings.HasPrefix(line, "*** ") {
				// next header will handle; ignore
				continue
			}
			// Interpret as context (equal) line
			if len(cur.Hunks) == 0 || len(cur.Hunks[len(cur.Hunks)-1].lines) == 0 {
				cur.Hunks = append(cur.Hunks, hunk{})
			}
			idx := len(cur.Hunks) - 1
			cur.Hunks[idx].lines = append(cur.Hunks[idx].lines, " "+line)
			cur.New = append(cur.New, line)
		}
	}
	if cur != nil {
		ops = append(ops, *cur)
	}
	if err := sc.Err(); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	// Post-validate
	for i := range ops {
		if ops[i].Action == "Add" && len(ops[i].New) == 0 {
			// it's allowed to add empty file
		}
		if ops[i].Action == "Update" && len(ops[i].Hunks) == 0 {
			observability.LoggerWithTrace(context.Background()).Warn().Str("path", ops[i].Path).Msg("apply_patch_update_without_hunks")
		}
	}
	return ops, nil
}

// buildUnifiedPatch converts hunks to a unified patch acceptable by go-diff's PatchFromText.
func buildUnifiedPatch(hs []hunk) string {
	// We cannot compute exact indices without original content; however, go-diff's PatchFromText requires @@ header.
	// We'll synthesize a minimal header with 0,0 positions and rely on fuzzy matching via PatchApply and context lines.
	var b strings.Builder
	for _, h := range hs {
		b.WriteString("@@ -0,0 +0,0 @@\n")
		for _, ln := range h.lines {
			// Ensure each line starts with one of ' ', '+', '-'
			if len(ln) == 0 {
				continue
			}
			switch ln[0] {
			case '+', '-', ' ':
				b.WriteString(ln)
				b.WriteString("\n")
			}
		}
	}
	return b.String()
}

func allTrue(xs []bool) bool {
	for _, v := range xs {
		if !v {
			return false
		}
	}
	return true
}
