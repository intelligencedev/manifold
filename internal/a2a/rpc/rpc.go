// Package rpc provides a simple JSON-RPC 2.0 implementation for A2A
package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sync"
)

// Custom context key types to avoid string collisions
type contextKey string

const (
	// RequestCtxKey is the context key for the HTTP request
	RequestCtxKey contextKey = "request"
	// ResponseWriterCtxKey is the context key for the HTTP response writer
	ResponseWriterCtxKey contextKey = "responseWriter"
)

// JSONRPCRequest represents a JSON-RPC 2.0 request
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	ID      interface{}     `json:"id,omitempty"`
	Params  json.RawMessage `json:"params"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response
type JSONRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      interface{}   `json:"id,omitempty"`
	Result  interface{}   `json:"result,omitempty"`
	Error   *JSONRPCError `json:"error,omitempty"`
}

// JSONRPCError represents a JSON-RPC 2.0 error
type JSONRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Standard JSON-RPC error codes
const (
	ParseErrorCode     = -32700
	InvalidRequestCode = -32600
	MethodNotFoundCode = -32601
	InvalidParamsCode  = -32602
	InternalErrorCode  = -32603
)

// A2A-specific error codes
const (
	TaskNotFoundErrorCode = -32000
	AuthErrorCode         = -32001
	ValidationErrorCode   = -32002
	ResourceExhaustedCode = -32003
	UnknownErrorCode      = -32004
)

// HandlerFunc is a function that handles a JSON-RPC method
type HandlerFunc func(ctx context.Context, rawParams json.RawMessage) (interface{}, *JSONRPCError)

// Router is a JSON-RPC method router
type Router struct {
	mu sync.RWMutex
	m  map[string]HandlerFunc
}

// NewRouter creates a new JSON-RPC router
func NewRouter() *Router {
	return &Router{m: make(map[string]HandlerFunc)}
}

// Register registers a handler for a JSON-RPC method
func (r *Router) Register(method string, h HandlerFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[method] = h
}

// ServeHTTP implements http.Handler
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// Verify content type
	if req.Header.Get("Content-Type") != "application/json" {
		writeError(w, nil, InvalidRequestCode, "Content-Type must be application/json")
		return
	}

	// Read request body
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		writeError(w, nil, ParseErrorCode, "Failed to read request body")
		return
	}

	// Close the original body
	req.Body.Close()

	// Create a new reader with the same data for later use
	req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	// Parse JSON-RPC request
	var jsonRPCReq JSONRPCRequest
	if err := json.Unmarshal(bodyBytes, &jsonRPCReq); err != nil {
		writeError(w, nil, ParseErrorCode, "Failed to parse JSON-RPC request")
		return
	}

	// Validate JSON-RPC version
	if jsonRPCReq.JSONRPC != "2.0" {
		writeError(w, jsonRPCReq.ID, InvalidRequestCode, "JSON-RPC version must be '2.0'")
		return
	}

	// Lookup handler
	r.mu.RLock()
	handler, ok := r.m[jsonRPCReq.Method]
	r.mu.RUnlock()

	if !ok {
		writeError(w, jsonRPCReq.ID, MethodNotFoundCode, "Method not found: "+jsonRPCReq.Method)
		return
	}

	// Create context with request info and response writer
	ctx := context.WithValue(req.Context(), RequestCtxKey, req)
	ctx = context.WithValue(ctx, ResponseWriterCtxKey, w)

	// Execute handler
	result, handlerErr := handler(ctx, jsonRPCReq.Params)

	// Check if this was handled via streaming
	if result == "STREAMING" {
		return // Response already sent via streaming
	}

	// Build response
	if handlerErr != nil {
		writeError(w, jsonRPCReq.ID, handlerErr.Code, handlerErr.Message)
		return
	}

	// Return successful response
	response := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      jsonRPCReq.ID,
		Result:  result,
	}

	// Write response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding JSON-RPC response: %v", err)
	}
}

// Helper function to write JSON-RPC error responses
func writeError(w http.ResponseWriter, id interface{}, code int, message string) {
	response := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK) // JSON-RPC always returns 200 OK
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding JSON-RPC error response: %v", err)
	}
}
