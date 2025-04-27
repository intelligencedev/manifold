package agent

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
)

// UpperTool converts text to uppercase
type UpperTool struct{}

func (UpperTool) Describe() ToolSpec {
	return ToolSpec{
		Name:        "upper",
		Description: "Convert text to UPPERCASE",
		Parameters: map[string]any{
			"text": map[string]string{
				"type":        "string",
				"description": "The text to convert to uppercase",
			},
		},
	}
}

func (UpperTool) Execute(_ context.Context, args map[string]any) (any, error) {
	text, ok := args["text"].(string)
	if !ok {
		return nil, fmt.Errorf("text parameter must be a string")
	}
	return strings.ToUpper(text), nil
}

// LowerTool converts text to lowercase
type LowerTool struct{}

func (LowerTool) Describe() ToolSpec {
	return ToolSpec{
		Name:        "lower",
		Description: "Convert text to lowercase",
		Parameters: map[string]any{
			"text": map[string]string{
				"type":        "string",
				"description": "The text to convert to lowercase",
			},
		},
	}
}

func (LowerTool) Execute(_ context.Context, args map[string]any) (any, error) {
	text, ok := args["text"].(string)
	if !ok {
		return nil, fmt.Errorf("text parameter must be a string")
	}
	return strings.ToLower(text), nil
}

// CountWordsTool counts words in a text
type CountWordsTool struct{}

func (CountWordsTool) Describe() ToolSpec {
	return ToolSpec{
		Name:        "countWords",
		Description: "Count the number of words in a text",
		Parameters: map[string]any{
			"text": map[string]string{
				"type":        "string",
				"description": "The text to count words in",
			},
		},
	}
}

func (CountWordsTool) Execute(_ context.Context, args map[string]any) (any, error) {
	text, ok := args["text"].(string)
	if !ok {
		return nil, fmt.Errorf("text parameter must be a string")
	}

	// Split by whitespace and count non-empty words
	words := strings.Fields(text)
	return len(words), nil
}

// FetchWebTool fetches content from a URL
type FetchWebTool struct{}

func (FetchWebTool) Describe() ToolSpec {
	return ToolSpec{
		Name:        "fetchWeb",
		Description: "Fetch content from a URL",
		Parameters: map[string]any{
			"url": map[string]string{
				"type":        "string",
				"description": "The URL to fetch content from",
			},
		},
	}
}

func (FetchWebTool) Execute(_ context.Context, args map[string]any) (any, error) {
	url, ok := args["url"].(string)
	if !ok {
		return nil, fmt.Errorf("url parameter must be a string")
	}

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return string(body), nil
}

// RegisterBuiltinTools registers built-in tools to the registry
func RegisterBuiltinTools(registry *Registry) {
	registry.Register("upper", UpperTool{})
	registry.Register("lower", LowerTool{})
	registry.Register("countWords", CountWordsTool{})
	registry.Register("fetchWeb", FetchWebTool{})
}
