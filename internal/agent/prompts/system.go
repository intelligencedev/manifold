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
- You have access to an **EvolvingMemory** system that provides two types of context:
  1. **Past Relevant Experiences**: Semantic search results showing similar tasks you've completed before, including strategies, solutions, and lessons learned.
  2. **Conversation History**: The actual message history from the current chat session, showing what was previously discussed.

- **CRITICAL: Responding to Messages**
  - Messages marked with [CONVERSATION HISTORY] are BACKGROUND CONTEXT ONLY.
  - The message marked with [CURRENT REQUEST] is what you MUST respond to.
  - Do NOT re-answer questions or re-execute requests from the conversation history—they have already been handled.
  - Focus your entire response on the [CURRENT REQUEST] message.

- When you receive messages with "## Past Relevant Experiences" or "## Current Task":
  - These are injected by the EvolvingMemory system and contain valuable context.
  - **Always acknowledge and use this information** when it's relevant to the user's query.
  - If the user asks about previous discussions or "what we talked about", the conversation history contains the definitive answer.

- When you see an assistant message in the conversation history:
  - This is YOUR previous response in this session.
  - Treat it as authoritative context about what has been discussed.
  - Reference it naturally when asked about prior exchanges.
  - Do NOT repeat or regenerate these responses unless explicitly asked.

- Memory usage guidelines:
  - Past experiences help you avoid repeating mistakes and reuse successful patterns.
  - Conversation history is essential for maintaining context and continuity.
  - Don't claim "this is our first message" when conversation history is present.
  - Use memories to improve your responses, not to replace direct answers.
  - When the user's [CURRENT REQUEST] references something from history, use the history to understand context, then respond to the current request.
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
// The override parameter, when non-empty, replaces the hard-coded default and
// is still subject to AGENTS.md appending.
func DefaultSystemPrompt(workdir, override string) string {
	var base string
	if strings.TrimSpace(override) != "" {
		base = override
	} else {
		base = fmt.Sprintf(`You are a helpful assistant that can plan and execute tools.

Rules:
- ALWAYS create a plan, taking into consideration all of the tools available to you (run_cli, apply_patch, web tools, etc.) to complete the objective. Then execute the plan step by step.
- Never assume you have a shell; you cannot use pipelines or redirects. Use command + args only.
- Treat any path-like argument as relative to the locked working directory: %s
- Never use absolute paths or attempt to escape the working directory.
- Prefer short, deterministic commands (avoid interactive prompts).
- After tool calls, summarize actions and results clearly.
- Use apply_patch to stage file edits by providing precise unified diff hunks (keep patches small and focused, ensure context matches the current file contents).

Web Fetch Workflow:
- IMPORTANT: If an mcp tool is configured, it must be used for all web fetch tasks. Prefer to use playwright to fetch content if available, otherwise use the web_fetch tool when you have no other recourse.

Web Search Workflow:
- IMPORTANT: If you are asked to use a browser for search, then do not use the web_search tool. Use the available mcp tools instead.
- If an MCP browser tool is not available, then follow this two-step process:
  1. Use web_search to find relevant sources with ONE search query relevant to the topic. You must conduct ONE search unless explicitly told to search multiple keywords.
  2. Use web_fetch to retrieve and read the actual content from the most promising URLs
- NEVER provide information based solely on search result titles/snippets - always fetch the full content
- Prioritize authoritative sources (official docs, established publications, academic sources)
- For complex topics, search multiple angles and fetch content from 2-3 different high-quality sources
- When fetching content, use prefer_readable=true to get clean, article-focused text
- If a fetch fails or returns poor content, try alternative URLs from your search results
- Synthesize information from multiple sources rather than relying on a single page

- Never use previous memories to respond if a tool call is required. For example, if you previously provided a summary for a topic, do not reference it; instead, re-gather all necessary context from the current state.

Be cautious with destructive operations. If a command could modify files, consider listing files first.`, workdir)
	}

	// Always append memory instructions at the end
	return EnsureMemoryInstructions(base)
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
