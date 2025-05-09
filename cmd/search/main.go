// mcp_test_client.go
package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

func main() {
	// Example usage of SearchDDG
	query := "openai"
	results, err := SearchDDG(query)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	for _, url := range results {
		fmt.Println(url)
	}

}

func SearchDDG(query string) ([]string, error) {
	ctx, cancel := chromedp.NewExecAllocator(context.Background(),
		append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.Flag("headless", true),
			// NORMAL UA – DuckDuckGo blocks “HeadlessChrome”
			chromedp.UserAgent(`Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/137.0.0.0 Safari/537.36`),
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
		return nil, err
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
	return urls, nil
}
