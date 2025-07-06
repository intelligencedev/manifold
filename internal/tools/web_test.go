package tools

import "testing"

func TestExtractMainContent(t *testing.T) {
	html := `<html><head><title>Test Page</title></head><body><nav>nav</nav><article><p>Hello</p><p>World</p></article></body></html>`
	pg, err := extractMainContent(html, "http://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pg.Title != "Test Page" {
		t.Errorf("expected title 'Test Page', got %s", pg.Title)
	}
	want := "Hello World"
	if pg.Content != want {
		t.Errorf("expected content %q, got %q", want, pg.Content)
	}
}

func TestCleanURL(t *testing.T) {
	url := "http://example.com/path?query=1." // trailing period
	cleaned := cleanURL(url)
	if cleaned != "http://example.com/path?query=1" {
		t.Errorf("unexpected cleaned url: %s", cleaned)
	}
}
