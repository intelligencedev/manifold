package sse

import (
	"net/http"
)

type SSEWriter struct {
	w http.ResponseWriter
	f http.Flusher
}

func NewSSEWriter(w http.ResponseWriter) *SSEWriter {
	flusher, _ := w.(http.Flusher)
	return &SSEWriter{w: w, f: flusher}
}

func (s *SSEWriter) Send(resp JSONRPCResponse) error {
	// marshal resp → data field, write "data:...\n\n", flush 
	return nil
}