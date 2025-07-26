package documents

import (
	"bufio"
	"io"
	"strings"
)

// AdvancedSplitter provides more intelligent splitting based on code structure
type AdvancedSplitter struct {
	MaxTokens     int
	OverlapTokens int
	Lang          Language
	Tok           Tokenizer
	detector      *BoundaryDetector
}

// NewAdvancedSplitter creates a new advanced splitter with intelligent boundary detection
func NewAdvancedSplitter(maxTokens, overlapTokens int, lang Language) *AdvancedSplitter {
	return &AdvancedSplitter{
		MaxTokens:     maxTokens,
		OverlapTokens: overlapTokens,
		Lang:          lang,
		Tok:           RuneTokenizer{},
		detector:      NewBoundaryDetector(lang),
	}
}

// StructureAwareSplit performs intelligent splitting that preserves logical code boundaries
func (s *AdvancedSplitter) StructureAwareSplit(text string) []string {
	lines := strings.Split(text, "\n")
	var chunks []string
	var currentChunk strings.Builder
	var currentTokens int

	// Track context for better splitting decisions
	context := &SplitContext{
		braceDepth:   0,
		parenDepth:   0,
		bracketDepth: 0,
		inString:     false,
		inComment:    false,
		indentLevel:  0,
	}

	for i, line := range lines {
		lineTokens := s.Tok.Count(line) + 1 // +1 for newline

		// Update context based on current line
		s.updateContext(context, line)

		// Check if this is a logical boundary
		isBoundary := s.detector.IsBoundary(line)

		// Check if we should preserve this structure together
		shouldPreserve := s.shouldPreserveStructure(lines, i, context)

		// Decide whether to split
		shouldSplit := false
		if currentTokens+lineTokens > s.MaxTokens {
			if isBoundary || context.braceDepth == 0 {
				shouldSplit = true
			} else if !shouldPreserve {
				// Force split if we're way over the limit
				shouldSplit = currentTokens > s.MaxTokens+s.MaxTokens/5 // 20% over limit
			}
		} else if isBoundary && currentTokens > s.MaxTokens/2 {
			// Split on boundaries if we're at least halfway to the limit
			shouldSplit = true
		}

		if shouldSplit && currentChunk.Len() > 0 {
			chunks = append(chunks, strings.TrimSpace(currentChunk.String()))

			// Handle overlap intelligently
			overlapText := s.getIntelligentOverlap(currentChunk.String())
			currentChunk.Reset()
			if overlapText != "" {
				currentChunk.WriteString(overlapText)
				currentChunk.WriteString("\n")
				currentTokens = s.Tok.Count(overlapText)
			} else {
				currentTokens = 0
			}
		}

		currentChunk.WriteString(line)
		currentChunk.WriteString("\n")
		currentTokens += lineTokens
	}

	// Add the final chunk
	if currentChunk.Len() > 0 {
		chunks = append(chunks, strings.TrimSpace(currentChunk.String()))
	}

	return chunks
}

// SplitContext tracks parsing state for better splitting decisions
type SplitContext struct {
	braceDepth   int // {}
	parenDepth   int // ()
	bracketDepth int // []
	inString     bool
	inComment    bool
	indentLevel  int
}

// updateContext updates the parsing context based on the current line
func (s *AdvancedSplitter) updateContext(ctx *SplitContext, line string) {
	trimmed := strings.TrimSpace(line)

	// Update indentation level
	ctx.indentLevel = len(line) - len(strings.TrimLeft(line, " \t"))

	// Simple comment detection (language-specific)
	switch s.Lang {
	case Go, JavaScript, TypeScript, Java, CSharp, Rust, Cpp, C:
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") {
			ctx.inComment = true
		}
	case Python:
		if strings.HasPrefix(trimmed, "#") {
			ctx.inComment = true
		}
	case Shell:
		if strings.HasPrefix(trimmed, "#") {
			ctx.inComment = true
		}
	}

	// Count braces, parentheses, and brackets (simplified)
	for _, char := range line {
		switch char {
		case '{':
			ctx.braceDepth++
		case '}':
			ctx.braceDepth--
		case '(':
			ctx.parenDepth++
		case ')':
			ctx.parenDepth--
		case '[':
			ctx.bracketDepth++
		case ']':
			ctx.bracketDepth--
		}
	}
}

// shouldPreserveStructure determines if a code structure should be kept together
func (s *AdvancedSplitter) shouldPreserveStructure(lines []string, currentIndex int, ctx *SplitContext) bool {
	// Don't split in the middle of braces, parentheses, or brackets
	if ctx.braceDepth > 0 || ctx.parenDepth > 0 || ctx.bracketDepth > 0 {
		return true
	}

	// Language-specific preservation rules
	switch s.Lang {
	case Python:
		// Preserve indented blocks
		if currentIndex < len(lines)-1 {
			nextLine := strings.TrimSpace(lines[currentIndex+1])
			if nextLine != "" && strings.HasPrefix(lines[currentIndex+1], "    ") {
				return true
			}
		}
	case Go, Java, CSharp, JavaScript, TypeScript:
		// Preserve function bodies and class definitions
		currentLine := strings.TrimSpace(lines[currentIndex])
		if strings.Contains(currentLine, "{") && !strings.Contains(currentLine, "}") {
			return true
		}
	}

	return false
}

// getIntelligentOverlap creates contextually relevant overlap text
func (s *AdvancedSplitter) getIntelligentOverlap(text string) string {
	if s.OverlapTokens <= 0 {
		return ""
	}

	lines := strings.Split(text, "\n")
	if len(lines) == 0 {
		return ""
	}

	// Try to find important context to preserve
	importantLines := []string{}

	// Get important patterns for the language
	patterns := s.detector.GetImportantPatterns()

	// Look for recent function/class definitions to preserve as context
	for i := len(lines) - 1; i >= 0 && len(importantLines) < 3; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		// Check if this line matches important patterns
		for _, pattern := range patterns {
			if pattern.MatchString(line) {
				importantLines = append([]string{lines[i]}, importantLines...)
				break
			}
		}
	}

	// If no important lines found, fall back to simple token-based overlap
	if len(importantLines) == 0 {
		return lastTokens(text, s.Tok, s.OverlapTokens)
	}

	return strings.Join(importantLines, "\n")
}

// Stream provides the same interface as the basic splitter but with advanced logic
func (s *AdvancedSplitter) Stream(r io.Reader, emit func(Chunk) error) error {
	// Read all content first for structure-aware analysis
	scanner := bufio.NewScanner(r)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	text := strings.Join(lines, "\n")
	chunks := s.StructureAwareSplit(text)

	// Emit chunks with proper token counting
	start := 0
	for i, chunk := range chunks {
		tokens := s.Tok.Count(chunk)
		if err := emit(Chunk{
			Index:      i,
			Text:       chunk,
			StartToken: start,
			EndToken:   start + tokens,
		}); err != nil {
			return err
		}
		start += tokens - s.OverlapTokens
	}

	return nil
}
