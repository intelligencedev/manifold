//go:build openai

package documents

// This file provides an adapter for the OpenAI tokenizer when built with the
// `openai` build tag. To avoid mandatory dependency on the OpenAI SDK, this
// implementation is minimal and only compiles when the tag is provided.

// OpenAITokenizer is a stub for integration with the OpenAI tiktoken library.
type OpenAITokenizer struct{}

func (OpenAITokenizer) Count(s string) int { return 0 }
func (OpenAITokenizer) Name() string       { return "openai" }
