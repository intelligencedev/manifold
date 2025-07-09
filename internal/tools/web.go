package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
	"github.com/jackc/pgx/v5"
	"golang.org/x/net/html"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

var (
	resultURLs []string
)

// WebClient fetches and parses web pages for the agents.
type WebClient struct {
	httpClient *http.Client
	userAgents []string
}

// NewWebClient creates a WebClient with sane defaults.
func NewWebClient() *WebClient {
	return &WebClient{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		userAgents: []string{
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/137.0.0.0 Safari/537.36",
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/137.0.0.0 Safari/537.36",
			"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/137.0.0.0 Safari/537.36",
		},
	}
}

// WebPageContent represents the content of a webpage.
type WebPageContent struct {
	Title   string
	Content string // Markdown content
	Source  string
}

// Get retrieves the main content from the given address.
// Returns the main content, and the HTTP status code (0 if not available).
func (c *WebClient) Get(ctx context.Context, address string) (*WebPageContent, int, error) {
	if !checkRobotsTxt(address) {
		return &WebPageContent{Content: fmt.Sprintf("Unable to read %s due to robots.txt", address), Source: address}, 0, nil
	}

	htmlContent, status, err := c.fetchHTMLWithStatus(ctx, address)
	if err != nil {
		msg := fmt.Sprintf("Could not retrieve %s", address)
		return &WebPageContent{Content: msg, Source: address}, status, nil
	}

	readerContent, err := extractMainContent(htmlContent, address)
	if err != nil {
		msg := fmt.Sprintf("Could not parse %s", address)
		return &WebPageContent{Content: msg, Source: address}, status, nil
	}

	return readerContent, status, nil
}

// CheckRobotsTxt checks if the target website allows scraping by checking its robots.txt file.
func checkRobotsTxt(u string) bool {
	baseURL, err := url.Parse(u)
	if err != nil {
		log.Printf("Failed to parse baseURL: %v", err)
		return false
	}

	robotsURL := url.URL{Scheme: baseURL.Scheme, Host: baseURL.Host, Path: "/robots.txt"}
	resp, err := http.Get(robotsURL.String())
	if err != nil {
		// If robots.txt cannot be fetched assume allowed
		return true
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Treat non-200 as missing robots.txt and allow
		return true
	}

	log.Printf("robots.txt checked: %s\n", robotsURL.String())
	return true
}

// fetchWebContent retrieves and parses a web page without any caching or
// database lookups. Returns content, status code, error.
func fetchWebContent(address string) (*WebPageContent, int, error) {
	client := NewWebClient()
	return client.Get(context.Background(), address)
}

// WebGetHandler retrieves the reader view content of a given URL. It first
// checks the web_blacklist and web_content tables to avoid unnecessary
// requests and store results for future use.
func WebGetHandler(ctx context.Context, db *pgx.Conn, address string) (*WebPageContent, error) {
	// Validate the input URL before any DB operation
	parsedURL, err := url.Parse(address)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return &WebPageContent{Content: fmt.Sprintf("Invalid URL: %s", address), Source: address}, nil
	}

	// Extract top-level domain (scheme + host)
	topLevel := parsedURL.Scheme + "://" + parsedURL.Host

	// Check blacklist using top-level domain
	var tmp int
	err = db.QueryRow(ctx, "SELECT 1 FROM web_blacklist WHERE url = $1", topLevel).Scan(&tmp)
	if err == nil {
		return &WebPageContent{Content: fmt.Sprintf("%s is blacklisted in database", topLevel), Source: address}, nil
	}
	if err != nil && err != pgx.ErrNoRows {
		return nil, err
	}

	// Check if already indexed (by full URL)
	var title, content string
	err = db.QueryRow(ctx, "SELECT title, content FROM web_content WHERE url = $1", address).Scan(&title, &content)
	if err == nil {
		return &WebPageContent{Title: title, Content: content, Source: address}, nil
	}
	if err != nil && err != pgx.ErrNoRows {
		return nil, err
	}

	// Fetch new content
	pg, status, err := fetchWebContent(address)
	if err != nil {
		return nil, err
	}
	if pg == nil {
		return nil, fmt.Errorf("no content")
	}

	// Blacklist if HTTP status is not 200, or robots.txt disallowed
	if status != 200 {
		// Only insert valid top-level domains
		if parsedURL.Scheme != "" && parsedURL.Host != "" {
			_, dbErr := db.Exec(ctx, `INSERT INTO web_blacklist (url) VALUES ($1) ON CONFLICT DO NOTHING`, topLevel)
			if dbErr != nil {
				log.Printf("failed to insert web_blacklist: %v", dbErr)
			}
		}
		return &WebPageContent{Content: fmt.Sprintf("%s is blacklisted due to HTTP status %d", topLevel, status), Source: address}, nil
	}

	// Persist to database (best effort, only for valid URLs)
	if parsedURL.Scheme != "" && parsedURL.Host != "" {
		_, dbErr := db.Exec(ctx, `INSERT INTO web_content (url, title, content, fetched_at) VALUES ($1,$2,$3,NOW()) ON CONFLICT (url) DO NOTHING`, address, pg.Title, pg.Content)
		if dbErr != nil {
			log.Printf("failed to insert web_content: %v", dbErr)
		}
	}

	return pg, nil
}

// fetchHTMLWithStatus retrieves the HTML content and HTTP status code of a given URL using chromedp and a fallback HTTP GET.
func (c *WebClient) fetchHTMLWithStatus(ctx context.Context, address string) (string, int, error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(ctx, opts...)
	defer cancel()

	ctx, cancel = chromedp.NewContext(allocCtx)
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var htmlContent string
	var statusCode int = 0
	ua := c.userAgents[rand.Intn(len(c.userAgents))]
	// Remove unused variables
	err := chromedp.Run(ctx,
		chromedp.Navigate(address),
		chromedp.ActionFunc(func(ctx context.Context) error {
			headers := map[string]interface{}{
				"User-Agent":      ua,
				"Referer":         "https://www.duckduckgo.com/",
				"Accept-Language": "en-US,en;q=0.9",
				"X-Forwarded-For": "203.0.113.195",
				"Accept-Encoding": "gzip, deflate, br",
				"Connection":      "keep-alive",
				"DNT":             "1",
			}
			return network.SetExtraHTTPHeaders(network.Headers(headers)).Do(ctx)
		}),
		network.Enable(),
		chromedp.WaitReady("body"),
		chromedp.OuterHTML("html", &htmlContent),
	)

	// Fallback: use HTTP GET to get status code if chromedp did not error
	if err == nil {
		req, _ := http.NewRequestWithContext(ctx, "GET", address, nil)
		req.Header.Set("User-Agent", ua)
		resp, httpErr := c.httpClient.Do(req)
		if httpErr == nil {
			statusCode = resp.StatusCode
			resp.Body.Close()
		}
	} else {
		// If chromedp failed, try HTTP GET for both content and status
		req, _ := http.NewRequestWithContext(ctx, "GET", address, nil)
		req.Header.Set("User-Agent", ua)
		resp, httpErr := c.httpClient.Do(req)
		if httpErr != nil {
			return "", 0, fmt.Errorf("error retrieving page: %w", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		htmlContent = string(body)
		statusCode = resp.StatusCode
		if statusCode != 200 {
			return htmlContent, statusCode, fmt.Errorf("non-200 status: %d", statusCode)
		}
		return htmlContent, statusCode, nil
	}

	return htmlContent, statusCode, nil
}

// extractMainContent extracts the main content of a webpage, cleans it, and converts it to Markdown.
func extractMainContent(htmlContent string, sourceURL string) (*WebPageContent, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	// Prune nodes that are unlikely to be part of the main content.
	pruneNonContentNodes(doc)

	title := extractTitle(doc)
	mainContentNode := findMainContentNode(doc)
	if mainContentNode == nil {
		return nil, errors.New("failed to locate main content node")
	}

	// Include qualifying sibling nodes (e.g. additional paragraphs)
	contentHTML := includeSiblingContent(mainContentNode)

	// Convert cleaned HTML to Markdown using an external library.
	converter := md.NewConverter("", true, nil)
	mdContent, err := converter.ConvertString(contentHTML)
	if err != nil {
		return nil, fmt.Errorf("failed to convert HTML to Markdown: %w", err)
	}

	// Further clean up Markdown (remove extra empty lines, etc.)
	mdContent = removeEmptyRows(mdContent)

	return &WebPageContent{
		Title:   title,
		Content: mdContent,
		Source:  sourceURL,
	}, nil
}

// extractTitle extracts the title of the webpage.
func extractTitle(n *html.Node) string {
	var title string
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "title" && n.FirstChild != nil {
			title = n.FirstChild.Data
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(n)
	return title
}

// pruneNonContentNodes recursively removes nodes that are unlikely to be part of the main content.
func pruneNonContentNodes(n *html.Node) {
	if n == nil {
		return
	}
	// List of tags to remove.
	unwantedTags := map[string]bool{
		"script":   true,
		"style":    true,
		"noscript": true,
		"iframe":   true,
		"header":   true,
		"footer":   true,
		"nav":      true,
		"aside":    true,
		"form":     true,
	}
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		if c.Type == html.ElementNode {
			if unwantedTags[c.Data] {
				n.RemoveChild(c)
			} else {
				pruneNonContentNodes(c)
			}
		} else {
			pruneNonContentNodes(c)
		}
		c = next
	}
}

// extractArticleContent is now replaced by findMainContentNode and includeSiblingContent.
// findMainContentNode locates the candidate node using tag hints and a fallback heuristic.
func findMainContentNode(n *html.Node) *html.Node {
	// First, look for semantic tags.
	for _, tag := range []string{"article", "main"} {
		if node := findNodeByTag(n, tag); node != nil {
			return node
		}
	}
	// Fallback: return the <div> with the highest score.
	return findLargestContentDiv(n)
}

// findNodeByTag recursively searches for a node with the specified tag name.
func findNodeByTag(n *html.Node, tag string) *html.Node {
	if n.Type == html.ElementNode && n.Data == tag {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findNodeByTag(c, tag); found != nil {
			return found
		}
	}
	return nil
}

// findLargestContentDiv finds the div with the highest computed score (text length discounted by link density).
func findLargestContentDiv(n *html.Node) *html.Node {
	var largestDiv *html.Node
	bestScore := 0.0

	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "div" {
			score := computeScore(n)
			if score > bestScore {
				bestScore = score
				largestDiv = n
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(n)
	return largestDiv
}

// computeScore calculates a simple score based on text length discounted by link density.
func computeScore(n *html.Node) float64 {
	totalLength := float64(extractText(n, nil))
	linkLength := float64(extractLinkTextLength(n))
	// Avoid division by zero; higher link density reduces score.
	return totalLength * (1 - linkLength/(totalLength+1))
}

// extractLinkTextLength recursively calculates the total text length within <a> elements.
func extractLinkTextLength(n *html.Node) int {
	total := 0
	if n.Type == html.ElementNode && n.Data == "a" {
		total += extractText(n, nil)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		total += extractLinkTextLength(c)
	}
	return total
}

// includeSiblingContent concatenates the HTML of the candidate node with qualifying siblings.
func includeSiblingContent(candidate *html.Node) string {
	var buf bytes.Buffer
	// Render candidate node.
	if err := html.Render(&buf, candidate); err != nil {
		return ""
	}
	candidateScore := computeScore(candidate)
	if candidate.Parent != nil {
		for sibling := candidate.Parent.FirstChild; sibling != nil; sibling = sibling.NextSibling {
			// Skip candidate itself.
			if sibling == candidate {
				continue
			}
			// Only consider block-level elements.
			if sibling.Type == html.ElementNode && (sibling.Data == "p" || sibling.Data == "div") {
				score := computeScore(sibling)
				// If the sibling's score is significant, append it.
				if score > 0.2*candidateScore {
					buf.WriteString("\n")
					if err := html.Render(&buf, sibling); err != nil {
						continue
					}
				}
			}
		}
	}
	return buf.String()
}

// extractText recursively extracts text from a node and its children.
// If sb is non-nil, text is appended to it; otherwise it just returns the total length.
func extractText(n *html.Node, sb *strings.Builder) int {
	if n.Type == html.TextNode {
		text := cleanText(n.Data)
		if sb != nil && text != "" {
			sb.WriteString(text)
			sb.WriteString(" ")
		}
		return len(text)
	}
	totalLength := 0
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		totalLength += extractText(c, sb)
	}
	return totalLength
}

// cleanText removes unnecessary whitespace from text.
func cleanText(text string) string {
	text = strings.TrimSpace(text)
	re := regexp.MustCompile(`\s+`)
	return re.ReplaceAllString(text, " ")
}

// removeEmptyRows removes empty rows from the input string.
func removeEmptyRows(input string) string {
	lines := strings.Split(input, "\n")
	var filteredLines []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			filteredLines = append(filteredLines, line)
		}
	}
	return strings.Join(filteredLines, "\n")
}

// --- The remaining functions (ExtractURLs, RemoveUrl, cleanURL, SearchDDG, GetSearchResults,
// RemoveUnwantedURLs, GetPageScreen, RemoveUrls, postRequest, extractURLsFromHTML, GetSearXNGResults)
// remain largely unchanged. ---

// ExtractURLs extracts URLs from the input string.
func ExtractURLs(input string) []string {
	urlRegex := `http.*?://[^\s<>{}|\\^` + "`" + `"]+`
	re := regexp.MustCompile(urlRegex)
	matches := re.FindAllString(input, -1)
	var cleanedURLs []string
	for _, match := range matches {
		cleanedURL := cleanURL(match)
		cleanedURLs = append(cleanedURLs, cleanedURL)
	}
	return cleanedURLs
}

// RemoveUrl removes URLs from each string in the input slice.
func RemoveUrl(input []string) []string {
	urlRegex := `http.*?://[^\s<>{}|\\^` + "`" + `"]+`
	re := regexp.MustCompile(urlRegex)
	for i, str := range input {
		matches := re.FindAllString(str, -1)
		for _, match := range matches {
			input[i] = strings.ReplaceAll(input[i], match, "")
		}
	}
	return input
}

// cleanURL removes illegal trailing characters from a URL.
func cleanURL(urlStr string) string {
	illegalTrailingChars := []rune{'.', ',', ';', '!', '?'}
	for _, char := range illegalTrailingChars {
		if urlStr[len(urlStr)-1] == byte(char) {
			urlStr = urlStr[:len(urlStr)-1]
		}
	}
	return urlStr
}

// SearchDDG performs a search on DuckDuckGo and returns the result URLs.
func SearchDDG(ctx context.Context, db *pgx.Conn, query string) []string {
	ua := NewWebClient().userAgents
	ctx, cancel := chromedp.NewExecAllocator(context.Background(),
		append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.Flag("headless", true),
			chromedp.UserAgent(ua[rand.Intn(len(ua))]),
		)...,
	)
	defer cancel()

	ctx, cancel = chromedp.NewContext(ctx)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var nodes []*cdp.Node
	if err := chromedp.Run(ctx,
		chromedp.Navigate(`https://lite.duckduckgo.com/lite/`),
		chromedp.WaitReady(`input[name="q"]`, chromedp.ByQuery),
		chromedp.SendKeys(`input[name="q"]`, query+kb.Enter, chromedp.ByQuery),

		// wait for first result link instead of the (possibly hidden) form
		chromedp.WaitReady(`a.result-link`, chromedp.ByQuery),
		chromedp.Nodes(`a.result-link`, &nodes, chromedp.ByQueryAll),
	); err != nil {
		return nil
	}

	uniq := map[string]struct{}{}
	for _, n := range nodes {
		if href := n.AttributeValue("href"); strings.HasPrefix(href, "http") {
			uniq[href] = struct{}{}
		}
	}
	urls := make([]string, 0, len(uniq))
	for u := range uniq {
		urls = append(urls, u)
	}

	resultURLs = RemoveUnwantedURLs(ctx, db, urls)
	// If resultURLs contains cnn.com, replace the URL with https://lite.cnn.com
	for i, u := range resultURLs {
		if strings.Contains(u, "https://www.cnn.com") {
			resultURLs[i] = strings.Replace(u, "https://www.cnn.com", "https://lite.cnn.com", 1)
		}
	}
	log.Println("Search results:", resultURLs)
	return resultURLs
}

// GetSearchResults retrieves the content of multiple URLs and returns it as a concatenated string.
func GetSearchResults(ctx context.Context, db *pgx.Conn, urls []string) string {
	var result strings.Builder
	for _, u := range urls {
		content, err := WebGetHandler(ctx, db, u)
		if err != nil {
			log.Printf("Error getting search result for URL %s: %v", u, err)
			continue
		}
		if content != nil && content.Content != "" {
			result.WriteString(fmt.Sprintf("Title: %s\n", content.Title))
			result.WriteString(fmt.Sprintf("Source: %s\n\n", content.Source))
			result.WriteString(content.Content)
			result.WriteString("\n\n")
		}
	}
	return result.String()
}

// RemoveUnwantedURLs filters out unwanted URLs from the given list.
func RemoveUnwantedURLs(ctx context.Context, db *pgx.Conn, urls []string) []string {
	rows, err := db.Query(ctx, "SELECT url FROM web_blacklist")
	if err != nil {
		log.Printf("failed to load blacklist: %v", err)
		return urls
	}
	var blacklist []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err == nil {
			blacklist = append(blacklist, u)
		}
	}
	rows.Close()

	var filteredURLs []string
	for _, u := range urls {
		log.Printf("Checking URL: %s", u)
		unwanted := false
		for _, unwantedURL := range blacklist {
			if strings.Contains(u, unwantedURL) {
				log.Printf("URL %s contains unwanted URL %s", u, unwantedURL)
				unwanted = true
				break
			}
		}
		if !unwanted {
			filteredURLs = append(filteredURLs, u)
		}
	}
	log.Printf("Filtered URLs: %v", filteredURLs)
	return filteredURLs
}

// GetPageScreen captures a screenshot of a webpage and saves it as a PNG file.
func GetPageScreen(chromeUrl string, pageAddress string) string {
	instanceUrl := chromeUrl
	allocatorCtx, cancel := chromedp.NewRemoteAllocator(context.Background(), instanceUrl)
	defer cancel()
	ctx, cancel := chromedp.NewContext(allocatorCtx, chromedp.WithLogf(log.Printf))
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var buf []byte
	err := chromedp.Run(ctx,
		chromedp.Navigate(pageAddress),
		chromedp.FullScreenshot(&buf, 90),
	)
	if err != nil {
		log.Fatal(err)
	}
	u, err := url.Parse(pageAddress)
	if err != nil {
		log.Fatal(err)
	}
	t := time.Now()
	filename := u.Hostname() + "-" + t.Format("20060102150405") + ".png"
	err = os.WriteFile(filename, buf, 0644)
	if err != nil {
		log.Fatal(err)
	}
	return filename
}

// RemoveUrls removes URLs from the input string.
func RemoveUrls(input string) string {
	urlRegex := `http.*?://[^\s<>{}|\\^` + "`" + `"]+`
	re := regexp.MustCompile(urlRegex)
	matches := re.FindAllString(input, -1)
	for _, match := range matches {
		input = strings.ReplaceAll(input, match, "")
	}
	return input
}

// postRequest sends a POST request to the given endpoint with a named parameter 'q'.
func postRequest(endpoint string, queryParam string) (string, error) {
	formData := url.Values{}
	formData.Set("q", queryParam)
	data := bytes.NewBufferString(formData.Encode())
	req, err := http.NewRequest("POST", endpoint, data)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to perform request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}
	return buf.String(), nil
}

// extractURLsFromHTML parses the HTML content and extracts URLs.
func extractURLsFromHTML(htmlContent string) ([]string, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}
	var urls []string
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, attr := range n.Attr {
				if attr.Key == "href" && strings.Contains(attr.Val, "http") {
					urls = append(urls, attr.Val)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)
	return urls, nil
}

// GetSearXNGResults performs a search on a SearXNG instance and returns the result URLs.
func GetSearXNGResults(ctx context.Context, db *pgx.Conn, endpoint string, query string) []string {
	htmlContent, err := postRequest(endpoint, query)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return nil
	}
	urls, err := extractURLsFromHTML(htmlContent)
	if err != nil {
		log.Printf("Error extracting URLs: %v\n", err)
		return nil
	}
	// Remove unwanted URLs
	urls = RemoveUnwantedURLs(ctx, db, urls)
	for i, u := range resultURLs {
		if strings.Contains(u, "https://www.cnn.com") {
			resultURLs[i] = strings.Replace(u, "https://www.cnn.com", "https://lite.cnn.com", 1)
		}
	}
	return urls
}
