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

// CompactionCapabilityProvider lets a client expose whether its current
// endpoint/model configuration actually supports native compaction.
type CompactionCapabilityProvider interface {
	SupportsCompaction() bool
}

// ProviderSupportsCompaction reports whether a provider can be used for native
// compacted conversation state. Providers that implement Compact but not an
// explicit capability method are treated as capable for backward compatibility.
func ProviderSupportsCompaction(provider Provider) bool {
	if provider == nil {
		return false
	}
	if capability, ok := provider.(CompactionCapabilityProvider); ok {
		return capability.SupportsCompaction()
	}
	_, ok := provider.(CompactionProvider)
	return ok
}

// ProviderCompactor returns the compaction implementation when the provider's
// current configuration supports native compaction.
func ProviderCompactor(provider Provider) (CompactionProvider, bool) {
	if !ProviderSupportsCompaction(provider) {
		return nil, false
	}
	compactor, ok := provider.(CompactionProvider)
	return compactor, ok
}
