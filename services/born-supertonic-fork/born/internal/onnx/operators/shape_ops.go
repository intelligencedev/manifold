//go:build !wasm

package operators

import (
	"fmt"
	"os"

	"github.com/born-ml/born/internal/tensor"
)

// registerShapeOps adds shape manipulation operators to the registry.
func (r *Registry) registerShapeOps() {
	r.Register("Reshape", handleReshape)
	r.Register("Transpose", handleTranspose)
	r.Register("Squeeze", handleSqueeze)
	r.Register("Unsqueeze", handleUnsqueeze)
	r.Register("Concat", handleConcat)
	r.Register("Split", handleSplit)
	r.Register("Slice", handleSlice)
	r.Register("Gather", handleGather)
	r.Register("Flatten", handleFlatten)
	r.Register("Expand", handleExpand)
}

func handleReshape(_ *Context, _ *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) != 2 {
		return nil, fmt.Errorf("reshape requires 2 inputs (data, shape), got %d", len(inputs))
	}

	// Get target shape from second input
	shapeData := inputs[1].AsInt64()
	newShape := make(tensor.Shape, len(shapeData))
	for i, v := range shapeData {
		newShape[i] = int(v)
	}

	result, err := tensor.Reshape(inputs[0], newShape)
	if err != nil {
		return nil, fmt.Errorf("reshape: %w", err)
	}
	return []*tensor.RawTensor{result}, nil
}

func handleTranspose(_ *Context, node *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) != 1 {
		return nil, fmt.Errorf("transpose requires 1 input, got %d", len(inputs))
	}

	perm := GetAttrInts(node, "perm")
	var axes []int
	if len(perm) > 0 {
		axes = make([]int, len(perm))
		for i, v := range perm {
			axes[i] = int(v)
		}
	}

	result, err := tensor.TransposeAxes(inputs[0], axes...)
	if err != nil {
		return nil, fmt.Errorf("transpose: %w", err)
	}
	return []*tensor.RawTensor{result}, nil
}

func handleSqueeze(_ *Context, node *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) < 1 {
		return nil, fmt.Errorf("squeeze requires at least 1 input, got %d", len(inputs))
	}

	// ONNX 13+: axes is second input
	var axes []int
	if len(inputs) >= 2 && inputs[1] != nil {
		axesData := inputs[1].AsInt64()
		axes = make([]int, len(axesData))
		for i, v := range axesData {
			axes[i] = int(v)
		}
	} else {
		// Fall back to attribute
		axesAttr := GetAttrInts(node, "axes")
		if len(axesAttr) > 0 {
			axes = make([]int, len(axesAttr))
			for i, v := range axesAttr {
				axes[i] = int(v)
			}
		}
	}

	result, err := tensor.Squeeze(inputs[0], axes...)
	if err != nil {
		return nil, fmt.Errorf("squeeze: %w", err)
	}
	return []*tensor.RawTensor{result}, nil
}

func handleUnsqueeze(_ *Context, node *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) < 1 {
		return nil, fmt.Errorf("unsqueeze requires at least 1 input, got %d", len(inputs))
	}

	// ONNX 13+: axes is second input
	var axes []int
	if len(inputs) >= 2 && inputs[1] != nil {
		axesData := inputs[1].AsInt64()
		axes = make([]int, len(axesData))
		for i, v := range axesData {
			axes[i] = int(v)
		}
	} else {
		axesAttr := GetAttrInts(node, "axes")
		if len(axesAttr) > 0 {
			axes = make([]int, len(axesAttr))
			for i, v := range axesAttr {
				axes[i] = int(v)
			}
		}
	}

	if len(axes) == 0 {
		return nil, fmt.Errorf("unsqueeze requires axes")
	}

	result, err := tensor.Unsqueeze(inputs[0], axes...)
	if err != nil {
		return nil, fmt.Errorf("unsqueeze: %w", err)
	}
	return []*tensor.RawTensor{result}, nil
}

func handleConcat(_ *Context, node *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) < 1 {
		return nil, fmt.Errorf("concat requires at least 1 input")
	}

	axis := int(GetAttrInt(node, "axis", 0))

	result, err := tensor.Concat(inputs, axis)
	if err != nil {
		return nil, fmt.Errorf("concat: %w", err)
	}
	return []*tensor.RawTensor{result}, nil
}

func handleSplit(_ *Context, node *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) < 1 {
		return nil, fmt.Errorf("split requires at least 1 input, got %d", len(inputs))
	}

	axis := int(GetAttrInt(node, "axis", 0))

	// Get split sizes
	var splitSizes []int
	if len(inputs) >= 2 && inputs[1] != nil {
		// ONNX 13+: split sizes from input
		sizesData := inputs[1].AsInt64()
		splitSizes = make([]int, len(sizesData))
		for i, v := range sizesData {
			splitSizes[i] = int(v)
		}
	} else {
		// Fall back to attribute
		sizesAttr := GetAttrInts(node, "split")
		if len(sizesAttr) > 0 {
			splitSizes = make([]int, len(sizesAttr))
			for i, v := range sizesAttr {
				splitSizes[i] = int(v)
			}
		}
	}

	results, err := tensor.Split(inputs[0], axis, splitSizes)
	if err != nil {
		return nil, fmt.Errorf("split: %w", err)
	}
	return results, nil
}

func handleSlice(_ *Context, _ *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) < 3 {
		return nil, fmt.Errorf("slice requires at least 3 inputs (data, starts, ends), got %d", len(inputs))
	}

	starts := inputs[1].AsInt64()
	ends := inputs[2].AsInt64()

	var axes, steps []int64
	if len(inputs) >= 4 && inputs[3] != nil {
		axes = inputs[3].AsInt64()
	}
	if len(inputs) >= 5 && inputs[4] != nil {
		steps = inputs[4].AsInt64()
	}

	if bornSliceDebug {
		fmt.Printf("SLICE in=%v starts=%v ends=%v axes=%v steps=%v\n", inputs[0].Shape(), starts, ends, axes, steps)
	}

	result, err := tensor.Slice(inputs[0], starts, ends, axes, steps)
	if err != nil {
		return nil, fmt.Errorf("slice: %w", err)
	}
	return []*tensor.RawTensor{result}, nil
}

func handleGather(_ *Context, node *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) != 2 {
		return nil, fmt.Errorf("gather requires 2 inputs (data, indices), got %d", len(inputs))
	}

	axis := int(GetAttrInt(node, "axis", 0))

	result, err := tensor.Gather(inputs[0], inputs[1], axis)
	if err != nil {
		return nil, fmt.Errorf("gather: %w", err)
	}
	return []*tensor.RawTensor{result}, nil
}

func handleFlatten(_ *Context, node *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) != 1 {
		return nil, fmt.Errorf("flatten requires 1 input, got %d", len(inputs))
	}

	axis := int(GetAttrInt(node, "axis", 1))

	result, err := tensor.Flatten(inputs[0], axis)
	if err != nil {
		return nil, fmt.Errorf("flatten: %w", err)
	}
	return []*tensor.RawTensor{result}, nil
}

func handleExpand(_ *Context, _ *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) != 2 {
		return nil, fmt.Errorf("expand requires 2 inputs (input, shape), got %d", len(inputs))
	}

	// Get target shape from second input
	shapeData := inputs[1].AsInt64()
	targetShape := make(tensor.Shape, len(shapeData))
	for i, v := range shapeData {
		targetShape[i] = int(v)
	}

	// ONNX Expand broadcasts the input against the target shape (bidirectional),
	// so a target dim of 1 keeps the input's (possibly larger) dim.
	outShape, err := onnxBroadcastShape(inputs[0].Shape(), targetShape)
	if err != nil {
		return nil, fmt.Errorf("expand: %w", err)
	}

	result, err := tensor.Expand(inputs[0], outShape)
	if err != nil {
		return nil, fmt.Errorf("expand: %w", err)
	}
	return []*tensor.RawTensor{result}, nil
}

//nolint:gochecknoglobals // debug flag
var bornSliceDebug = os.Getenv("BORN_SLICE_DEBUG") != ""
