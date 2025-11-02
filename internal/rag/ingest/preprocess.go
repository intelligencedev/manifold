package ingest

import (
    "context"
    "crypto/sha256"
    "encoding/hex"
    "regexp"
    "strings"
)

// LanguageDetector abstracts language identification for FTS tokenization.
type LanguageDetector interface {
    Detect(ctx context.Context, text string) (string, error)
}

// DefaultLanguageDetector returns a fixed language (english).
type DefaultLanguageDetector struct{}

func (DefaultLanguageDetector) Detect(ctx context.Context, text string) (string, error) {
    return "english", nil
}

// PreprocessedDoc contains normalized text and metadata computed during preprocessing.
type PreprocessedDoc struct {
    Text     string
    Language string
    Hash     string
}

var whitespaceRe = regexp.MustCompile(`(?m)[\t\x0b\x0c\r ]+`)

// normalizeWhitespace collapses runs of spaces/tabs and normalizes newlines.
func normalizeWhitespace(s string) string {
    // Normalize CRLF to LF
    s = strings.ReplaceAll(s, "\r\n", "\n")
    s = strings.ReplaceAll(s, "\r", "\n")
    // Collapse horizontal whitespace but keep newlines
    s = whitespaceRe.ReplaceAllString(s, " ")
    // Collapse multiple blank lines to at most two newlines
    s = regexp.MustCompile(`\n{3,}`).ReplaceAllString(s, "\n\n")
    // Trim surrounding space/newlines
    s = strings.TrimSpace(s)
    return s
}

// ComputeHash creates a stable SHA-256 hex digest using text+source+url.
func ComputeHash(text, source, url string) string {
    h := sha256.New()
    // include separators to avoid ambiguity
    h.Write([]byte(text))
    h.Write([]byte{"|"[0]})
    h.Write([]byte(source))
    h.Write([]byte{"|"[0]})
    h.Write([]byte(url))
    sum := h.Sum(nil)
    return hex.EncodeToString(sum)
}

// Preprocess performs whitespace normalization, language detection, and doc hash computation.
func Preprocess(ctx context.Context, det LanguageDetector, in IngestRequest) (PreprocessedDoc, error) {
    if det == nil {
        det = DefaultLanguageDetector{}
    }
    norm := normalizeWhitespace(in.Text)
    lang, err := det.Detect(ctx, norm)
    if err != nil || lang == "" {
        lang = "english"
    }
    hash := ComputeHash(norm, in.Source, in.URL)
    return PreprocessedDoc{Text: norm, Language: lang, Hash: hash}, nil
}

