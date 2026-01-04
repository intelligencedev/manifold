package patchtool

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type pendingFile struct {
	Path            string
	Content         string
	Exists          bool
	OriginalExists  bool
	Dirty           bool
	RemoveReport    bool
	MoveSource      string // non-empty if this entry is the destination of a move
	MoveDestination string // non-empty if this entry was moved to another path
	Mode            os.FileMode
	ModeSet         bool
}

type applyState struct {
	workdir string
	files   map[string]*pendingFile
}

func newApplyState(workdir string) *applyState {
	abs, err := filepath.Abs(workdir)
	if err != nil {
		abs = filepath.Clean(workdir)
	}
	return &applyState{workdir: abs, files: make(map[string]*pendingFile)}
}

func (s *applyState) addFile(path, contents string) error {
	entry, err := s.ensureEntry(path)
	if err != nil {
		return err
	}
	if entry.Exists && !entry.RemoveReport {
		return fmt.Errorf("file already exists: %s", path)
	}
	if entry.Exists && entry.RemoveReport {
		// Previously scheduled for deletion; treat as replacement.
		entry.OriginalExists = false
	}
	entry.Content = contents
	entry.Exists = true
	entry.Dirty = true
	entry.RemoveReport = false
	entry.MoveSource = ""
	entry.MoveDestination = ""
	if !entry.ModeSet {
		entry.Mode = 0o644
		entry.ModeSet = true
	}
	return nil
}

func (s *applyState) deleteFile(path string, report bool) error {
	entry, err := s.ensureEntry(path)
	if err != nil {
		return err
	}
	if !entry.Exists && !entry.OriginalExists {
		return fmt.Errorf("cannot delete non-existent file: %s", path)
	}
	if !entry.Exists && entry.MoveDestination != "" {
		return fmt.Errorf("cannot delete %s: file already moved to %s", path, entry.MoveDestination)
	}
	entry.Exists = false
	entry.Dirty = true
	entry.RemoveReport = report
	entry.MoveSource = ""
	entry.MoveDestination = ""
	entry.Content = ""
	return nil
}

func (s *applyState) updateFile(path, movePath string, chunks []UpdateChunk) error {
	entry, err := s.ensureEntry(path)
	if err != nil {
		return err
	}
	if !entry.Exists {
		return fmt.Errorf("cannot update non-existent file: %s", path)
	}
	if entry.MoveDestination != "" {
		return fmt.Errorf("file %s was moved to %s earlier in the patch", path, entry.MoveDestination)
	}

	newContent, err := applyChunksToContent(path, entry.Content, chunks)
	if err != nil {
		return err
	}

	if movePath != "" && movePath != path {
		dest, err := s.ensureEntry(movePath)
		if err != nil {
			return err
		}
		dest.Content = newContent
		dest.Exists = true
		dest.Dirty = true
		dest.MoveSource = path
		dest.RemoveReport = false
		if !dest.ModeSet {
			// Adopt the source mode if known, otherwise default later.
			if entry.ModeSet {
				dest.Mode = entry.Mode
				dest.ModeSet = true
			} else {
				dest.Mode = 0o644
				dest.ModeSet = true
			}
		}
		entry.Exists = false
		entry.Dirty = true
		entry.RemoveReport = false
		entry.MoveDestination = movePath
		entry.Content = ""
		return nil
	}

	entry.Content = newContent
	entry.Exists = true
	entry.Dirty = true
	entry.RemoveReport = false
	return nil
}

func (s *applyState) ensureEntry(path string) (*pendingFile, error) {
	if entry, ok := s.files[path]; ok {
		return entry, nil
	}
	full, err := resolveUnderRoot(s.workdir, path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(full)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			entry := &pendingFile{Path: path, Exists: false, OriginalExists: false}
			s.files[path] = entry
			return entry, nil
		}
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}
	info, statErr := os.Stat(full)
	mode := os.FileMode(0o644)
	if statErr == nil {
		mode = info.Mode().Perm()
	}
	entry := &pendingFile{
		Path:           path,
		Content:        string(data),
		Exists:         true,
		OriginalExists: true,
		Dirty:          false,
		Mode:           mode,
		ModeSet:        true,
	}
	s.files[path] = entry
	return entry, nil
}

func (s *applyState) writeToDisk() error {
	// Remove files first.
	for _, entry := range s.files {
		if entry.Exists {
			continue
		}
		if err := s.removeFile(entry); err != nil {
			return err
		}
	}

	for _, entry := range s.files {
		if !entry.Exists {
			continue
		}
		if !entry.Dirty && entry.MoveSource == "" {
			// No change needed.
			continue
		}
		if err := s.writeFile(entry); err != nil {
			return err
		}
	}
	return nil
}

func (s *applyState) removeFile(entry *pendingFile) error {
	if !entry.OriginalExists && entry.MoveDestination == "" {
		// Nothing to remove on disk.
		return nil
	}
	target, err := resolveUnderRoot(s.workdir, entry.Path)
	if err != nil {
		return err
	}
	err = os.Remove(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if entry.OriginalExists {
				return fmt.Errorf("failed to delete %s: %w", entry.Path, err)
			}
			return nil
		}
		return fmt.Errorf("failed to delete %s: %w", entry.Path, err)
	}
	return nil
}

func (s *applyState) writeFile(entry *pendingFile) error {
	target, err := resolveUnderRoot(s.workdir, entry.Path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("failed to create directories for %s: %w", entry.Path, err)
	}
	mode := entry.Mode
	if !entry.ModeSet {
		mode = 0o644
	}
	if err := os.WriteFile(target, []byte(entry.Content), mode); err != nil {
		return fmt.Errorf("failed to write %s: %w", entry.Path, err)
	}
	return nil
}

func resolveUnderRoot(absRoot, relPath string) (string, error) {
	relPath = strings.TrimSpace(relPath)
	if relPath == "" {
		return "", fmt.Errorf("empty path")
	}
	if filepath.IsAbs(relPath) {
		return "", fmt.Errorf("absolute paths are not allowed: %s", relPath)
	}

	cleanRel := filepath.Clean(relPath)
	if cleanRel == "." {
		return "", fmt.Errorf("path resolves to root")
	}
	if cleanRel == ".." || strings.HasPrefix(cleanRel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes root: %s", relPath)
	}

	// absRoot is expected to already be absolute, but enforce it defensively.
	root, err := filepath.Abs(absRoot)
	if err != nil {
		root = filepath.Clean(absRoot)
	}

	candidate, err := filepath.Abs(filepath.Join(root, cleanRel))
	if err != nil {
		return "", fmt.Errorf("resolve path %s: %w", relPath, err)
	}
	relToRoot, err := filepath.Rel(root, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve path %s: %w", relPath, err)
	}
	if relToRoot == "." {
		return "", fmt.Errorf("path resolves to root")
	}
	if relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes root: %s", relPath)
	}

	return candidate, nil
}

func (s *applyState) summarize() (added, modified, deleted []string, moves []moveSummary) {
	added = make([]string, 0)
	modified = make([]string, 0)
	deleted = make([]string, 0)
	moves = make([]moveSummary, 0)
	for _, entry := range s.files {
		if entry.Exists {
			if entry.MoveSource != "" {
				moves = append(moves, moveSummary{From: entry.MoveSource, To: entry.Path})
				modified = append(modified, entry.Path)
				continue
			}
			if !entry.OriginalExists {
				added = append(added, entry.Path)
				continue
			}
			if entry.Dirty {
				modified = append(modified, entry.Path)
			}
		} else if entry.RemoveReport {
			deleted = append(deleted, entry.Path)
		}
	}
	sort.Strings(added)
	sort.Strings(modified)
	sort.Strings(deleted)
	sort.Slice(moves, func(i, j int) bool { return strings.Compare(moves[i].To, moves[j].To) < 0 })
	return added, modified, deleted, moves
}

func applyChunksToContent(path, content string, chunks []UpdateChunk) (string, error) {
	lines := strings.Split(content, "\n")
	hadTrailing := false
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		hadTrailing = true
		lines = lines[:len(lines)-1]
	}

	replacements := make([]replacement, 0, len(chunks))
	cursor := 0

	for _, chunk := range chunks {
		if chunk.ChangeContext != nil {
			idx := seekSequence(lines, []string{*chunk.ChangeContext}, cursor, false)
			if idx < 0 {
				return "", fmt.Errorf("failed to find context '%s' in %s", *chunk.ChangeContext, path)
			}
			cursor = idx + 1
		}

		if len(chunk.OldLines) == 0 {
			insertIdx := len(lines)
			replacements = append(replacements, replacement{Index: insertIdx, Remove: 0, NewLines: append([]string(nil), chunk.NewLines...)})
			continue
		}

		pattern := append([]string(nil), chunk.OldLines...)
		newSlice := append([]string(nil), chunk.NewLines...)

		idx := seekSequence(lines, pattern, cursor, chunk.IsEndOfFile)
		if idx < 0 && len(pattern) > 0 && pattern[len(pattern)-1] == "" {
			pattern = pattern[:len(pattern)-1]
			if len(newSlice) > 0 && newSlice[len(newSlice)-1] == "" {
				newSlice = newSlice[:len(newSlice)-1]
			}
			idx = seekSequence(lines, pattern, cursor, chunk.IsEndOfFile)
		}
		if idx < 0 {
			return "", fmt.Errorf("failed to find expected lines in %s", path)
		}
		replacements = append(replacements, replacement{Index: idx, Remove: len(pattern), NewLines: newSlice})
		cursor = idx + len(pattern)
	}

	sort.Slice(replacements, func(i, j int) bool { return replacements[i].Index < replacements[j].Index })
	lines = applyReplacements(lines, replacements)

	if hadTrailing || len(lines) == 0 || lines[len(lines)-1] != "" {
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n"), nil
}

type replacement struct {
	Index    int
	Remove   int
	NewLines []string
}

func applyReplacements(lines []string, repls []replacement) []string {
	out := append([]string(nil), lines...)
	for i := len(repls) - 1; i >= 0; i-- {
		r := repls[i]
		if r.Index > len(out) {
			r.Index = len(out)
		}
		end := r.Index + r.Remove
		if end > len(out) {
			end = len(out)
		}
		out = append(out[:r.Index], append(r.NewLines, out[end:]...)...)
	}
	return out
}

func seekSequence(haystack, needle []string, start int, requireEOF bool) int {
	if len(needle) == 0 {
		if requireEOF {
			return len(haystack)
		}
		return start
	}
	max := len(haystack) - len(needle)
	for i := start; i <= max; i++ {
		match := true
		for j := 0; j < len(needle); j++ {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			if requireEOF && i+len(needle) != len(haystack) {
				continue
			}
			return i
		}
	}
	return -1
}
