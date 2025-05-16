// Package sse provides Server-Sent Events support for A2A
package sse

import (
	"encoding/json"
	"fmt"
	"net/http"

	"manifold/internal/a2a/rpc"
)

// SSEWriter wraps an http.ResponseWriter to provide SSE functionality
type SSEWriter struct {
	w http.ResponseWriter
	f http.Flusher
}

// NewSSEWriter creates a new SSE writer
func NewSSEWriter(w http.ResponseWriter) *SSEWriter {
	// Set required headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Get the flusher interface if supported
	flusher, ok := w.(http.Flusher)
	if !ok {
		panic("Streaming is not supported by the underlying http.ResponseWriter")
	}

	return &SSEWriter{w: w, f: flusher}
}

// Send sends a JSON-RPC response as an SSE event
func (s *SSEWriter) Send(resp rpc.JSONRPCResponse) error {
	// Marshal the response to JSON
	data, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON-RPC response: %w", err)
	}

	// Write SSE format: "data: <json>\n\n"
	_, err = fmt.Fprintf(s.w, "data: %s\n\n", data)
	if err != nil {
		return fmt.Errorf("failed to write SSE event: %w", err)
	}

	// Flush to ensure the data is sent immediately
	s.f.Flush()
	return nil
}

// Close sends a final event and closes the SSE stream
func (s *SSEWriter) Close() {
	fmt.Fprintf(s.w, "event: close\ndata: {}\n\n")
	s.f.Flush()
}
