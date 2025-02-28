// manifold/documents.go
package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"manifold/internal/documents"
	"manifold/internal/repoconcat"
	"manifold/internal/web"

	"github.com/labstack/echo/v4"
)

func repoconcatHandler(c echo.Context) error {
	var req RepoConcatRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}
	if len(req.Paths) == 0 || len(req.Types) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Paths and types are required"})
	}
	rc := repoconcat.NewRepoConcat()
	result, err := rc.Concatenate(req.Paths, req.Types, req.Recursive, req.IgnorePattern)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.String(http.StatusOK, result)
}

func splitTextHandler(c echo.Context) error {
	var req struct {
		Text      string `json:"text"`
		Splitter  string `json:"splitter"`
		ChunkSize int    `json:"chunk_size,omitempty"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}
	if req.Text == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Text is required"})
	}
	splitterType := documents.Language(req.Splitter)
	if splitterType == "" {
		splitterType = documents.DEFAULT
	}
	splitter, err := documents.FromLanguage(splitterType)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if splitterType == documents.DEFAULT && req.ChunkSize > 0 {
		splitter.ChunkSize = req.ChunkSize
	}
	chunks := splitter.SplitText(req.Text)
	return c.JSON(http.StatusOK, map[string]interface{}{"chunks": chunks})
}

func saveFileHandler(c echo.Context) error {
	type SaveFileRequest struct {
		Filepath string `json:"filepath" form:"filepath"`
		Content  string `json:"content" form:"content"`
	}
	var req SaveFileRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}
	if req.Filepath == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Parameter 'filepath' is required"})
	}
	dir := filepath.Dir(req.Filepath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Failed to create directory '%s': %v", dir, err)})
	}
	if err := os.WriteFile(req.Filepath, []byte(req.Content), 0644); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Failed to save file '%s': %v", req.Filepath, err)})
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "File saved successfully"})
}

func openFileHandler(c echo.Context) error {
	var req struct {
		Filepath string `json:"filepath"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}
	if req.Filepath == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Filepath is required"})
	}
	content, err := os.ReadFile(req.Filepath)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Failed to read file '%s': %v", req.Filepath, err)})
	}
	return c.String(http.StatusOK, string(content))
}

func webContentHandler(c echo.Context) error {
	urlsParam := c.QueryParam("urls")
	if urlsParam == "" {
		return c.String(http.StatusBadRequest, "URLs are required")
	}
	urls := strings.Split(urlsParam, ",")
	var wg sync.WaitGroup
	var mu sync.Mutex
	results := make(map[string]interface{})
	done := make(chan bool)
	go func() {
		for _, pageURL := range urls {
			wg.Add(1)
			go func(url string) {
				defer wg.Done()
				content, err := web.WebGetHandler(url)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					results[url] = map[string]string{"error": fmt.Sprintf("Error extracting web content: %v", err)}
				} else {
					results[url] = content
				}
			}(pageURL)
		}
		wg.Wait()
		done <- true
	}()
	select {
	case <-done:
		return c.JSON(http.StatusOK, results)
	case <-time.After(60 * time.Second):
		return c.JSON(http.StatusOK, results)
	}
}

func webSearchHandler(c echo.Context) error {
	query := c.QueryParam("query")
	if query == "" {
		return c.String(http.StatusBadRequest, "Query is required")
	}
	resultSize := 3
	if size := c.QueryParam("result_size"); size != "" {
		if parsedSize, err := strconv.Atoi(size); err == nil {
			resultSize = parsedSize
		}
	}
	searchBackend := c.QueryParam("search_backend")
	if searchBackend == "" {
		searchBackend = "ddg"
	}
	var results []string
	if searchBackend == "sxng" {
		sxngURL := c.QueryParam("sxng_url")
		if sxngURL == "" {
			return c.String(http.StatusBadRequest, "sxng_url is required when search_backend is sxng")
		}
		results = web.GetSearXNGResults(sxngURL, query)
	} else {
		results = web.SearchDDG(query)
	}
	if results == nil {
		return c.String(http.StatusInternalServerError, "Error performing web search")
	}
	if len(results) > resultSize {
		results = results[:resultSize]
	}
	return c.JSON(http.StatusOK, results)
}
