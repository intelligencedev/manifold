// Package tokenizer provides text tokenization for LLM inference.
//
// The tokenizer package implements various tokenization strategies:
//   - tiktoken: BPE tokenizer used by GPT-3/GPT-4 (cl100k_base, p50k_base)
//   - BPE: Byte-Pair Encoding from HuggingFace tokenizer.json
//   - Chat templates: Format messages for conversational models
//
// Supported formats:
//   - ChatML: <|im_start|>role\ncontent<|im_end|> (DeepSeek, OpenAI)
//   - LLaMA: [INST] user message [/INST] assistant response
//   - Mistral: Similar to LLaMA with variations
//
// Example usage:
//
//	// Load tiktoken
//	tok, err := tiktoken.NewTiktoken("cl100k_base")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Encode text
//	tokens, err := tok.Encode("Hello, world!")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Decode tokens
//	text, err := tok.Decode(tokens)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Apply chat template
//	messages := []ChatMessage{
//	    {Role: "system", Content: "You are helpful."},
//	    {Role: "user", Content: "Hi!"},
//	}
//	prompt := ApplyChatMLTemplate(messages)
package tokenizer
