package skills

import (
	"context"
	"io"
	"path"
	"strings"

	"github.com/rs/zerolog/log"
)

// S3SkillsLoader loads skills directly from S3 without hydrating the entire workspace.
// This is optimized for enterprise deployments where skills need to be fetched
// independently from the full workspace sync.
type S3SkillsLoader struct {
	projectSvc SkillsProjectService
}

// NewS3SkillsLoader creates a new S3-backed skills loader.
func NewS3SkillsLoader(svc SkillsProjectService) *S3SkillsLoader {
	return &S3SkillsLoader{projectSvc: svc}
}

// LoadSkillsOnly fetches only .manifold/skills/** from S3 without hydrating the entire workspace.
// Returns parsed skill metadata without downloading the full project.
func (l *S3SkillsLoader) LoadSkillsOnly(ctx context.Context, userID int64, projectID string) LoadOutcome {
	var outcome LoadOutcome

	if l.projectSvc == nil {
		log.Debug().Msg("s3_skills_loader: no project service configured")
		return outcome
	}

	// List entries under .manifold/skills/
	skillsPath := ".manifold/skills"
	entries, err := l.projectSvc.ListTreeForSkills(ctx, userID, projectID, skillsPath)
	if err != nil {
		log.Debug().Err(err).Str("projectID", projectID).Str("path", skillsPath).Msg("s3_skills_loader: list tree failed")
		return outcome
	}

	// Find all SKILL.md files
	skillFiles := l.findSkillFiles(ctx, userID, projectID, skillsPath, entries)

	for _, filePath := range skillFiles {
		md, err := l.loadSkillFile(ctx, userID, projectID, filePath)
		if err != nil {
			outcome.Errors = append(outcome.Errors, Error{Path: filePath, Message: err.Error()})
			continue
		}
		outcome.Skills = append(outcome.Skills, md)
	}

	log.Debug().
		Str("projectID", projectID).
		Int("skillsFound", len(outcome.Skills)).
		Int("errors", len(outcome.Errors)).
		Msg("s3_skills_loader: load complete")

	return outcome
}

// findSkillFiles recursively discovers SKILL.md files under the given path.
func (l *S3SkillsLoader) findSkillFiles(ctx context.Context, userID int64, projectID, basePath string, entries []SkillsFileEntry) []string {
	var files []string

	for _, entry := range entries {
		fullPath := path.Join(basePath, entry.Name)

		if entry.Type == "dir" {
			// Skip hidden directories (except the base skills dir itself)
			if strings.HasPrefix(entry.Name, ".") && entry.Name != ".manifold" {
				continue
			}
			// Recursively list subdirectory
			subEntries, err := l.projectSvc.ListTreeForSkills(ctx, userID, projectID, fullPath)
			if err != nil {
				log.Debug().Err(err).Str("path", fullPath).Msg("s3_skills_loader: subdir list failed")
				continue
			}
			files = append(files, l.findSkillFiles(ctx, userID, projectID, fullPath, subEntries)...)
		} else if entry.Name == skillFileName {
			files = append(files, fullPath)
		}
	}

	return files
}

// loadSkillFile reads and parses a single SKILL.md file from S3.
func (l *S3SkillsLoader) loadSkillFile(ctx context.Context, userID int64, projectID, filePath string) (Metadata, error) {
	reader, err := l.projectSvc.ReadFile(ctx, userID, projectID, filePath)
	if err != nil {
		return Metadata{}, err
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return Metadata{}, err
	}

	return parseSkillContent(data, filePath, ScopeRepo)
}

// parseSkillContent parses skill metadata from raw file content.
// This is a variant of parseSkill that works with in-memory content.
func parseSkillContent(data []byte, filePath string, scope Scope) (Metadata, error) {
	fm, err := extractFrontmatter(string(data))
	if err != nil {
		return Metadata{}, err
	}

	name := singleLine(fm.Name)
	desc := singleLine(fm.Description)
	short := singleLine(fm.Metadata.ShortDescription)

	if name == "" {
		return Metadata{}, errMissingName
	}
	if len([]rune(name)) > maxNameLen {
		return Metadata{}, errNameTooLong
	}
	if desc == "" {
		return Metadata{}, errMissingDesc
	}
	if len([]rune(desc)) > maxDescLen {
		return Metadata{}, errDescTooLong
	}
	if short != "" && len([]rune(short)) > maxShortDescLen {
		return Metadata{}, errShortDescTooLong
	}

	return Metadata{
		Name:             name,
		Description:      desc,
		ShortDescription: short,
		Path:             filePath,
		Scope:            scope,
	}, nil
}

// Sentinel errors for skill parsing.
var (
	errMissingName      = newSkillError("missing field `name`")
	errNameTooLong      = newSkillError("invalid name: exceeds character limit")
	errMissingDesc      = newSkillError("missing field `description`")
	errDescTooLong      = newSkillError("invalid description: exceeds character limit")
	errShortDescTooLong = newSkillError("invalid metadata.short-description: exceeds character limit")
)

type skillError struct {
	msg string
}

func newSkillError(msg string) *skillError {
	return &skillError{msg: msg}
}

func (e *skillError) Error() string {
	return e.msg
}
