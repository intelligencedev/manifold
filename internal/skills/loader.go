package skills

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

const (
	skillsDirName = ".skills"
	skillFileName = "SKILL.md"

	maxNameLen      = 64
	maxDescLen      = 1024
	maxShortDescLen = maxDescLen
)

// Loader discovers and parses skills from the project .skills folder.
type Loader struct {
	// Workdir is the cwd for the current request; repo skills are discovered relative to it.
	Workdir string
}

// LoadFromDir directly loads skills from {dir}/.skills without walking up
// the directory tree. This is the preferred method for project-scoped skills.
func LoadFromDir(dir string) LoadOutcome {
	var outcome LoadOutcome

	if strings.TrimSpace(dir) == "" {
		return outcome
	}

	skillsPath := filepath.Join(dir, skillsDirName)
	log.Debug().Str("skillsPath", skillsPath).Msg("skills_load_from_dir")

	info, err := os.Stat(skillsPath)
	if err != nil || !info.IsDir() {
		log.Debug().Str("skillsPath", skillsPath).Bool("exists", err == nil).Msg("skills_dir_not_found")
		return outcome
	}

	for _, path := range discoverSkillFiles(skillsPath) {
		md, err := parseSkill(path, ScopeRepo)
		if err != nil {
			outcome.Errors = append(outcome.Errors, Error{Path: path, Message: err.Error()})
			continue
		}
		outcome.Skills = append(outcome.Skills, md)
	}

	return outcome
}

// Load returns discovered skills from the project .skills folder.
func (l Loader) Load() LoadOutcome {
	var outcome LoadOutcome

	root := l.repoSkillsRoot()
	if root == "" {
		return outcome
	}

	for _, path := range discoverSkillFiles(root) {
		md, err := parseSkill(path, ScopeRepo)
		if err != nil {
			outcome.Errors = append(outcome.Errors, Error{Path: path, Message: err.Error()})
			continue
		}
		outcome.Skills = append(outcome.Skills, md)
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
		log.Debug().Str("checking", candidate).Msg("skills_loader_checking_path")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			log.Debug().Str("found", candidate).Msg("skills_loader_found_skills_dir")
			return candidate
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			log.Debug().Str("dir", dir).Msg("skills_loader_reached_root")
			break
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			log.Debug().Str("dir", dir).Msg("skills_loader_stopped_at_git")
			break
		}
		dir = parent
	}
	log.Debug().Str("workdir", wd).Msg("skills_loader_no_skills_found")
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
