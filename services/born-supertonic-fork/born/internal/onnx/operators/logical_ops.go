//go:build !wasm

package operators

import (
	"fmt"

	"github.com/born-ml/born/internal/tensor"
)

// registerLogicalOps adds logical operators to the registry.
func (r *Registry) registerLogicalOps() {
	r.Register("Not", handleNot)
	r.Register("And", handleAnd)
	r.Register("Or", handleOr)
	r.Register("Xor", handleXor)
}

func handleNot(ctx *Context, _ *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) != 1 {
		return nil, fmt.Errorf("not expects 1 input, got %d", len(inputs))
	}
	result := ctx.Backend.Not(inputs[0])
	return []*tensor.RawTensor{result}, nil
}

func handleAnd(ctx *Context, _ *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) != 2 {
		return nil, fmt.Errorf("and expects 2 inputs, got %d", len(inputs))
	}
	result := ctx.Backend.And(inputs[0], inputs[1])
	return []*tensor.RawTensor{result}, nil
}

func handleOr(ctx *Context, _ *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) != 2 {
		return nil, fmt.Errorf("or expects 2 inputs, got %d", len(inputs))
	}
	result := ctx.Backend.Or(inputs[0], inputs[1])
	return []*tensor.RawTensor{result}, nil
}

func handleXor(ctx *Context, _ *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) != 2 {
		return nil, fmt.Errorf("xor expects 2 inputs, got %d", len(inputs))
	}
	a, b := inputs[0], inputs[1]
	if a.DType() != tensor.Bool || b.DType() != tensor.Bool {
		return nil, fmt.Errorf("xor expects both tensors to have bool dtype")
	}
	result := ctx.Backend.NotEqual(a, b)
	return []*tensor.RawTensor{result}, nil
}
