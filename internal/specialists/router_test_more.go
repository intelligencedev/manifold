package specialists

import (
	"testing"

	"manifold/internal/config"
)

func TestRoute_EmptyAndNoMatch(t *testing.T) {
	if Route(nil, "something") != "" {
		t.Fatalf("expected empty when routes nil")
	}
	if Route([]config.SpecialistRoute{}, "x") != "" {
		t.Fatalf("expected empty when routes empty")
	}
	if Route([]config.SpecialistRoute{{Name: "a", Contains: []string{"nomatch"}}}, "hello") != "" {
		t.Fatalf("expected no match for contains")
	}
}

func TestRoute_ContainsMatchAndCaseInsensitive(t *testing.T) {
	routes := []config.SpecialistRoute{
		{Name: "caps", Contains: []string{"HELLO"}},
		{Name: "other", Contains: []string{"bye"}},
	}
	// should match case-insensitively
	if got := Route(routes, "well hello there"); got != "caps" {
		t.Fatalf("expected caps, got %s", got)
	}
}

func TestRoute_RegexMatchAndInvalidRegex(t *testing.T) {
	routes := []config.SpecialistRoute{
		{Name: "r1", Regex: []string{"[0-9]+"}},
		{Name: "bad", Regex: []string{"(unclosed"}},
	}
	if got := Route(routes, "contains 12345 here"); got != "r1" {
		t.Fatalf("expected r1, got %s", got)
	}
	// invalid regex should be ignored and not panic
	if got := Route(routes, "no digits"); got != "" {
		t.Fatalf("expected empty for no match with invalid regex present, got %s", got)
	}
}

func TestRoute_OrderFirstWins(t *testing.T) {
	routes := []config.SpecialistRoute{
		{Name: "first", Contains: []string{"a"}},
		{Name: "second", Contains: []string{"a"}},
	}
	if got := Route(routes, "A"); got != "first" {
		t.Fatalf("expected first to win, got %s", got)
	}
}
