package prompts

import "fmt"

func DefaultSystemPrompt(workdir string) string {
    return fmt.Sprintf(`You are a helpful build/ops agent...
Rules:
- No shell; command + args only.
- Paths are relative to: %s
- Avoid destructive ops; prefer listing before modifying.
After tools: summarize actions & results.`, workdir)
}
