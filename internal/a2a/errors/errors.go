// Package errors provides error handling for the A2A protocol
package errors

import (
	"encoding/json"
)

const (
	CodeTaskNotFound          = -32001
	CodeTaskNotCancelable     = -32002
	CodePushNotSupported      = -32003
	CodeOperationNotSupported = -32004
)

func NewJSONRPCError(code int, msg string, data interface{}) *JSONRPCError {
	rawData, _ := json.Marshal(data)
	return &JSONRPCError{code, msg, rawData}
}

// JSONRPCError represents an error in JSON-RPC 2.0 format
type JSONRPCError struct {
	Code    int
	Message string
	Data    json.RawMessage
}
