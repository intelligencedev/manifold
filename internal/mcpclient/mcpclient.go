package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	mcppkg "github.com/modelcontextprotocol/go-sdk/mcp"

	"intelligence.dev/internal/config"
	"intelligence.dev/internal/tools"
	"intelligence.dev/internal/version"
)

// Manager holds active MCP client sessions and generated tool wrappers.
type Manager struct {
	sessions map[string]*mcppkg.ClientSession
}

// NewManager creates a new Manager.
func NewManager() *Manager { return &Manager{sessions: map[string]*mcppkg.ClientSession{}} }

// Close closes all active sessions.
func (m *Manager) Close() {
	for _, s := range m.sessions {
		_ = s.Close()
	}
}

// RegisterFromConfig connects to configured MCP servers, lists their tools, and
// registers wrappers into the provided registry. Tools are registered with names
// in the form "mcp_<server>_<tool>" to avoid collisions.
func (m *Manager) RegisterFromConfig(ctx context.Context, reg tools.Registry, mcpCfg config.MCPConfig) error {
	for _, srv := range mcpCfg.Servers {
		if strings.TrimSpace(srv.Command) == "" || strings.TrimSpace(srv.Name) == "" {
			continue
		}
		// Build command
		cmd := exec.Command(srv.Command, srv.Args...)
		// Merge env
		if len(srv.Env) > 0 {
			env := os.Environ()
			for k, v := range srv.Env {
				env = append(env, fmt.Sprintf("%s=%s", k, v))
			}
			cmd.Env = env
		}
		// Create client
		opts := &mcppkg.ClientOptions{}
		if srv.KeepAliveSeconds > 0 {
			opts.KeepAlive = time.Duration(srv.KeepAliveSeconds) * time.Second
		}
		client := mcppkg.NewClient(&mcppkg.Implementation{Name: "intelligence.dev", Version: version.Version}, opts)
		// Connect
		session, err := client.Connect(ctx, mcppkg.NewCommandTransport(cmd))
		if err != nil {
			// Don't fail entire setup; just skip this server.
			continue
		}
		m.sessions[srv.Name] = session

		// List all tools and register wrappers
		// Use iterator to fetch all pages.
		for tool, err := range session.Tools(ctx, nil) {
			if err != nil {
				break
			}
			reg.Register(&mcpTool{server: srv.Name, session: session, tool: tool})
		}
	}
	return nil
}

// mcpTool adapts an MCP tool to the local tools.Tool interface.
type mcpTool struct {
	server  string
	session *mcppkg.ClientSession
	tool    *mcppkg.Tool
}

func (t *mcpTool) Name() string {
	return sanitizeName("mcp_" + t.server + "_" + t.tool.Name)
}

func (t *mcpTool) JSONSchema() map[string]any {
	// Start with a safe default that OpenAI accepts: object with empty properties
	params := map[string]any{"type": "object", "properties": map[string]any{}}
	if t.tool.InputSchema != nil {
		if b, err := json.Marshal(t.tool.InputSchema); err == nil {
			var m map[string]any
			if json.Unmarshal(b, &m) == nil && m != nil {
				// Merge onto defaults
				for k, v := range m {
					params[k] = v
				}
			}
		}
	}
	// Enforce required fields for OpenAI tool schemas
	if params["type"] != "object" {
		params["type"] = "object"
	}
	if _, ok := params["properties"]; !ok || params["properties"] == nil {
		params["properties"] = map[string]any{}
	}
	// Normalize recursively to satisfy OpenAI's stricter requirements
	sanitizeSchema(params, "")
	return map[string]any{
		"description": t.tool.Description,
		"parameters":  params,
	}
}

// sanitizeSchema normalizes a JSON schema map in-place to meet OpenAI function
// tool requirements. It ensures:
// - object schemas always have a properties map
// - array schemas always have an items schema (defaults to {"type":"string"})
// - recursively sanitizes nested properties/items/oneOf/anyOf/allOf
func sanitizeSchema(s map[string]any, prop string) {
	// Helper to check type == value or contains value in array type
	hasType := func(v any, want string) bool {
		switch tt := v.(type) {
		case string:
			return tt == want
		case []any:
			for _, x := range tt {
				if xs, ok := x.(string); ok && xs == want {
					return true
				}
			}
		case []string:
			for _, xs := range tt {
				if xs == want {
					return true
				}
			}
		}
		return false
	}

	// Ensure object has properties
	if hasType(s["type"], "object") {
		if _, ok := s["properties"].(map[string]any); !ok {
			s["properties"] = map[string]any{}
		}
	}
	// Ensure arrays have items
	if hasType(s["type"], "array") {
		if _, ok := s["items"].(map[string]any); !ok {
			// Best-effort default to string list
			s["items"] = map[string]any{"type": "string"}
		}
	}
	// Recurse into properties
	if props, ok := s["properties"].(map[string]any); ok {
		for k, v := range props {
			if m, ok := v.(map[string]any); ok {
				sanitizeSchema(m, k)
			}
		}
	}
	// Recurse into items
	if it, ok := s["items"].(map[string]any); ok {
		sanitizeSchema(it, prop+"[]")
	}
	// Handle composition keywords
	for _, key := range []string{"oneOf", "anyOf", "allOf"} {
		if arr, ok := s[key].([]any); ok {
			for _, v := range arr {
				if m, ok := v.(map[string]any); ok {
					sanitizeSchema(m, prop)
				}
			}
		}
	}
	// Normalize required to []string, if given in other forms
	if req, ok := s["required"]; ok {
		switch rr := req.(type) {
		case []any:
			out := make([]string, 0, len(rr))
			for _, x := range rr {
				if xs, ok := x.(string); ok {
					out = append(out, xs)
				}
			}
			s["required"] = out
		}
	}
}

func (t *mcpTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args any
	if len(raw) > 0 {
		// accept any JSON as arguments; if not an object, server may reject
		_ = json.Unmarshal(raw, &args)
	}
	if args == nil {
		args = map[string]any{}
	}
	res, err := t.session.CallTool(ctx, &mcppkg.CallToolParams{Name: t.tool.Name, Arguments: args})
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	// Convert result into a compact JSON structure for our agent.
	// Try to extract textual content for convenience.
	texts := make([]string, 0, len(res.Content))
	for _, c := range res.Content {
		switch v := c.(type) {
		case *mcppkg.TextContent:
			texts = append(texts, v.Text)
		default:
			// content types will be preserved in raw form below
		}
	}
	out := map[string]any{
		"ok":         !res.IsError,
		"text":       strings.Join(texts, "\n"),
		"structured": res.StructuredContent,
	}
	// Also include raw content for completeness
	if b, err := json.Marshal(res.Content); err == nil {
		var anyc any
		if json.Unmarshal(b, &anyc) == nil {
			out["content"] = anyc
		}
	}
	return out, nil
}

func sanitizeName(s string) string {
	// Keep it simple: replace spaces and slashes with underscores
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, ":", "_")
	return s
}
