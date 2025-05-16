package errors

const (
	CodeTaskNotFound = -32001
	CodeTaskNotCancelable = -32002
	CodePushNotSupported = -32003
	CodeOperationNotSupported = -32004
)

func NewJSONRPCError(code int, msg string, data any) *JSONRPCError {
	return &JSONRPCError{code, msg, json.RawMessage(json.Marshal(data))}
}

// JSONRPCError represents an error in JSON-RPC 2.0 format

type JSONRPCError struct {
	Code int
	Message string
	Data json.RawMessage
}