package terminal

func startSchema(manager *Manager) map[string]any {
	maxRuntime := defaultTerminalRuntimeSeconds
	if manager != nil && manager.cfg.MaxTerminalRuntimeSeconds > 0 {
		maxRuntime = manager.cfg.MaxTerminalRuntimeSeconds
	}
	return map[string]any{
		"name":        "terminal_start",
		"description": "Start a policy-controlled and sandboxed PTY-backed terminal in the current project workspace. Use run_cli for short foreground commands.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "Bare binary name, or a command with inline args. No shell is inserted automatically.",
				},
				"args": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Additional command arguments. Path-like args must stay inside the project workspace.",
				},
				"stdin": map[string]any{
					"type":        "string",
					"description": "Optional initial input to write immediately after the terminal starts.",
				},
				"timeout_seconds": map[string]any{
					"type":        "integer",
					"minimum":     1,
					"maximum":     maxRuntime,
					"description": "Maximum runtime for this terminal. Defaults to the configured maximum.",
				},
				"name": map[string]any{
					"type":        "string",
					"description": "Optional human-friendly terminal label.",
				},
				"cols": map[string]any{
					"type":        "integer",
					"minimum":     20,
					"maximum":     300,
					"description": "PTY column count. Defaults to 80.",
				},
				"rows": map[string]any{
					"type":        "integer",
					"minimum":     5,
					"maximum":     120,
					"description": "PTY row count. Defaults to 24.",
				},
			},
			"required": []string{"command"},
		},
	}
}

func readSchema(manager *Manager) map[string]any {
	maxBytes := defaultTerminalOutputBufferBytes
	if manager != nil && manager.cfg.TerminalOutputBufferBytes > 0 {
		maxBytes = manager.cfg.TerminalOutputBufferBytes
	}
	return map[string]any{
		"name":        "terminal_read",
		"description": "Read buffered output from a terminal session. Pass since_seq from the previous response to poll incrementally.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"terminal_id": map[string]any{
					"type":        "string",
					"description": "Terminal session ID returned by terminal_start or terminal_list.",
				},
				"since_seq": map[string]any{
					"type":        "integer",
					"minimum":     0,
					"description": "Return output chunks with seq greater than this value.",
				},
				"max_bytes": map[string]any{
					"type":        "integer",
					"minimum":     1,
					"maximum":     maxBytes,
					"description": "Maximum output bytes to return.",
				},
			},
			"required": []string{"terminal_id"},
		},
	}
}

func writeSchema() map[string]any {
	return map[string]any{
		"name":        "terminal_write",
		"description": "Write input to a running PTY-backed terminal session.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"terminal_id": map[string]any{"type": "string", "description": "Terminal session ID."},
				"input":       map[string]any{"type": "string", "description": "Bytes/text to write to terminal stdin."},
			},
			"required": []string{"terminal_id", "input"},
		},
	}
}

func stopSchema() map[string]any {
	return map[string]any{
		"name":        "terminal_stop",
		"description": "Stop a running terminal session. Sends a graceful signal first, then force-kills after a short grace period.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"terminal_id": map[string]any{"type": "string", "description": "Terminal session ID."},
				"signal": map[string]any{
					"type":        "string",
					"enum":        []string{"interrupt", "term", "kill"},
					"description": "Optional signal style. Defaults to interrupt.",
				},
			},
			"required": []string{"terminal_id"},
		},
	}
}

func listSchema() map[string]any {
	return map[string]any{
		"name":        "terminal_list",
		"description": "List terminal sessions visible to the current chat session and project workspace.",
		"parameters": map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}
}
