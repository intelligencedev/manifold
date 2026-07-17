//go:build !wasm

package operators

import (
	"fmt"

	"github.com/born-ml/born/internal/tensor"
)

// registerActivations adds activation operators to the registry.
func (r *Registry) registerActivations() {
	r.Register("Relu", handleRelu)
	r.Register("LeakyRelu", handleLeakyRelu)
	r.Register("PRelu", handlePRelu)
	r.Register("Sigmoid", handleSigmoid)
	r.Register("Tanh", handleTanh)
	r.Register("Softmax", handleSoftmax)
	r.Register("LogSoftmax", handleLogSoftmax)
	r.Register("Gelu", handleGelu)
	r.Register("Silu", handleSilu)
	r.Register("Clip", handleClip)
}

func handleRelu(_ *Context, _ *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) != 1 {
		return nil, fmt.Errorf("relu requires 1 input, got %d", len(inputs))
	}
	result, err := tensor.ReLU(inputs[0])
	if err != nil {
		return nil, fmt.Errorf("relu: %w", err)
	}
	return []*tensor.RawTensor{result}, nil
}

func handleLeakyRelu(_ *Context, node *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) != 1 {
		return nil, fmt.Errorf("leakyRelu requires 1 input, got %d", len(inputs))
	}
	alpha := GetAttrFloat(node, "alpha", 0.01)
	result, err := tensor.LeakyReLU(inputs[0], alpha)
	if err != nil {
		return nil, fmt.Errorf("leakyRelu: %w", err)
	}
	return []*tensor.RawTensor{result}, nil
}

func handlePRelu(_ *Context, _ *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) != 2 {
		return nil, fmt.Errorf("pRelu requires 2 inputs (x, slope), got %d", len(inputs))
	}
	result, err := tensor.PReLU(inputs[0], inputs[1])
	if err != nil {
		return nil, fmt.Errorf("pRelu: %w", err)
	}
	return []*tensor.RawTensor{result}, nil
}

func handleSigmoid(_ *Context, _ *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) != 1 {
		return nil, fmt.Errorf("sigmoid requires 1 input, got %d", len(inputs))
	}
	result, err := tensor.Sigmoid(inputs[0])
	if err != nil {
		return nil, fmt.Errorf("sigmoid: %w", err)
	}
	return []*tensor.RawTensor{result}, nil
}

func handleTanh(_ *Context, _ *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) != 1 {
		return nil, fmt.Errorf("tanh requires 1 input, got %d", len(inputs))
	}
	result, err := tensor.Tanh(inputs[0])
	if err != nil {
		return nil, fmt.Errorf("tanh: %w", err)
	}
	return []*tensor.RawTensor{result}, nil
}

func handleSoftmax(_ *Context, node *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) != 1 {
		return nil, fmt.Errorf("softmax requires 1 input, got %d", len(inputs))
	}
	axis := int(GetAttrInt(node, "axis", -1))
	result, err := tensor.Softmax(inputs[0], axis)
	if err != nil {
		return nil, fmt.Errorf("softmax: %w", err)
	}
	return []*tensor.RawTensor{result}, nil
}

func handleLogSoftmax(_ *Context, node *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) != 1 {
		return nil, fmt.Errorf("logSoftmax requires 1 input, got %d", len(inputs))
	}
	axis := int(GetAttrInt(node, "axis", -1))
	result, err := tensor.LogSoftmax(inputs[0], axis)
	if err != nil {
		return nil, fmt.Errorf("logSoftmax: %w", err)
	}
	return []*tensor.RawTensor{result}, nil
}

func handleGelu(_ *Context, _ *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) != 1 {
		return nil, fmt.Errorf("gelu requires 1 input, got %d", len(inputs))
	}
	result, err := tensor.GELU(inputs[0])
	if err != nil {
		return nil, fmt.Errorf("gelu: %w", err)
	}
	return []*tensor.RawTensor{result}, nil
}

func handleSilu(_ *Context, _ *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) != 1 {
		return nil, fmt.Errorf("silu requires 1 input, got %d", len(inputs))
	}
	result, err := tensor.SiLU(inputs[0])
	if err != nil {
		return nil, fmt.Errorf("silu: %w", err)
	}
	return []*tensor.RawTensor{result}, nil
}

func handleClip(_ *Context, node *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) < 1 {
		return nil, fmt.Errorf("clip requires at least 1 input, got %d", len(inputs))
	}

	// ONNX Clip is defined for numeric types; the Supertonic graphs clip int64
	// length/index tensors. Born's tensor.Clip is float32-only, so handle int64
	// directly here.
	if inputs[0] != nil && inputs[0].DType() == tensor.Int64 {
		return clipInt64(inputs)
	}

	// ONNX 11+: min and max are inputs, not attributes
	var minVal, maxVal *float32
	if len(inputs) >= 2 && inputs[1] != nil {
		v := inputs[1].AsFloat32()[0]
		minVal = &v
	}
	if len(inputs) >= 3 && inputs[2] != nil {
		v := inputs[2].AsFloat32()[0]
		maxVal = &v
	}

	// Fall back to attributes for older ONNX versions
	if minVal == nil {
		v := GetAttrFloat(node, "min", -3.4028235e+38)
		minVal = &v
	}
	if maxVal == nil {
		v := GetAttrFloat(node, "max", 3.4028235e+38)
		maxVal = &v
	}

	result, err := tensor.Clip(inputs[0], *minVal, *maxVal)
	if err != nil {
		return nil, fmt.Errorf("clip: %w", err)
	}
	return []*tensor.RawTensor{result}, nil
}
