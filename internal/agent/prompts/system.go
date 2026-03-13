package prompts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"

	"manifold/internal/skills"
)

var skillsCache = skills.DefaultCache()

const memoryInstructions = `
[memory]
- EvolvingMemory provides two context sources:
	1. Past Relevant Experiences: similar prior tasks, solutions, and lessons.
	2. Conversation History: the actual earlier messages in this session.
- Respond only to [CURRENT REQUEST]. [CONVERSATION HISTORY] is background context; do not re-answer or re-run it.
- If "## Past Relevant Experiences" or "## Current Task" appears, use it when relevant.
- Assistant messages in conversation history are your prior responses in this session. Treat them as authoritative session context; reference them when useful, but do not regenerate them unless asked.
- Use memory to improve continuity and avoid repeating mistakes.
- Do not claim this is the first message when conversation history exists.
- If the current request refers to prior discussion, use history to understand context, then answer the current request.
[/memory]`

// EnsureMemoryInstructions appends memory system instructions to any system prompt
// if they are not already present. This ensures all agents (orchestrator, specialists,
// and delegated agents) receive memory usage guidance.
func EnsureMemoryInstructions(systemPrompt string) string {
	if strings.Contains(systemPrompt, "[memory]") {
		return systemPrompt
	}
	return systemPrompt + memoryInstructions
}

// DefaultSystemPrompt describes the run_cli tool clearly so the model will use it.
// If an AGENTS.md file exists in the provided workdir, its contents will be
// appended to the returned system prompt to provide additional agent-specific
// instructions.
//
// The override parameter, when non-empty, is appended after the hard-coded
// default so custom orchestrator guidance preserves the shared base rules.
func DefaultSystemPrompt(workdir, override string) string {
	base := fmt.Sprintf(`
Rules:
- Plan first, then execute.
- No shell features: no pipelines or redirects; use command + args only.
- Treat all paths as relative to the locked working directory: %s
- Never use absolute paths or escape the working directory.
- Prefer short, deterministic, non-interactive commands; pass input via flags/args.
- After tool calls, summarize actions and results.
- If a tool is required, do not answer from prior memory alone; re-gather current context.

Web Search Workflow:
- Search once unless explicitly told otherwise.
- Fetch full pages with web_fetch before answering; never rely on titles/snippets alone.
- Prefer authoritative sources; for complex topics, use 2-3 good sources.
- Use prefer_readable=true when available.
- If a fetch is poor or fails, try another result.
- Synthesize across sources, not just one page.

HTML Rendering:
- To render HTML in chat, emit raw HTML in the markdown body. Never include comments or non-renderable HTML.
- Do not fence or indent renderable HTML unless the user wants source code only.
- For rendered examples, use semantic HTML with a top-level div and inline styles.
- NEVER nest divs. Prefer simple structures with inline styles for layout and presentation. You can use multiple sibling divs for complex layouts, but do not create nested div structures.
- Do not add background colors, borders, or other styling that may not fit the user's interface. Focus on clean, semantic HTML that can adapt to different environments.
- Use SVG with SMIL animations and CSS 3D transforms for dynamic visuals when possible.
- Never include <script>, event handlers, forms, iframes, or external embeds.
- If both live output and source are useful, emit raw HTML first, then a fenced html block.
`, workdir)
	if trimmed := strings.TrimSpace(override); trimmed != "" {
		base = combinePromptSections(base, trimmed)
	}

	// Always append memory instructions at the end
	return EnsureMemoryInstructions(base)
}

func combinePromptSections(base, addition string) string {
	base = strings.TrimSpace(base)
	addition = strings.TrimSpace(addition)
	switch {
	case base == "":
		return addition
	case addition == "":
		return base
	default:
		return base + "\n\n" + addition
	}
}

// RenderSkillsForProject builds a markdown "## Skills" section from SKILL.md files
// discovered directly under the project's .skills folder. Only metadata is
// injected to keep context small. Returns empty string if no skills are found.
func RenderSkillsForProject(projectDir string) string {
	if strings.TrimSpace(projectDir) == "" {
		return ""
	}

	projectID, gen, skillsGen := readGenerations(projectDir)
	cacheKey := projectID
	if cacheKey == "" {
		cacheKey = projectDir
	}

	cached, err := skillsCache.GetOrLoad(cacheKey, gen, skillsGen, func() (*skills.CachedSkills, error) {
		outcome := skills.LoadFromDir(projectDir)

		log.Debug().
			Str("projectDir", projectDir).
			Int("skillsFound", len(outcome.Skills)).
			Int("errors", len(outcome.Errors)).
			Msg("skills_loader_result")

		for _, e := range outcome.Errors {
			log.Debug().Str("path", e.Path).Str("error", e.Message).Msg("skills_loader_error")
		}

		prompt := renderSkillsSection(outcome.Skills)
		if prompt == "" {
			return nil, nil
		}

		return &skills.CachedSkills{
			Generation:       gen,
			SkillsGeneration: skillsGen,
			Skills:           outcome.Skills,
			RenderedPrompt:   prompt,
		}, nil
	})

	if err != nil || cached == nil {
		if err != nil {
			log.Debug().Err(err).Msg("skills_cache_load_failed")
		}
		return ""
	}

	return cached.RenderedPrompt
}

func renderSkillsSection(skillsList []skills.Metadata) string {
	if len(skillsList) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Skills\n")
	b.WriteString("These skills are discovered from the project's .skills folder. Each entry includes a name, description, and file path.\n")
	for _, s := range skillsList {
		desc := s.Description
		if strings.TrimSpace(s.ShortDescription) != "" {
			desc = s.ShortDescription
		}
		fmt.Fprintf(&b, "- %s: %s (file: %s)\n", s.Name, desc, s.Path)
	}

	b.WriteString("- Trigger rules: If the user names a skill (with $skill-name or plain text) OR the task matches a skill description, use it for that turn. Multiple mentions mean use them all.\n")
	b.WriteString("- Progressive disclosure: After selecting a skill, open its SKILL.md; load additional files (references/, scripts/, assets/) only as needed.\n")
	b.WriteString("- Missing/blocked: If a named skill path cannot be read, say so briefly and continue with a fallback.\n")
	b.WriteString("- Context hygiene: Keep context small—summarize long files, avoid bulk-loading references, and only load variant-specific files when relevant.\n")
	return b.String()
}

type projectGenerations struct {
	ID               string `json:"id"`
	Generation       int64  `json:"generation"`
	SkillsGeneration int64  `json:"skillsGeneration"`
}

func readGenerations(projectDir string) (string, int64, int64) {
	// Prefer sync-manifest (ephemeral workspaces) then project metadata (legacy).
	paths := []string{
		filepath.Join(projectDir, ".meta", "sync-manifest.json"),
		filepath.Join(projectDir, ".meta", "project.json"),
	}

	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var meta projectGenerations
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
		}
		return meta.ID, meta.Generation, meta.SkillsGeneration
	}
	return "", 0, 0
}
