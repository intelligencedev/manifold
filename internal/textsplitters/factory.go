package textsplitters

import "fmt"

// Kind identifies a splitter strategy.
type Kind string

const (
	// KindFixed selects the fixed-length splitter.
	KindFixed Kind = "fixed"
	// KindSentences groups along sentence boundaries up to a target size.
	KindSentences Kind = "sentences"
	// KindParagraphs groups along paragraph boundaries up to a target size.
	KindParagraphs Kind = "paragraphs"
	// KindMarkdown splits by Markdown headings, then groups within sections.
	KindMarkdown Kind = "markdown"
	// KindCode splits code by function/class blocks when possible.
	KindCode Kind = "code"
	// KindSemantic creates segments at semantic breakpoints, then groups.
	KindSemantic Kind = "semantic"
	// KindTextTiling applies a simplified TextTiling-like segmentation.
	KindTextTiling Kind = "texttiling"
	// KindRollingSentences creates rolling windows of N sentences.
	KindRollingSentences Kind = "rolling_sentences"
	// KindHybrid merges sentences up to a target size at natural boundaries.
	KindHybrid Kind = "hybrid"
	// KindLayout applies simple layout-aware heuristics.
	KindLayout Kind = "layout"
	// KindRecursive applies hierarchical splitting: headings -> paragraphs -> sentences -> fixed.
	KindRecursive Kind = "recursive"
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
	Kind       Kind
	Fixed      FixedConfig
	Boundary   BoundaryConfig
	Markdown   MarkdownConfig
	Code       CodeConfig
	Semantic   SemanticConfig
	TextTiling TextTilingConfig
	Rolling    RollingConfig
	Layout     LayoutConfig
	Recursive  RecursiveConfig
}

// NewFromConfig constructs a Splitter from a Config.
func NewFromConfig(c Config) (Splitter, error) {
	switch c.Kind {
	case KindFixed:
		return newFixedSplitter(c.Fixed)
	case KindSentences:
		return newSentenceSplitter(c.Boundary)
	case KindParagraphs:
		return newParagraphSplitter(c.Boundary)
	case KindMarkdown:
		return newMarkdownSplitter(c.Markdown)
	case KindCode:
		return newCodeSplitter(c.Code)
	case KindSemantic:
		return newSemanticSplitter(c.Semantic)
	case KindTextTiling:
		return newTextTilingSplitter(c.TextTiling)
	case KindRollingSentences:
		return newRollingSentenceSplitter(c.Rolling)
	case KindHybrid:
		return newHybridSplitter(c.Boundary)
	case KindLayout:
		return newLayoutSplitter(c.Layout)
	case KindRecursive:
		return newRecursiveSplitter(c.Recursive)
	default:
		return nil, fmt.Errorf("unknown splitter kind: %q", c.Kind)
	}
}
