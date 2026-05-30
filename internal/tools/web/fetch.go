// internal/tools/web/fetch.go
package web

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	readability "github.com/go-shiori/go-readability"
	"golang.org/x/net/html/charset"
)

// Result is the structured output; Markdown is the main payload.
type Result struct {
	InputURL     string
	FinalURL     string
	Status       int
	ContentType  string
	Charset      string
	Title        string
	Markdown     string
	UsedReadable bool
	FetchedAt    time.Time
}

// FetchOptions tunes behavior. Zero value is sensible; use NewFetcher() defaults.
type FetchOptions struct {
	// Overall deadline for the request (headers + body).
	Timeout time.Duration

	// Max bytes to read for the body to avoid OOMs.
	MaxBytes int64

	// If true, try to extract main article using Readability, falling back to full HTML.
	PreferReadable bool

	// Optional UA override.
	UserAgent string

	// Allow up to this many redirects. 0 => use default (10).
	MaxRedirects int
}

// Option is the functional option type.
type Option func(*FetchOptions)

// WithTimeout sets the total timeout.
func WithTimeout(d time.Duration) Option { return func(o *FetchOptions) { o.Timeout = d } }

// WithMaxBytes sets the maximum bytes to read.
func WithMaxBytes(n int64) Option { return func(o *FetchOptions) { o.MaxBytes = n } }

// WithPreferReadable toggles readability extraction.
func WithPreferReadable(v bool) Option { return func(o *FetchOptions) { o.PreferReadable = v } }

// WithUserAgent sets a custom UA.
func WithUserAgent(ua string) Option { return func(o *FetchOptions) { o.UserAgent = ua } }

// WithMaxRedirects caps redirects.
func WithMaxRedirects(n int) Option { return func(o *FetchOptions) { o.MaxRedirects = n } }

// Fetcher holds the http.Client and options.
type Fetcher struct {
	client *http.Client
	opts   FetchOptions
	uaList []string
}

// NewFetcher creates a fetcher with hardened defaults.
func NewFetcher(opts ...Option) *Fetcher {
	o := FetchOptions{
		Timeout:        20 * time.Second,
		MaxBytes:       8 * 1000 * 1000, // 8 MB minimum
		PreferReadable: true,
		UserAgent:      "",
		MaxRedirects:   10,
	}
	for _, fn := range opts {
		fn(&o)
	}

	dialer := &net.Dialer{
		Timeout:   7 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         dialer.DialContext,
		ForceAttemptHTTP2:   true,
		TLSHandshakeTimeout: 7 * time.Second,
		// Increase connection pool sizes to better support concurrent fetches.
		MaxIdleConns:          200,
		MaxIdleConnsPerHost:   50,
		MaxConnsPerHost:       200,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		// NOTE: net/http will transparently gzip-decode if we don't set Accept-Encoding ourselves.
	}

	checkRedirect := func(req *http.Request, via []*http.Request) error {
		if o.MaxRedirects <= 0 {
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			return nil
		}
		if len(via) > o.MaxRedirects {
			return fmt.Errorf("stopped after %d redirects", o.MaxRedirects)
		}
		return nil
	}

	client := &http.Client{
		Transport:     transport,
		CheckRedirect: checkRedirect,
		Timeout:       o.Timeout, // hard cap
	}

	// List of realistic browser User-Agents
	uaList := []string{
		// Chrome (macOS)
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36",
		// Firefox (macOS)
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:102.0) Gecko/20100101 Firefox/102.0",
		// Safari (macOS)
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/15.1 Safari/605.1.15",
		// Edge (Windows)
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36 Edg/115.0.0.0",
	}
	return &Fetcher{client: client, opts: o, uaList: uaList}
}

// FetchMarkdown fetches the URL and returns best-effort Markdown content.
// It never returns nil Result on success; Markdown can be a short stub for non-text types.
func (f *Fetcher) FetchMarkdown(ctx context.Context, rawURL string) (*Result, error) {
	req, err := f.newFetchRequest(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	payload, err := f.readResponse(rawURL, resp)
	if err != nil {
		return nil, err
	}
	return f.resultFromPayload(payload)
}

func (f *Fetcher) newFetchRequest(ctx context.Context, rawURL string) (*http.Request, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("unsupported scheme: %s", u.Scheme)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", f.userAgent())
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,image/apng,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	// NOTE: Do NOT set Accept-Encoding manually - let Go's HTTP client handle compression automatically
	return req, nil
}

func (f *Fetcher) userAgent() string {
	if f.opts.UserAgent != "" {
		return f.opts.UserAgent
	}
	if len(f.uaList) == 0 {
		return ""
	}
	return f.uaList[int(time.Now().UnixNano())%len(f.uaList)]
}

type fetchPayload struct {
	rawURL      string
	finalURL    string
	status      int
	contentType string
	charset     string
	body        []byte
	utf8Body    []byte
	fetchedAt   time.Time
}

func (f *Fetcher) readResponse(rawURL string, resp *http.Response) (fetchPayload, error) {
	finalURL := resp.Request.URL.String()
	ct, cs := parseContentType(resp.Header.Get("Content-Type"))
	body, err := f.readLimitedBody(resp.Body)
	if err != nil {
		return fetchPayload{}, err
	}
	utf8Body, err := toUTF8(body, cs)
	if err != nil {
		return fetchPayload{}, fmt.Errorf("charset decode: %w", err)
	}
	return fetchPayload{
		rawURL:      rawURL,
		finalURL:    finalURL,
		status:      resp.StatusCode,
		contentType: ct,
		charset:     cs,
		body:        body,
		utf8Body:    utf8Body,
		fetchedAt:   time.Now(),
	}, nil
}

func (f *Fetcher) readLimitedBody(body io.Reader) ([]byte, error) {
	limited := io.LimitReader(body, f.opts.MaxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if int64(len(data)) > f.opts.MaxBytes {
		return nil, fmt.Errorf("response exceeds max bytes (%d)", f.opts.MaxBytes)
	}
	return data, nil
}

func (f *Fetcher) resultFromPayload(payload fetchPayload) (*Result, error) {
	res := &Result{
		InputURL:    payload.rawURL,
		FinalURL:    payload.finalURL,
		Status:      payload.status,
		ContentType: payload.contentType,
		Charset:     payload.charset,
		FetchedAt:   payload.fetchedAt,
	}
	switch {
	case isHTML(payload.contentType):
		return f.htmlResult(res, payload)
	case strings.HasPrefix(payload.contentType, "text/"):
		res.Markdown = fenced(string(payload.utf8Body), guessFenceLanguage(payload.contentType))
	case payload.contentType == "application/json" || strings.HasSuffix(payload.contentType, "+json"):
		res.Markdown = fenced(string(payload.utf8Body), "json")
	default:
		res.Markdown = binaryMarkdown(payload)
	}
	return res, nil
}

func (f *Fetcher) htmlResult(res *Result, payload fetchPayload) (*Result, error) {
	articleHTML, title, usedRead := f.readableHTML(string(payload.utf8Body), payload.finalURL)
	md, err := htmltomarkdown.ConvertString(articleHTML, converter.WithDomain(baseOrigin(payload.finalURL)))
	if err != nil {
		return nil, fmt.Errorf("html→markdown: %w", err)
	}
	if title != "" && !hasLeadingH1(md) {
		md = "# " + title + "\n\n" + md
	}
	res.Markdown = strings.TrimSpace(md)
	res.Title = title
	res.UsedReadable = usedRead
	return res, nil
}

func (f *Fetcher) readableHTML(html, finalURL string) (string, string, bool) {
	if f.opts.PreferReadable {
		base, _ := url.Parse(finalURL)
		art, err := readability.FromReader(strings.NewReader(html), base)
		if err == nil && strings.TrimSpace(art.Content) != "" {
			return art.Content, strings.TrimSpace(art.Title), true
		}
	}
	return html, "", false
}

func binaryMarkdown(payload fetchPayload) string {
	name := payload.contentType
	if name == "" {
		name = "application/octet-stream"
	}
	return fmt.Sprintf(
		"**Downloaded a non-text resource** (`%s`, %d bytes).\n\n[Download original](%s)",
		name, len(payload.body), payload.finalURL,
	)
}

func parseContentType(h string) (ctype, charset string) {
	if h == "" {
		return "", ""
	}
	mt, params, err := mime.ParseMediaType(h)
	if err != nil {
		return h, ""
	}
	return strings.ToLower(mt), strings.ToLower(params["charset"])
}

func isHTML(ct string) bool {
	return ct == "text/html" || ct == "application/xhtml+xml" || strings.HasSuffix(ct, "html")
}

func toUTF8(b []byte, charsetLabel string) ([]byte, error) {
	if charsetLabel == "" || strings.EqualFold(charsetLabel, "utf-8") || strings.EqualFold(charsetLabel, "utf8") {
		return b, nil
	}
	r, err := charset.NewReaderLabel(charsetLabel, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	return io.ReadAll(r)
}

func baseOrigin(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func guessFenceLanguage(ct string) string {
	switch ct {
	case "text/markdown":
		return "md"
	case "text/csv":
		return "csv"
	case "text/xml", "application/xml":
		return "xml"
	case "text/html", "application/xhtml+xml":
		return "html"
	default:
		return ""
	}
}

func fenced(s, lang string) string {
	s = strings.TrimRight(s, "\n")
	if lang != "" {
		return "```" + lang + "\n" + s + "\n```"
	}
	return "```\n" + s + "\n```"
}

func hasLeadingH1(md string) bool {
	md = strings.TrimLeft(md, "\n")
	return strings.HasPrefix(md, "# ")
}
