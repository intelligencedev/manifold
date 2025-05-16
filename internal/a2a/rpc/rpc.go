package rpc

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
)

type JSONRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	Method string `json:"method"`
	ID interface{} `json:"id,omitempty"`
	Params json.RawMessage `json:"params"`
}

type JSONRPCResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID interface{} `json:"id,omitempty"`
	Result interface{} `json:"result,omitempty"`
	Error *JSONRPCError `json:"error,omitempty"`
}

type JSONRPCError struct {
	Code int `json:"code"`
	Message string `json:"message"`
	Data json.RawMessage `json:"data,omitempty"`
}

type HandlerFunc func(ctx context.Context, rawParams json.RawMessage) (interface{}, *JSONRPCError)

type Router struct {
	mu sync.RWMutex
	m map[string]HandlerFunc
}

func NewRouter() *Router {
	return &Router{m: make(map[string]HandlerFunc)}
}

func (r *Router) Register(method string, h HandlerFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[method] = h
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// read + decode JSONRPCRequest
	// lookup handler, invoke, build JSONRPCResponse
	// marshal + write back
}

type Task struct {
	ID string
	Status string
	Params map[string]interface{}
}