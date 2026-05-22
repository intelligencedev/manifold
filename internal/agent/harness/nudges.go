package harness

import (
	"fmt"
	"strings"
)

func unknownToolNudge(unknown []string, allowed []string) string {
	var b strings.Builder
	b.WriteString("Your previous response called an unavailable tool: ")
	b.WriteString(strings.Join(unknown, ", "))
	b.WriteString(". Retry by calling only available tools")
	if len(allowed) > 0 {
		b.WriteString(": ")
		b.WriteString(strings.Join(allowed, ", "))
	}
	b.WriteString(".")
	return b.String()
}

func workflowTextNudge(terminalTools []string) string {
	if len(terminalTools) == 0 {
		return "Do not answer with bare text yet. Retry by calling the appropriate tool for this workflow step."
	}
	return fmt.Sprintf("Do not answer with bare text yet. This workflow must finish through one of these terminal tools: %s.", strings.Join(terminalTools, ", "))
}

func requiredStepsNudge(tool string, missing []string) string {
	return fmt.Sprintf("The terminal tool %q cannot be called yet. Complete the required step(s) first: %s.", tool, strings.Join(missing, ", "))
}

func prerequisiteNudge(tool string, prereq Prerequisite) string {
	if prereq.MatchArg == "" {
		return fmt.Sprintf("The tool %q cannot be called yet. First call %q successfully.", tool, prereq.Tool)
	}
	return fmt.Sprintf("The tool %q cannot be called yet. First call %q successfully with the same %q argument.", tool, prereq.Tool, prereq.MatchArg)
}

// ToolErrorNudge tells the model to recover from a failed tool execution.
func ToolErrorNudge(tool string, err ToolError) string {
	detail := strings.TrimSpace(err.Message)
	if detail == "" {
		detail = "the tool returned an error"
	}
	return fmt.Sprintf("The tool %q failed: %s. Retry with corrected arguments or choose another valid tool.", tool, detail)
}
