package util

import "unicode"

// CountTokens provides a rough token count suitable for estimating LLM usage.
// Punctuation is counted separately to improve accuracy over simple space-based splitting.
func CountTokens(s string) int {
	inWord := false
	count := 0
	for _, r := range s {
		if unicode.IsSpace(r) {
			if inWord {
				count++
				inWord = false
			}
		} else if unicode.IsPunct(r) {
			if inWord {
				count++
				inWord = false
			}
			count++
		} else {
			inWord = true
		}
	}
	if inWord {
		count++
	}
	return count
}
