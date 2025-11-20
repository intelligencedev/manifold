package google

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	genai "google.golang.org/genai"

	"manifold/internal/config"
	"manifold/internal/llm"
)

type Client struct {
	client      *genai.Client
	model       string
	httpOptions genai.HTTPOptions
}

func New(cfg config.GoogleConfig, httpClient *http.Client) (*Client, error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = "gemini-1.5-flash"
	}

	httpOpts := genai.HTTPOptions{}
	if base := strings.TrimSpace(cfg.BaseURL); base != "" {
		httpOpts.BaseURL = strings.TrimSuffix(base, "/") + "/"
	}

	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:      strings.TrimSpace(cfg.APIKey),
		HTTPClient:  httpClient,
		HTTPOptions: httpOpts,
	})
	if err != nil {
		return nil, fmt.Errorf("init google client: %w", err)
	}

	return &Client{
		client:      client,
		model:       model,
		httpOptions: httpOpts,
	}, nil
}

func (c *Client) Chat(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string) (llm.Message, error) {
	contents, toolMap, err := toContents(msgs)
	if err != nil {
		return llm.Message{}, err
	}

	toolDecls, toolCfg, err := adaptTools(tools, toolMap)
	if err != nil {
		return llm.Message{}, err
	}

	resp, err := c.client.Models.GenerateContent(ctx, c.pickModel(model), contents, &genai.GenerateContentConfig{
		HTTPOptions: &c.httpOptions,
		Tools:       toolDecls,
		ToolConfig:  toolCfg,
	})
	if err != nil {
		return llm.Message{}, err
	}
	return messageFromResponse(resp)
}

func (c *Client) ChatStream(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string, h llm.StreamHandler) error {
	contents, toolMap, err := toContents(msgs)
	if err != nil {
		return err
	}

	toolDecls, toolCfg, err := adaptTools(tools, toolMap)
	if err != nil {
		return err
	}

	stream := c.client.Models.GenerateContentStream(ctx, c.pickModel(model), contents, &genai.GenerateContentConfig{
		HTTPOptions: &c.httpOptions,
		Tools:       toolDecls,
		ToolConfig:  toolCfg,
	})

	for resp, err := range stream {
		if err != nil {
			return err
		}
		msg, err := messageFromResponse(resp)
		if err != nil {
			return err
		}
		if h != nil && msg.Content != "" {
			h.OnDelta(msg.Content)
		}
		for _, tc := range msg.ToolCalls {
			if h != nil {
				h.OnToolCall(tc)
			}
		}
	}
	return nil
}

func (c *Client) pickModel(model string) string {
	m := strings.TrimSpace(model)
	if m == "" {
		return c.model
	}
	return m
}

func toContents(msgs []llm.Message) ([]*genai.Content, map[string]string, error) {
	if len(msgs) == 0 {
		return nil, nil, fmt.Errorf("messages required")
	}

	toolNamesByID := make(map[string]string)
	thoughtSigByID := make(map[string]string)
	var lastFuncName string
	var lastThoughtSig string
	contents := make([]*genai.Content, 0, len(msgs))
	for _, m := range msgs {
		role := strings.ToLower(strings.TrimSpace(m.Role))
		switch role {
		case "", "user", "system":
			role = genai.RoleUser
		case "assistant":
			role = genai.RoleModel
			for _, tc := range m.ToolCalls {
				if tc.ID != "" && tc.Name != "" {
					toolNamesByID[tc.ID] = tc.Name
					if tc.ThoughtSignature != "" {
						thoughtSigByID[tc.ID] = tc.ThoughtSignature
					}
				}
				if strings.TrimSpace(tc.Name) != "" {
					lastFuncName = tc.Name
					lastThoughtSig = tc.ThoughtSignature
				}
			}
		case "tool":
			// Tool responses are passed back to the model as function responses.
			name := toolNamesByID[m.ToolID]
			if name == "" {
				name = lastFuncName
				if name == "" {
					name = "tool_response"
				}
			}
			sig := thoughtSigByID[m.ToolID]
			if sig == "" {
				sig = lastThoughtSig
			}
			respMap := map[string]any{}
			if trimmed := strings.TrimSpace(m.Content); trimmed != "" {
				if err := json.Unmarshal([]byte(trimmed), &respMap); err != nil {
					respMap = map[string]any{"output": m.Content}
				}
			}
			part := genai.NewPartFromFunctionResponse(name, respMap)
			part.FunctionResponse.ID = m.ToolID
			if strings.TrimSpace(sig) != "" {
				part.ThoughtSignature = []byte(sig)
			}
			contents = append(contents, genai.NewContentFromParts([]*genai.Part{part}, genai.RoleUser))
			continue
		default:
			return nil, nil, fmt.Errorf("unsupported role for google provider: %s", m.Role)
		}
		text := m.Content
		if role == genai.RoleUser && strings.ToLower(strings.TrimSpace(m.Role)) == "system" {
			text = "[system] " + text
		}
		parts := []*genai.Part{}
		if strings.TrimSpace(text) != "" {
			parts = append(parts, &genai.Part{Text: text})
		}
		if role == genai.RoleModel {
			for _, tc := range m.ToolCalls {
				var args map[string]any
				if len(tc.Args) > 0 {
					_ = json.Unmarshal(tc.Args, &args)
				}
				if len(args) == 0 && len(tc.Args) > 0 {
					args = map[string]any{"input": string(tc.Args)}
				}
				p := genai.NewPartFromFunctionCall(tc.Name, args)
				if strings.TrimSpace(tc.ThoughtSignature) != "" {
					p.ThoughtSignature = []byte(tc.ThoughtSignature)
				}
				parts = append(parts, p)
			}
		}
		if len(parts) == 0 {
			continue
		}
		contents = append(contents, &genai.Content{
			Role:  role,
			Parts: parts,
		})
	}
	return contents, toolNamesByID, nil
}

func messageFromResponse(resp *genai.GenerateContentResponse) (llm.Message, error) {
	if resp == nil || len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return llm.Message{}, fmt.Errorf("empty response from google provider")
	}
	content := resp.Candidates[0].Content
	var sb strings.Builder
	var tcs []llm.ToolCall
	callIdx := 0
	for _, part := range content.Parts {
		if part == nil {
			continue
		}
		if part.Text != "" {
			sb.WriteString(part.Text)
		}
		if part.FunctionCall != nil {
			args, _ := json.Marshal(part.FunctionCall.Args)
			callIdx++
			id := part.FunctionCall.ID
			if strings.TrimSpace(id) == "" {
				id = "call-" + strconv.Itoa(callIdx)
			}
			tcs = append(tcs, llm.ToolCall{
				Name:             part.FunctionCall.Name,
				Args:             args,
				ID:               id,
				ThoughtSignature: string(part.ThoughtSignature),
			})
		}
	}

	return llm.Message{
		Role:    "assistant",
		Content: sb.String(),
		ToolCalls: func() []llm.ToolCall {
			if len(tcs) == 0 {
				return nil
			}
			return tcs
		}(),
	}, nil
}

func adaptTools(schemas []llm.ToolSchema, toolNames map[string]string) ([]*genai.Tool, *genai.ToolConfig, error) {
	if len(schemas) == 0 {
		return nil, nil, nil
	}
	fd := make([]*genai.FunctionDeclaration, 0, len(schemas))
	names := make([]string, 0, len(schemas))
	for _, s := range schemas {
		if strings.TrimSpace(s.Name) == "" {
			return nil, nil, fmt.Errorf("google provider: tool name required")
		}
		names = append(names, s.Name)
		fd = append(fd, &genai.FunctionDeclaration{
			Name:                 s.Name,
			Description:          s.Description,
			ParametersJsonSchema: s.Parameters,
		})
	}
	sort.Strings(names)
	// Use AUTO mode to let the model decide whether to call a function or respond with text.
	// This prevents infinite loops where the model repeatedly calls the same function.
	// Note: AllowedFunctionNames should only be set when mode is ANY, not AUTO.
	// See: https://ai.google.dev/gemini-api/docs/function-calling#function-calling-modes
	cfg := &genai.ToolConfig{
		FunctionCallingConfig: &genai.FunctionCallingConfig{
			Mode: genai.FunctionCallingConfigModeAuto,
			// AllowedFunctionNames is intentionally omitted in AUTO mode per API requirements
		},
	}
	tool := &genai.Tool{FunctionDeclarations: fd}
	return []*genai.Tool{tool}, cfg, nil
}
