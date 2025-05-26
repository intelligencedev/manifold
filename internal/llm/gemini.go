package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"

	cfg "manifold/internal/config"
)

// GeminiProxyRequest defines the payload expected for the Gemini proxy endpoint.
type GeminiProxyRequest struct {
	Model    string          `json:"model"`
	Contents json.RawMessage `json:"contents"`
}

// HandleGemini proxies streaming requests to the Google Gemini API.
func HandleGemini(c echo.Context, config *cfg.Config) error {
	var req GeminiProxyRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if req.Model == "" || len(req.Contents) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "model and contents required"})
	}

	body, err := json.Marshal(map[string]json.RawMessage{
		"contents": req.Contents,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to marshal request"})
	}

	endpoint := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:streamGenerateContent?key=%s", req.Model, config.GoogleGeminiKey)
	httpReq, err := http.NewRequestWithContext(c.Request().Context(), "POST", endpoint, bytes.NewBuffer(body))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create request"})
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to send request to gemini api: " + err.Error()})
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("gemini api returned an error: %d %s", resp.StatusCode, string(bodyBytes))})
	}

	c.Response().Header().Set(echo.HeaderContentType, resp.Header.Get("Content-Type"))
	c.Response().Header().Set("Cache-Control", "no-cache")
	flusher, ok := c.Response().Writer.(http.Flusher)
	if !ok {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "streaming not supported"})
	}

	buf := make([]byte, 1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, wErr := c.Response().Writer.Write(buf[:n]); wErr != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to write response: " + wErr.Error()})
			}
			flusher.Flush()
		}
		if err != nil {
			if err != io.EOF {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "error reading from gemini api: " + err.Error()})
			}
			break
		}
	}
	return nil
}
