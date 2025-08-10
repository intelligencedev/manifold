package cli

// buildSchema constructs the OpenAI-compatible JSON schema for run_cli.
func buildSchema(t *tool) map[string]any {
    max := 30 // default; the ExecutorImpl enforces its own max timeout too
    return map[string]any{
        "name":        "run_cli",
        "description": "Execute a CLI command in a restricted working directory (no shell, no absolute paths).",
        "parameters": map[string]any{
            "type": "object",
            "properties": map[string]any{
                "command": map[string]any{"type": "string", "description": "Bare binary name (e.g., ls, git, go)."},
                "args": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
                "timeout_seconds": map[string]any{"type": "integer", "minimum": 1, "maximum": max},
                "stdin": map[string]any{"type": "string"},
            },
            "required": []string{"command"},
        },
    }
}

