package skills

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	skillsDirName = ".manifold/skills"
	skillFileName = "SKILL.md"

	maxNameLen      = 64
	maxDescLen      = 1024
	maxShortDescLen = maxDescLen
)

// Loader discovers and parses skills from well-known roots.
type Loader struct {
	// Workdir is the cwd for the current request; repo skills are discovered relative to it.
	Workdir string
	// UserDir is the user-scoped skills root (e.g., ~/.manifold/skills).
	UserDir string
	// AdminDir is an optional machine-wide skills root (e.g., /etc/codex/skills).
	AdminDir string
}

// Load returns discovered skills in precedence order (repo > user > admin) with deduplication by name.
func (l Loader) Load() LoadOutcome {
	roots := []struct {
		Path  string
		Scope Scope
	}{
		{l.repoSkillsRoot(), ScopeRepo},
		{strings.TrimSpace(l.UserDir), ScopeUser},
	}
	if strings.TrimSpace(l.AdminDir) != "" {
		roots = append(roots, struct {
			Path  string
			Scope Scope
		}{strings.TrimSpace(l.AdminDir), ScopeAdmin})
	}

	var outcome LoadOutcome
	seen := make(map[string]struct{})

	for _, root := range roots {
		if root.Path == "" {
			continue
		}
		for _, path := range discoverSkillFiles(root.Path) {
			md, err := parseSkill(path, root.Scope)
			if err != nil {
				outcome.Errors = append(outcome.Errors, Error{Path: path, Message: err.Error()})
				continue
			}
			if _, dup := seen[md.Name]; dup {
				continue
			}
			seen[md.Name] = struct{}{}
			outcome.Skills = append(outcome.Skills, md)
		}
	}

	return outcome
}

func (l Loader) repoSkillsRoot() string {
	wd := strings.TrimSpace(l.Workdir)
	if wd == "" {
		return ""
	}

	dir := wd
	for {
		candidate := filepath.Join(dir, skillsDirName)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			break
		}
		dir = parent
	}
	return ""
}

func discoverSkillFiles(root string) []string {
	var paths []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == skillFileName {
			paths = append(paths, path)
		}
		return nil
	})
	return paths
}

func parseSkill(path string, scope Scope) (Metadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Metadata{}, fmt.Errorf("read: %w", err)
	}
	fm, err := extractFrontmatter(string(data))
	if err != nil {
		return Metadata{}, err
	}

	name := singleLine(fm.Name)
	desc := singleLine(fm.Description)
	short := singleLine(fm.Metadata.ShortDescription)

	if name == "" {
		return Metadata{}, fmt.Errorf("missing field `name`")
	}
	if len([]rune(name)) > maxNameLen {
		return Metadata{}, fmt.Errorf("invalid name: exceeds %d characters", maxNameLen)
	}
	if desc == "" {
		return Metadata{}, fmt.Errorf("missing field `description`")
	}
	if len([]rune(desc)) > maxDescLen {
		return Metadata{}, fmt.Errorf("invalid description: exceeds %d characters", maxDescLen)
	}
	if short != "" && len([]rune(short)) > maxShortDescLen {
		return Metadata{}, fmt.Errorf("invalid metadata.short-description: exceeds %d characters", maxShortDescLen)
	}

	return Metadata{
		Name:             name,
		Description:      desc,
		ShortDescription: short,
		Path:             filepath.Clean(path),
		Scope:            scope,
	}, nil
}

type frontmatter struct {
	Name        string     `yaml:"name"`
	Description string     `yaml:"description"`
	Metadata    fmMetadata `yaml:"metadata"`
}

type fmMetadata struct {
	ShortDescription string `yaml:"short-description"`
}

func extractFrontmatter(contents string) (frontmatter, error) {
	const delim = "---"
	lines := strings.Split(contents, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != delim {
		return frontmatter{}, fmt.Errorf("missing YAML frontmatter delimited by ---")
	}
	var body []string
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == delim {
			break
		}
		body = append(body, lines[i])
	}
	if len(body) == 0 {
		return frontmatter{}, fmt.Errorf("missing YAML frontmatter delimited by ---")
	}
	var fm frontmatter
	if err := yaml.Unmarshal([]byte(strings.Join(body, "\n")), &fm); err != nil {
		return frontmatter{}, fmt.Errorf("invalid YAML: %w", err)
	}
	return fm, nil
}

func singleLine(s string) string {
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}
