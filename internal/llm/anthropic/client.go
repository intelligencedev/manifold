package anthropic

import (
	"context"
	"errors"
	"net/http"

	"manifold/internal/config"
	"manifold/internal/llm"
)

// ErrNotImplemented identifies the current stub implementation.
var ErrNotImplemented = errors.New("anthropic provider is not implemented yet")

type Client struct {
	cfg        config.AnthropicConfig
	httpClient *http.Client
}

func New(cfg config.AnthropicConfig, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{cfg: cfg, httpClient: httpClient}
}

func (c *Client) Chat(context.Context, []llm.Message, []llm.ToolSchema, string) (llm.Message, error) {
	return llm.Message{}, ErrNotImplemented
}

func (c *Client) ChatStream(context.Context, []llm.Message, []llm.ToolSchema, string, llm.StreamHandler) error {
	return ErrNotImplemented
}
