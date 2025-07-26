//go:build claude

package documents

// ClaudeTokenizer is compiled only with the `claude` tag.
// Real implementation should wrap the Claude tokenizer.
type ClaudeTokenizer struct{}

func (ClaudeTokenizer) Count(s string) int { return 0 }
func (ClaudeTokenizer) Name() string       { return "claude" }
