package codeqa

import (
	"path/filepath"
	"regexp"
	"strings"
)

func MatchAny(path string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchGlob(path, pattern) {
			return true
		}
	}
	return false
}

func matchGlob(path string, pattern string) bool {
	cleanPath := filepath.ToSlash(strings.TrimSpace(path))
	cleanPattern := filepath.ToSlash(strings.TrimSpace(pattern))
	if cleanPath == "" || cleanPattern == "" {
		return false
	}
	if ok, err := filepath.Match(cleanPattern, cleanPath); err == nil && ok {
		return true
	}
	quoted := regexp.QuoteMeta(cleanPattern)
	quoted = strings.ReplaceAll(quoted, "\\*\\*", ".*")
	quoted = strings.ReplaceAll(quoted, "\\*", "[^/]*")
	quoted = strings.ReplaceAll(quoted, "\\?", ".")
	re := regexp.MustCompile("^" + quoted + "$")
	return re.MatchString(cleanPath)
}
