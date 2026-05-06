package openai

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// sseTransportWrapper wraps an HTTP transport to inject the Accept: text/event-stream
// header for streaming requests to self-hosted servers like mlx_lm. This fixes
// compatibility issues where mlx_lm.server expects the SSE accept header for proper
// chunked transfer encoding of streaming responses.
type sseTransportWrapper struct {
	inner      http.RoundTripper
	baseURL    string
	isSelfHost bool
}

func (t *sseTransportWrapper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Only inject header for self-hosted streaming requests
	if t.isSelfHost && strings.HasPrefix(req.URL.String(), t.baseURL) {
		// Check if this is a streaming request by looking for stream=true in params or body
		isStreaming := false
		if req.URL.Query().Get("stream") == "true" {
			isStreaming = true
		} else if req.Body != nil {
			// For POST requests, we need to peek at the body to check for stream=true
			bodyBytes, err := io.ReadAll(req.Body)
			if err == nil {
				req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
				var payload map[string]any
				if err := json.Unmarshal(bodyBytes, &payload); err == nil {
					if stream, ok := payload["stream"].(bool); ok && stream {
						isStreaming = true
					}
				}
			}
		}

		// Inject the header only for streaming requests (detected via stream:true).
		// mlx_lm.server handles streaming without the Accept header in many cases,
		// but adding it for explicit streaming improves interoperability and is benign.
		if isStreaming {
			req.Header.Set("Accept", "text/event-stream")
		}
	}

	return t.inner.RoundTrip(req)
}
