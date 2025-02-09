package documents

import (
	"errors"
	"regexp"
)

// RecursiveCharacterTextSplitter is a struct that represents a text splitter
// that splits text based on recursive character separators.
type RecursiveCharacterTextSplitter struct {
	Separators       []string
	KeepSeparator    bool
	IsSeparatorRegex bool
	ChunkSize        int
	OverlapSize      int
	LengthFunction   func(string) int
}

// Language is a type that represents a programming language.
type Language string

const (
	PYTHON   Language = "PYTHON"
	GO       Language = "GO"
	HTML     Language = "HTML"
	JS       Language = "JS"
	TS       Language = "TS"
	MARKDOWN Language = "MARKDOWN"
	JSON     Language = "JSON"
	DEFAULT  Language = "DEFAULT"
)

// SplitTextByCount splits the given text into chunks of the given size.
func SplitTextByCount(text string, size int) []string {
	// Slice the string into chunks of the specified size
	var chunks []string
	for i := 0; i < len(text); i += size {
		end := i + size
		if end > len(text) {
			end = len(text)
		}
		chunks = append(chunks, text[i:end])
	}
	return chunks
}

// SplitText splits the given text using a simple chunk-based approach if the language is not specifically defined.
// OR, it uses the recursive splitting approach if the language is defined.
func (r *RecursiveCharacterTextSplitter) SplitText(text string) []string {
	if len(r.Separators) > 0 {
		// Use the recursive splitting mechanism if separators are defined
		return r.splitTextHelper(text, r.Separators) // Corrected: Use all separators
	}
	// Use a simple character count-based splitting mechanism
	return SplitTextByCount(text, r.ChunkSize)
}

// FromLanguage creates a RecursiveCharacterTextSplitter based on the given language.
// If the language is not a special case, it will default to simple chunk-based splitting.
func FromLanguage(language Language) (*RecursiveCharacterTextSplitter, error) {
	// If language is not DEFAULT, create a RecursiveCharacterTextSplitter with specific settings
	if language != DEFAULT {
		separators, err := GetSeparatorsForLanguage(language)
		if err != nil {
			return nil, err
		}
		return &RecursiveCharacterTextSplitter{
			Separators:       separators,
			ChunkSize:        1000, // Add a default chunk size, even for language-specific splitters
			OverlapSize:      100,
			IsSeparatorRegex: true,
			LengthFunction:   func(s string) int { return len(s) }, // Add length function
		}, nil
	}

	// Fallback: for general text, create a simpler splitter that uses chunk sizes.
	return &RecursiveCharacterTextSplitter{
		ChunkSize:      1000, // Default chunk size
		OverlapSize:    100,
		LengthFunction: func(s string) int { return len(s) }, // Add length function
	}, nil
}

// GetSeparatorsForLanguage returns the separators for the given language.
func GetSeparatorsForLanguage(language Language) ([]string, error) {
	switch language {
	case PYTHON:
		return []string{"\nclass ", "\ndef ", "\n\n", "\n", " ", ""}, nil
	case GO:
		return []string{"\nfunc ", "\nvar ", "\nif ", "\n\n", "\n", " ", ""}, nil
	case HTML:
		return []string{"<div", "<p", "<h1", "<br", "<table", "", "\n"}, nil
	case JS, TS:
		return []string{"\nfunction ", "\nconst ", "\nlet ", "\nclass ", "\n\n", "\n", " ", ""}, nil
	case MARKDOWN:
		return []string{"\n#{1,6} ", "\n---+\n", "\n", " ", ""}, nil
	case JSON:
		return []string{"}\n", ""}, nil
	default:
		return nil, errors.New("unsupported language")
	}
}

// escapeString is a helper function that escapes special characters in a string.
func escapeString(s string) string {
	return regexp.QuoteMeta(s)
}

// splitTextWithRegex is a helper function that splits text using a regular expression separator.
func splitTextWithRegex(text string, separator string, keepSeparator bool) []string {
	sepPattern := regexp.MustCompile(separator)
	splits := sepPattern.Split(text, -1)
	if keepSeparator {
		matches := sepPattern.FindAllString(text, -1)
		result := make([]string, 0, len(splits)+len(matches))
		for i, split := range splits {
			result = append(result, split)
			if i < len(matches) {
				result = append(result, matches[i])
			}
		}
		return result
	}
	return splits
}

// Enforce chunk size strictly by splitting each chunk further if needed.
func (r *RecursiveCharacterTextSplitter) enforceChunkSize(chunks []string) []string {
	var result []string
	for _, chunk := range chunks {
		if len(chunk) > r.ChunkSize {
			// Split the chunk into smaller pieces of size `ChunkSize`
			subChunks := SplitTextByCount(chunk, r.ChunkSize)
			result = append(result, subChunks...)
		} else {
			result = append(result, chunk)
		}
	}
	return result
}

// Apply overlap to the chunks.
func (r *RecursiveCharacterTextSplitter) applyOverlap(chunks []string) []string {
	overlappedChunks := make([]string, 0)
	for i := 0; i < len(chunks)-1; i++ {
		currentChunk := chunks[i]
		nextChunk := chunks[i+1]

		// Ensure overlap does not go out of range
		overlapLength := min(len(nextChunk), r.OverlapSize)
		if overlapLength > len(nextChunk) {
			overlapLength = len(nextChunk)
		}

		nextChunkOverlap := nextChunk[:overlapLength]

		overlappedChunk := currentChunk + nextChunkOverlap
		overlappedChunks = append(overlappedChunks, overlappedChunk)
	}

	// Add the last chunk without any overlap
	if len(chunks) > 0 {
		overlappedChunks = append(overlappedChunks, chunks[len(chunks)-1])
	}

	return overlappedChunks
}

// splitTextHelper is a recursive helper function that splits text using the given separators.
func (r *RecursiveCharacterTextSplitter) splitTextHelper(text string, separators []string) []string {
	finalChunks := make([]string, 0)

	if len(separators) == 0 || r.LengthFunction(text) < r.ChunkSize {
		return []string{text} // Base case: no more separators or text is small enough
	}

	separator := separators[0]      //  Use separators in order
	newSeparators := separators[1:] // Remaining separators

	sepPattern := separator
	if !r.IsSeparatorRegex {
		sepPattern = escapeString(separator)
	}

	splits := splitTextWithRegex(text, sepPattern, r.KeepSeparator)

	for _, s := range splits {
		if r.LengthFunction(s) < r.ChunkSize {
			finalChunks = append(finalChunks, s)
		} else if len(newSeparators) > 0 {
			// If the split is too large, recursively split it with the remaining separators
			recursiveSplits := r.splitTextHelper(s, newSeparators)
			finalChunks = append(finalChunks, recursiveSplits...)
		} else {
			// If no more separators, split by count
			subChunks := SplitTextByCount(s, r.ChunkSize)
			finalChunks = append(finalChunks, subChunks...)
		}
	}

	return finalChunks
}

// Helper functions
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
