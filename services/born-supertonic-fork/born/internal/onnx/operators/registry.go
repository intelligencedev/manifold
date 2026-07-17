//go:build !wasm

package operators

import (
	"fmt"

	"github.com/born-ml/born/internal/tensor"
)

// OpHandler processes an ONNX node and returns output tensors.
type OpHandler func(ctx *Context, node *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error)

// Context provides backend and other execution context for operators.
type Context struct {
	Backend tensor.Backend
	// Future: Device selection, memory pool, etc.
}

// Registry maps ONNX operator types to handler functions.
type Registry struct {
	handlers map[string]OpHandler
}

// NewRegistry creates a new operator registry with all supported operators.
func NewRegistry() *Registry {
	r := &Registry{
		handlers: make(map[string]OpHandler),
	}

	// Register all operators
	r.registerMathOps()
	r.registerReduceOps()
	r.registerActivations()
	r.registerShapeOps()
	r.registerUtilityOps()
	r.registerComparisonOps()
	r.registerNormalizationOps()
	r.registerLogicalOps()
	r.registerPoolOps()
	r.registerConvOps()
	r.registerResizeOps()
	r.registerSupertonicOps()

	return r
}

// Register adds a custom operator handler.
func (r *Registry) Register(opType string, handler OpHandler) {
	r.handlers[opType] = handler
}

// Get returns the handler for an operator type.
func (r *Registry) Get(opType string) (OpHandler, bool) {
	h, ok := r.handlers[opType]
	return h, ok
}

// Execute runs an operator with the given inputs.
func (r *Registry) Execute(ctx *Context, node *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	handler, ok := r.handlers[node.OpType]
	if !ok {
		return nil, fmt.Errorf("unsupported operator: %s", node.OpType)
	}
	return handler(ctx, node, inputs)
}

// SupportedOps returns a list of all supported operator types.
func (r *Registry) SupportedOps() []string {
	ops := make([]string, 0, len(r.handlers))
	for op := range r.handlers {
		ops = append(ops, op)
	}
	return ops
}
