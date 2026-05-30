package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

const (
	skillsDirName = "skills"
	skillFileName = "SKILL.md"

	maxNameLen      = 64
	maxDescLen      = 1024
	maxShortDescLen = maxDescLen
)

type rootSpec struct {
	source Source
	path   string
	base   string
}

// LoadFromDir loads project skills from {dir}/skills plus universal skills
// under $HOME/.manifold/skills and $HOME/.agents/skills. Duplicate skill names
// are resolved by root order: project, manifold, agents.
func LoadFromDir(dir string) LoadOutcome {
	var outcome LoadOutcome
	seen := make(map[string]struct{})
	for _, root := range rootsForProject(dir) {
		loadRoot(root, &outcome, seen)
	}
	return outcome
}

// UniversalFingerprint returns a stable fingerprint of universal skill metadata
// files so cache entries can refresh when home-directory skills change.
func UniversalFingerprint() string {
	var parts []string
	for _, root := range universalRoots() {
		for _, path := range discoverSkillFiles(root.path) {
			info, err := os.Stat(path)
			if err != nil || info.IsDir() {
				continue
			}
			parts = append(parts, fmt.Sprintf("%s:%s:%d:%d", root.source, filepath.Clean(path), info.Size(), info.ModTime().UnixNano()))
		}
	}
	sort.Strings(parts)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:])
}

func rootsForProject(dir string) []rootSpec {
	roots := make([]rootSpec, 0, 3)
	if strings.TrimSpace(dir) != "" {
		roots = append(roots, rootSpec{
			source: SourceProject,
			path:   filepath.Join(dir, skillsDirName),
			base:   dir,
		})
	}
	return append(roots, universalRoots()...)
}

func universalRoots() []rootSpec {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return nil
	}
	return []rootSpec{
		{source: SourceManifold, path: filepath.Join(home, ".manifold", "skills"), base: filepath.Join(home, ".manifold", "skills")},
		{source: SourceAgents, path: filepath.Join(home, ".agents", "skills"), base: filepath.Join(home, ".agents", "skills")},
	}
}

func loadRoot(root rootSpec, outcome *LoadOutcome, seen map[string]struct{}) {
	if strings.TrimSpace(root.path) == "" {
		return
	}
	log.Debug().Str("skillsPath", root.path).Str("source", string(root.source)).Msg("skills_load_from_root")

	info, err := os.Stat(root.path)
	if err != nil || !info.IsDir() {
		log.Debug().Str("skillsPath", root.path).Bool("exists", err == nil).Msg("skills_dir_not_found")
		return
	}

	for _, path := range discoverSkillFiles(root.path) {
		md, err := parseSkill(path, root.source)
		if err != nil {
			outcome.Errors = append(outcome.Errors, Error{Path: path, Message: err.Error()})
			continue
		}
		key := strings.ToLower(md.Name)
		if _, ok := seen[key]; ok {
			continue
		}
		if rel, relErr := filepath.Rel(root.base, md.Path); relErr == nil {
			md.Path = rel
		}
		md.Path = filepath.ToSlash(md.Path)
		md.SkillID = skillID(md.Source, md.Name)
		md.SkillDir = filepath.Dir(path)
		seen[key] = struct{}{}
		outcome.Skills = append(outcome.Skills, md)
	}
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

func parseSkill(path string, source Source) (Metadata, error) {
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
		Source:           source,
	}, nil
}

func skillID(source Source, name string) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	for _, r := range normalized {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	id := strings.Trim(b.String(), "-")
	if id == "" {
		id = "skill"
	}
	return string(source) + ":" + id
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
