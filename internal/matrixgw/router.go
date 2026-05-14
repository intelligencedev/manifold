package matrixgw

import (
	"regexp"
	"sort"
	"strings"
)

// RoomConfig describes how a Matrix room should route inbound messages.
type RoomConfig struct {
	DefaultTarget    string
	AllowUnmentioned bool
	Mentions         map[string]string
}

// RoutedMessage is the routing decision for a single inbound Matrix message.
type RoutedMessage struct {
	Target string
	Prompt string
}

// RouteMessage selects the route target for a Matrix message and derives the
// prompt text that should be passed into chat execution.
func RouteMessage(cfg RoomConfig, body string) (RoutedMessage, bool) {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return RoutedMessage{}, false
	}

	for _, alias := range sortedAliases(cfg.Mentions) {
		prompt, matched := promptFromMessage(trimmed, alias)
		if !matched {
			continue
		}

		target := strings.TrimSpace(cfg.Mentions[alias])
		if target == "" {
			continue
		}

		return RoutedMessage{
			Target: target,
			Prompt: prompt,
		}, true
	}

	if !cfg.AllowUnmentioned {
		return RoutedMessage{}, false
	}

	target := strings.TrimSpace(cfg.DefaultTarget)
	if target == "" {
		return RoutedMessage{}, false
	}

	return RoutedMessage{
		Target: target,
		Prompt: trimmed,
	}, true
}

func sortedAliases(mentions map[string]string) []string {
	aliases := make([]string, 0, len(mentions))
	for alias := range mentions {
		alias = strings.TrimSpace(alias)
		if alias == "" {
			continue
		}
		aliases = append(aliases, alias)
	}

	sort.Slice(aliases, func(i, j int) bool {
		if len(aliases[i]) == len(aliases[j]) {
			return aliases[i] < aliases[j]
		}
		return len(aliases[i]) > len(aliases[j])
	})

	return aliases
}

func promptFromMessage(body, prefix string) (prompt string, matched bool) {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return "", false
	}

	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return trimmed, true
	}

	if isInlineMentionPrefix(prefix) {
		if !containsStandalonePrefix(trimmed, prefix) {
			return "", false
		}
		return trimmed, true
	}

	if !strings.HasPrefix(trimmed, prefix) {
		return "", false
	}

	return strings.TrimSpace(strings.TrimPrefix(trimmed, prefix)), true
}

func isInlineMentionPrefix(prefix string) bool {
	return strings.HasPrefix(strings.TrimSpace(prefix), "@")
}

func containsStandalonePrefix(body, prefix string) bool {
	pattern := `(^|[^[:alnum:]_])` + regexp.QuoteMeta(prefix) + `($|[^[:alnum:]_])`
	return regexp.MustCompile(pattern).FindStringIndex(body) != nil
}
