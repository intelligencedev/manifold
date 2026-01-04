package llm

import "context"

// CompactionItem represents the opaque state returned by the Responses compaction endpoint.
type CompactionItem struct {
	ID               string `json:"id,omitempty"`
	EncryptedContent string `json:"encrypted_content"`
}

// CompactionProvider exposes Responses API compaction for compatible providers.
type CompactionProvider interface {
	Compact(ctx context.Context, msgs []Message, model string, previous *CompactionItem) (*CompactionItem, error)
}
