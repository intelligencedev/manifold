package textsplitters

import "fmt"

// Kind identifies a splitter strategy.
type Kind string

const (
	// KindFixed selects the fixed-length splitter.
	KindFixed Kind = "fixed"
)

// Unit indicates what a splitter measures when computing chunk sizes.
type Unit string

const (
	// UnitChars splits by Unicode characters (runes).
	UnitChars Unit = "chars"
	// UnitTokens splits by tokens, as defined by a Tokenizer implementation.
	UnitTokens Unit = "tokens"
)

// Config configures a splitter. The Kind selects the concrete strategy and the
// corresponding sub-config should be populated.
type Config struct {
	Kind  Kind
	Fixed FixedConfig
}

// NewFromConfig constructs a Splitter from a Config.
func NewFromConfig(c Config) (Splitter, error) {
	switch c.Kind {
	case KindFixed:
		return newFixedSplitter(c.Fixed)
	default:
		return nil, fmt.Errorf("unknown splitter kind: %q", c.Kind)
	}
}
