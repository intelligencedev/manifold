package textsplitters

import (
	"testing"
)

func TestFixedCharsBasic(t *testing.T) {
	t.Parallel()
	s, err := NewFromConfig(Config{Kind: KindFixed, Fixed: FixedConfig{Unit: UnitChars, Size: 5}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	got := s.Split("abcdefghijklmnopqrstuvwxyz")
	want := []string{"abcde", "fghij", "klmno", "pqrst", "uvwxy", "z"}
	if len(got) != len(want) {
		t.Fatalf("len=%d want=%d got=%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("i=%d got=%q want=%q", i, got[i], want[i])
		}
	}
}

func TestFixedCharsOverlap(t *testing.T) {
	t.Parallel()
	s, err := NewFromConfig(Config{Kind: KindFixed, Fixed: FixedConfig{Unit: UnitChars, Size: 4, Overlap: 2}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	got := s.Split("abcdefg")
	want := []string{"abcd", "cdef", "efg"}
	if len(got) != len(want) {
		t.Fatalf("len=%d want=%d got=%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("i=%d got=%q want=%q", i, got[i], want[i])
		}
	}
}

func TestFixedTokensWhitespace(t *testing.T) {
	t.Parallel()
	s, err := NewFromConfig(Config{Kind: KindFixed, Fixed: FixedConfig{Unit: UnitTokens, Size: 3}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	got := s.Split("one  two\nthree\tfour five")
	want := []string{"one two three", "four five"}
	if len(got) != len(want) {
		t.Fatalf("len=%d want=%d got=%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("i=%d got=%q want=%q", i, got[i], want[i])
		}
	}
}

func TestFixedTokensOverlap(t *testing.T) {
	t.Parallel()
	s, err := NewFromConfig(Config{Kind: KindFixed, Fixed: FixedConfig{Unit: UnitTokens, Size: 2, Overlap: 1}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	got := s.Split("a b c d")
	want := []string{"a b", "b c", "c d"}
	if len(got) != len(want) {
		t.Fatalf("len=%d want=%d got=%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("i=%d got=%q want=%q", i, got[i], want[i])
		}
	}
}

func TestFixedEmpty(t *testing.T) {
	t.Parallel()
	s, err := NewFromConfig(Config{Kind: KindFixed, Fixed: FixedConfig{Unit: UnitChars, Size: 10}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	got := s.Split("")
	if len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}
