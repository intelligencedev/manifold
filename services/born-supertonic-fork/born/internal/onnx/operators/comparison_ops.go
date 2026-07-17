//go:build !wasm

package operators

import (
	"fmt"

	"github.com/born-ml/born/internal/tensor"
)

// registerComparisonOps adds comparison operators to the registry.
func (r *Registry) registerComparisonOps() {
	r.Register("Equal", handleEqual)
	r.Register("Greater", handleGreater)
	r.Register("GreaterOrEqual", handleGreaterOrEqual)
	r.Register("Less", handleLess)
	r.Register("LessOrEqual", handleLessOrEqual)
}

func handleEqual(ctx *Context, _ *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) != 2 {
		return nil, fmt.Errorf("equal requires 2 inputs, got %d", len(inputs))
	}
	result := ctx.Backend.Equal(inputs[0], inputs[1])
	return []*tensor.RawTensor{result}, nil
}

func handleGreater(ctx *Context, _ *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) != 2 {
		return nil, fmt.Errorf("greater requires 2 inputs, got %d", len(inputs))
	}
	result := ctx.Backend.Greater(inputs[0], inputs[1])
	return []*tensor.RawTensor{result}, nil
}

func handleGreaterOrEqual(ctx *Context, _ *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) != 2 {
		return nil, fmt.Errorf("greaterOrEqual requires 2 inputs, got %d", len(inputs))
	}
	result := ctx.Backend.GreaterEqual(inputs[0], inputs[1])
	return []*tensor.RawTensor{result}, nil
}

func handleLess(ctx *Context, _ *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) != 2 {
		return nil, fmt.Errorf("less requires 2 inputs, got %d", len(inputs))
	}
	result := ctx.Backend.Lower(inputs[0], inputs[1])
	return []*tensor.RawTensor{result}, nil
}

func handleLessOrEqual(ctx *Context, _ *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) != 2 {
		return nil, fmt.Errorf("lessOrEqual requires 2 inputs, got %d", len(inputs))
	}
	result := ctx.Backend.LowerEqual(inputs[0], inputs[1])
	return []*tensor.RawTensor{result}, nil
}
