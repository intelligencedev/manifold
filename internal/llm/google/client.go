package google

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	genai "google.golang.org/genai"

	"manifold/internal/config"
	"manifold/internal/llm"
)

// ErrToolsNotSupported indicates that Google provider currently does not support tool calling.
var ErrToolsNotSupported = errors.New("google provider: tool calling is not supported")

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
	if len(tools) > 0 {
		return llm.Message{}, ErrToolsNotSupported
	}
	contents, err := toContents(msgs)
	if err != nil {
		return llm.Message{}, err
	}

	resp, err := c.client.Models.GenerateContent(ctx, c.pickModel(model), contents, &genai.GenerateContentConfig{
		HTTPOptions: &c.httpOptions,
	})
	if err != nil {
		return llm.Message{}, err
	}
	return messageFromResponse(resp)
}

func (c *Client) ChatStream(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string, h llm.StreamHandler) error {
	if len(tools) > 0 {
		return ErrToolsNotSupported
	}
	contents, err := toContents(msgs)
	if err != nil {
		return err
	}

	stream := c.client.Models.GenerateContentStream(ctx, c.pickModel(model), contents, &genai.GenerateContentConfig{
		HTTPOptions: &c.httpOptions,
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

func toContents(msgs []llm.Message) ([]*genai.Content, error) {
	if len(msgs) == 0 {
		return nil, fmt.Errorf("messages required")
	}
	contents := make([]*genai.Content, 0, len(msgs))
	for _, m := range msgs {
		role := strings.ToLower(strings.TrimSpace(m.Role))
		switch role {
		case "", "user", "system":
			role = genai.RoleUser
		case "assistant":
			role = genai.RoleModel
		default:
			return nil, fmt.Errorf("unsupported role for google provider: %s", m.Role)
		}
		text := m.Content
		if role == genai.RoleUser && strings.ToLower(strings.TrimSpace(m.Role)) == "system" {
			text = "[system] " + text
		}
		parts := []*genai.Part{{Text: text}}
		contents = append(contents, &genai.Content{
			Role:  role,
			Parts: parts,
		})
	}
	return contents, nil
}

func messageFromResponse(resp *genai.GenerateContentResponse) (llm.Message, error) {
	if resp == nil || len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return llm.Message{}, fmt.Errorf("empty response from google provider")
	}
	content := resp.Candidates[0].Content
	var sb strings.Builder
	for _, part := range content.Parts {
		if part == nil {
			continue
		}
		if part.Text != "" {
			sb.WriteString(part.Text)
		}
	}

	return llm.Message{
		Role:    "assistant",
		Content: sb.String(),
	}, nil
}
