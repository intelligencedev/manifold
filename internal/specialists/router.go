package specialists

import (
	"regexp"
	"strings"

	"singularityio/internal/config"
)

// Route returns the name of the first matching specialist for the given text,
// or "" if no routes match.
func Route(routes []config.SpecialistRoute, text string) string {
	if text == "" || len(routes) == 0 {
		return ""
	}
	lc := strings.ToLower(text)
	for _, r := range routes {
		// contains checks
		for _, c := range r.Contains {
			c = strings.ToLower(strings.TrimSpace(c))
			if c != "" && strings.Contains(lc, c) {
				return r.Name
			}
		}
		// regex checks
		for _, pat := range r.Regex {
			pat = strings.TrimSpace(pat)
			if pat == "" {
				continue
			}
			re, err := regexp.Compile(pat)
			if err != nil {
				continue
			}
			if re.MatchString(text) {
				return r.Name
			}
		}
	}
	return ""
}
