package web

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
	"golang.org/x/net/html"
)

var (
	// These are the URLs we want to block from search results since they will likely fail
	// with the current implementation. We should make this list configurable in the future.
	unwantedURLs = []string{
		"web.archive.org",
		"www.youtube.com",
		"www.youtube.com/watch",
		"www.wired.com",
		"www.techcrunch.com",
		"www.wsj.com",
		"www.nytimes.com",
		"www.forbes.com",
		"www.businessinsider.com",
		"www.theverge.com",
		"www.thehill.com",
		"www.theatlantic.com",
		"www.foxnews.com",
		"www.theguardian.com",
		"www.nbcnews.com",
		"www.msn.com",
		"www.sciencedaily.com",
		"reuters.com",
		"bbc.com",
		"thenewstack.io",
		"abcnews.go.com",
		"apnews.com",
		"bloomberg.com",
		"polygon.com",
		"reddit.com",
		"indeed.com",
		"test.com",
		"medium.com",
		// Add more URLs to block from search results
	}

	resultURLs []string
)

// CheckRobotsTxt checks if the target website allows scraping by "et-bot".
func checkRobotsTxt(ctx context.Context, u string) bool {
	baseURL, err := url.Parse(u)
	if err != nil {
		log.Printf("Failed to parse baseURL: %v", err)
		return false
	}

	robotsUrl := url.URL{Scheme: baseURL.Scheme, Host: baseURL.Host, Path: "/robots.txt"}
	resp, err := http.Get(robotsUrl.String())
	if err != nil {
		log.Printf("Failed to fetch robots.txt for %s: %v", baseURL.String(), err)
		return false
	}
	defer resp.Body.Close()

	// Check if the status code is 200
	if resp.StatusCode != 200 {
		log.Printf("Failed to fetch robots.txt for %s: %v", baseURL.String(), err)

		// We assume its allowed if not found
		return true
	}

	// Parse the robots.txt content if needed
	// Print the URL and the content of the robots.txt
	log.Printf("URL: %s\n", robotsUrl.String())
	return true
}

// WebPageContent represents the content of a webpage.
type WebPageContent struct {
	Title   string
	Content string
	Source  string
}

// WebGetHandler retrieves the reader view content of a given URL.
func WebGetHandler(address string) (*WebPageContent, error) {
	if !checkRobotsTxt(context.Background(), address) {
		return nil, errors.New("scraping not allowed according to robots.txt")
	}

	htmlContent, err := fetchHTML(address)
	if err != nil {
		return nil, err
	}

	readerContent, err := extractMainContent(htmlContent, address)
	if err != nil {
		return nil, err
	}

	return readerContent, nil
}

// fetchHTML retrieves the HTML content of a given URL using chromedp.
func fetchHTML(address string) (string, error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var htmlContent string
	err := chromedp.Run(ctx,
		chromedp.Navigate(address),
		chromedp.ActionFunc(func(ctx context.Context) error {
			headers := map[string]interface{}{
				"User-Agent":      "et-bot", // Set user agent to et-bot
				"Referer":         "https://www.duckduckgo.com/",
				"Accept-Language": "en-US,en;q=0.9",
				"X-Forwarded-For": "203.0.113.195",
				"Accept-Encoding": "gzip, deflate, br",
				"Connection":      "keep-alive",
				"DNT":             "1",
			}
			return network.SetExtraHTTPHeaders(network.Headers(headers)).Do(ctx)
		}),
		chromedp.WaitReady("body"),
		chromedp.OuterHTML("html", &htmlContent),
	)

	if err != nil {
		return "", fmt.Errorf("error retrieving page: %w", err)
	}

	return htmlContent, nil
}

// extractMainContent extracts the main content of a webpage and returns it in reader view format.
func extractMainContent(htmlContent string, sourceURL string) (*WebPageContent, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	title := extractTitle(doc)
	mainContent := extractArticleContent(doc)

	// Clean up the content
	cleanedContent := cleanText(mainContent)

	return &WebPageContent{
		Title:   title,
		Content: cleanedContent,
		Source:  sourceURL,
	}, nil
}

// extractTitle extracts the title of the webpage.
func extractTitle(n *html.Node) string {
	var title string
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "title" {
			if n.FirstChild != nil && n.FirstChild.Type == html.TextNode {
				title = n.FirstChild.Data
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(n)
	return title
}

// extractArticleContent extracts the main article content from the HTML node.
func extractArticleContent(n *html.Node) string {
	// Find the main content node (e.g., <article>, <main>, or the largest <div>)
	mainContentNode := findMainContentNode(n)
	if mainContentNode == nil {
		return ""
	}

	// Extract text content from the main content node
	var content strings.Builder
	extractText(mainContentNode, &content)
	return content.String()
}

// findMainContentNode attempts to locate the primary content node within the HTML document.
func findMainContentNode(n *html.Node) *html.Node {
	// Define a list of potential content node tag names
	contentTags := []string{"article", "main"}

	// Search for specific content tags
	for _, tag := range contentTags {
		if node := findNodeByTag(n, tag); node != nil {
			return node
		}
	}

	// Fallback: find the div with the most text content as a last resort
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

// findLargestContentDiv finds the div with the most significant text content.
func findLargestContentDiv(n *html.Node) *html.Node {
	var largestDiv *html.Node
	maxTextLength := 0

	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "div" {
			textLength := extractText(n, nil)
			if textLength > maxTextLength {
				maxTextLength = textLength
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

// extractText recursively extracts text from a node and its children, optionally writing to a strings.Builder.
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

// cleanText removes unnecessary whitespace and symbols from text.
func cleanText(text string) string {
	// Remove leading and trailing whitespace
	text = strings.TrimSpace(text)

	// Replace multiple spaces with a single space
	re := regexp.MustCompile(`\s+`)
	text = re.ReplaceAllString(text, " ")

	// Remove newlines
	text = strings.ReplaceAll(text, "\n", " ")

	return text
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
func cleanURL(url string) string {
	illegalTrailingChars := []rune{'.', ',', ';', '!', '?'}

	for _, char := range illegalTrailingChars {
		if url[len(url)-1] == byte(char) {
			url = url[:len(url)-1]
		}
	}

	return url
}

// SearchDDG performs a search on DuckDuckGo and returns the result URLs.
func SearchDDG(query string) []string {
	resultURLs = nil

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
	)
	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()
	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var nodes []*cdp.Node

	err := chromedp.Run(ctx,
		chromedp.Navigate(`https://lite.duckduckgo.com/lite/`),
		chromedp.WaitVisible(`input[name="q"]`, chromedp.ByQuery),
		chromedp.SendKeys(`input[name="q"]`, query+kb.Enter, chromedp.ByQuery),
		chromedp.Sleep(5*time.Second),
		chromedp.WaitVisible(`input[name="q"]`, chromedp.ByQuery),
		chromedp.Nodes(`a`, &nodes, chromedp.ByQueryAll),
	)
	if err != nil {
		log.Printf("Error during search: %v", err)
		return nil
	}

	err = chromedp.Run(ctx,
		chromedp.ActionFunc(func(c context.Context) error {
			re, err := regexp.Compile(`^http[s]?://`)
			if err != nil {
				return err
			}

			uniqueUrls := make(map[string]bool)
			for _, n := range nodes {
				for _, attr := range n.Attributes {
					if re.MatchString(attr) && !strings.Contains(attr, "duckduckgo") {
						uniqueUrls[attr] = true
					}
				}
			}

			for u := range uniqueUrls {
				resultURLs = append(resultURLs, u)
			}

			return nil
		}),
	)

	if err != nil {
		log.Printf("Error processing results: %v", err)
		return nil
	}

	resultURLs = RemoveUnwantedURLs(resultURLs)

	// If resultURLs is contains cnn.com, replace the URL with https://lite.cnn.com
	for i, u := range resultURLs {
		if strings.Contains(u, "https://www.cnn.com") {
			resultURLs[i] = strings.Replace(u, "https://www.cnn.com", "https://lite.cnn.com", 1)
		}
	}

	log.Println("Search results:", resultURLs)

	return resultURLs
}

// GetSearchResults retrieves the content of multiple URLs and returns it as a concatenated string.
func GetSearchResults(urls []string) string {
	var result strings.Builder

	for _, u := range urls {
		content, err := WebGetHandler(u)
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
func RemoveUnwantedURLs(urls []string) []string {
	var filteredURLs []string
	for _, u := range urls {
		log.Printf("Checking URL: %s", u)

		unwanted := false
		for _, unwantedURL := range unwantedURLs {
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
func GetSearXNGResults(endpoint string, query string) []string {
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
	urls = RemoveUnwantedURLs(urls)

	for i, u := range resultURLs {
		if strings.Contains(u, "https://www.cnn.com") {
			resultURLs[i] = strings.Replace(u, "https://www.cnn.com", "https://lite.cnn.com", 1)
		}
	}

	return urls
}
