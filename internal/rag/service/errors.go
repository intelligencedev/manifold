package service

import "errors"

// Sentinel errors used by the RAG service before business logic is implemented.
var (
    ErrNotImplemented = errors.New("rag service: not implemented")
)

