package imagetool

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"singularityio/internal/llm"
	"singularityio/internal/llm/openai"
	"singularityio/internal/sandbox"
	"singularityio/internal/tools"
)

// ProviderFactory allows building a provider with an alternate base URL.
type ProviderFactory func(baseURL string) llm.Provider

type DescribeTool struct {
	Provider       llm.Provider
	Workdir        string
	DefaultModel   string
	NewWithBaseURL ProviderFactory
}

func NewDescribeTool(p llm.Provider, workdir, defaultModel string, f ProviderFactory) *DescribeTool {
	return &DescribeTool{Provider: p, Workdir: workdir, DefaultModel: defaultModel, NewWithBaseURL: f}
}

func (t *DescribeTool) Name() string { return "describe_image" }

func (t *DescribeTool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": "Describe an image file located under the locked working directory. The image will be sent to the LLM as an inline data URL.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":     map[string]any{"type": "string", "description": "Relative path to an image file under WORKDIR (e.g., images/photo.jpg)"},
				"prompt":   map[string]any{"type": "string", "description": "Optional additional instruction or question about the image"},
				"model":    map[string]any{"type": "string", "description": "Optional model override"},
				"base_url": map[string]any{"type": "string", "description": "Optional API base URL override"},
			},
			"required": []string{"path"},
		},
	}
}

func (t *DescribeTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct {
		Path    string `json:"path"`
		Prompt  string `json:"prompt"`
		Model   string `json:"model"`
		BaseURL string `json:"base_url"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}

	rel, err := sandbox.SanitizeArg(t.Workdir, args.Path)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	full := filepath.Join(t.Workdir, rel)
	f, err := os.Open(full)
	if err != nil {
		return map[string]any{"ok": false, "error": fmt.Sprintf("open: %v", err)}, nil
	}
	defer f.Close()

	// Read a bounded prefix to sniff content type, then the rest
	hdr := make([]byte, 512)
	n, _ := io.ReadFull(f, hdr)
	// reset to beginning
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	content, err := io.ReadAll(f)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	mime := http.DetectContentType(hdr[:n])

	b64 := base64.StdEncoding.EncodeToString(content)

	// Build messages
	sys := "You are a helpful image understanding assistant. Answer concisely and describe visual details, objects, colors, text, and any notable attributes."
	userContent := ""
	if args.Prompt != "" {
		userContent = args.Prompt + "\n\n"
	} else {
		userContent = "Describe the image below in plain text. Include objects, colors, scene, and any readable text."
	}
	// Use markdown image data URL so the assistant can detect an image in the text
	userContent = userContent + "\n\n![image](data:" + mime + ";base64," + b64 + ")\n"

	msgs := []llm.Message{{Role: "system", Content: sys}, {Role: "user", Content: userContent}}

	// Prefer provider from context if the caller (agent/specialist) propagated one.
	p := t.Provider
	if ctxProvider := tools.ProviderFromContext(ctx); ctxProvider != nil {
		p = ctxProvider
	}
	// If caller requested a baseURL override, build a new provider using the
	// supplied factory so the tool uses the correct base URL and headers.
	if args.BaseURL != "" && t.NewWithBaseURL != nil {
		if np := t.NewWithBaseURL(args.BaseURL); np != nil {
			p = np
		}
	}
	model := args.Model
	// Don't fallback to t.DefaultModel - let the provider use its own default model
	// This ensures that when providers are propagated from context (specialists/agents),
	// tools use the same model as the invoking agent/specialist.

	// Try to use OpenAI-specific image attachment method if available
	if openaiClient, ok := p.(*openai.Client); ok {
		out, err := openaiClient.ChatWithImageAttachment(ctx, msgs, mime, b64, nil, model)
		if err != nil {
			return map[string]any{"ok": false, "error": err.Error()}, nil
		}
		return map[string]any{"ok": true, "output": out.Content}, nil
	}

	// Fallback to original data URL method for other providers
	out, err := p.Chat(ctx, msgs, nil, model)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	return map[string]any{"ok": true, "output": out.Content}, nil
}
