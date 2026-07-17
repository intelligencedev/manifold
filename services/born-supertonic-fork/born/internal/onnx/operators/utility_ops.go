//go:build !wasm

package operators

import (
	"fmt"

	"github.com/born-ml/born/internal/tensor"
)

// registerUtilityOps adds utility operators to the registry.
func (r *Registry) registerUtilityOps() {
	r.Register("Identity", handleIdentity)
	r.Register("Dropout", handleDropout)
	r.Register("Constant", handleConstant)
	r.Register("Cast", handleCast)
	r.Register("ConstantOfShape", handleConstantOfShape)
	r.Register("Shape", handleShape)
	r.Register("Size", handleSize)
	r.Register("Where", handleWhere)
}

func handleIdentity(_ *Context, _ *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) != 1 {
		return nil, fmt.Errorf("identity requires 1 input, got %d", len(inputs))
	}
	// Identity just passes through
	return inputs, nil
}

func handleDropout(_ *Context, _ *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) < 1 {
		return nil, fmt.Errorf("dropout requires at least 1 input, got %d", len(inputs))
	}
	// In inference mode, Dropout is identity
	// Return input and optional mask (all true)
	outputs := []*tensor.RawTensor{inputs[0]}

	// If model expects mask output, create one filled with true (1.0)
	// This is optional - many models don't use the mask output
	return outputs, nil
}

//nolint:gocognit // handleConstant handles multiple attribute types.
func handleConstant(_ *Context, node *Node, _ []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	// Look for 'value' attribute which contains TensorProto
	for i := range node.Attributes {
		attr := &node.Attributes[i]
		if attr.Name == "value" {
			// attr.T contains embedded TensorProto
			t, err := tensorFromAttribute(attr)
			if err != nil {
				return nil, fmt.Errorf("constant: %w", err)
			}
			return []*tensor.RawTensor{t}, nil
		}
		if attr.Name == "value_float" {
			// Single float value
			t, err := tensor.NewRaw(tensor.Shape{1}, tensor.Float32, tensor.CPU)
			if err != nil {
				return nil, fmt.Errorf("constant value_float: %w", err)
			}
			t.AsFloat32()[0] = attr.F
			return []*tensor.RawTensor{t}, nil
		}
		if attr.Name == "value_int" {
			// Single int value
			t, err := tensor.NewRaw(tensor.Shape{1}, tensor.Int64, tensor.CPU)
			if err != nil {
				return nil, fmt.Errorf("constant value_int: %w", err)
			}
			t.AsInt64()[0] = attr.I
			return []*tensor.RawTensor{t}, nil
		}
		if attr.Name == "value_floats" {
			// Float array
			t, err := tensor.NewRaw(tensor.Shape{len(attr.Floats)}, tensor.Float32, tensor.CPU)
			if err != nil {
				return nil, fmt.Errorf("constant value_floats: %w", err)
			}
			copy(t.AsFloat32(), attr.Floats)
			return []*tensor.RawTensor{t}, nil
		}
		if attr.Name == "value_ints" {
			// Int array
			t, err := tensor.NewRaw(tensor.Shape{len(attr.Ints)}, tensor.Int64, tensor.CPU)
			if err != nil {
				return nil, fmt.Errorf("constant value_ints: %w", err)
			}
			copy(t.AsInt64(), attr.Ints)
			return []*tensor.RawTensor{t}, nil
		}
	}
	return nil, fmt.Errorf("constant: no value attribute found")
}

func handleCast(_ *Context, node *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) != 1 {
		return nil, fmt.Errorf("cast requires 1 input, got %d", len(inputs))
	}

	to := int(GetAttrInt(node, "to", 1))
	dtype := onnxTypeToTensorType(to)

	result, err := tensor.Cast(inputs[0], dtype)
	if err != nil {
		return nil, fmt.Errorf("cast: %w", err)
	}
	return []*tensor.RawTensor{result}, nil
}

func handleConstantOfShape(_ *Context, node *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) != 1 {
		return nil, fmt.Errorf("constantOfShape requires 1 input (shape), got %d", len(inputs))
	}

	// Get shape from input
	shapeData := inputs[0].AsInt64()
	targetShape := make(tensor.Shape, len(shapeData))
	for i, v := range shapeData {
		targetShape[i] = int(v)
	}

	// The "value" attribute is a scalar tensor whose dtype determines the output
	// dtype (default: float32 0). Honoring int64 here is required by the
	// Supertonic graphs, which build shape vectors via ConstantOfShape + int Mul.
	dtype := tensor.Float32
	var valTensor *tensor.RawTensor
	for i := range node.Attributes {
		if node.Attributes[i].Name == "value" && node.Attributes[i].T != nil {
			valTensor = node.Attributes[i].T
			dtype = valTensor.DType()
			break
		}
	}

	result, err := tensor.NewRaw(targetShape, dtype, tensor.CPU)
	if err != nil {
		return nil, fmt.Errorf("constantOfShape: %w", err)
	}
	hasVal := valTensor != nil && valTensor.NumElements() > 0
	switch dtype {
	case tensor.Float32:
		var v float32
		if hasVal {
			v = valTensor.AsFloat32()[0]
		}
		out := result.AsFloat32()
		for i := range out {
			out[i] = v
		}
	case tensor.Int64:
		var v int64
		if hasVal {
			v = valTensor.AsInt64()[0]
		}
		out := result.AsInt64()
		for i := range out {
			out[i] = v
		}
	case tensor.Int32:
		var v int32
		if hasVal {
			v = valTensor.AsInt32()[0]
		}
		out := result.AsInt32()
		for i := range out {
			out[i] = v
		}
	default:
		return nil, fmt.Errorf("constantOfShape: unsupported dtype %s", dtype)
	}
	return []*tensor.RawTensor{result}, nil
}

func handleShape(_ *Context, _ *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) != 1 {
		return nil, fmt.Errorf("shape requires 1 input, got %d", len(inputs))
	}

	shape := inputs[0].Shape()
	result, err := tensor.NewRaw(tensor.Shape{len(shape)}, tensor.Int64, tensor.CPU)
	if err != nil {
		return nil, fmt.Errorf("shape: %w", err)
	}

	data := result.AsInt64()
	for i, v := range shape {
		data[i] = int64(v)
	}

	return []*tensor.RawTensor{result}, nil
}

func handleSize(_ *Context, _ *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) != 1 {
		return nil, fmt.Errorf("size requires 1 input, got %d", len(inputs))
	}

	size := inputs[0].NumElements()
	result, err := tensor.NewRaw(tensor.Shape{1}, tensor.Int64, tensor.CPU)
	if err != nil {
		return nil, fmt.Errorf("size: %w", err)
	}
	result.AsInt64()[0] = int64(size)

	return []*tensor.RawTensor{result}, nil
}

func handleWhere(_ *Context, _ *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) != 3 {
		return nil, fmt.Errorf("where requires 3 inputs (condition, X, Y), got %d", len(inputs))
	}

	result, err := tensor.WhereRaw(inputs[0], inputs[1], inputs[2])
	if err != nil {
		return nil, fmt.Errorf("where: %w", err)
	}
	return []*tensor.RawTensor{result}, nil
}

// tensorFromAttribute creates a RawTensor from an ONNX TensorProto attribute.
func tensorFromAttribute(attr *Attribute) (*tensor.RawTensor, error) {
	// The attribute contains embedded tensor data
	// This is typically used for small constant values
	if attr.T != nil {
		return attr.T, nil
	}
	if len(attr.Floats) > 0 {
		t, err := tensor.NewRaw(tensor.Shape{len(attr.Floats)}, tensor.Float32, tensor.CPU)
		if err != nil {
			return nil, err
		}
		copy(t.AsFloat32(), attr.Floats)
		return t, nil
	}
	if len(attr.Ints) > 0 {
		t, err := tensor.NewRaw(tensor.Shape{len(attr.Ints)}, tensor.Int64, tensor.CPU)
		if err != nil {
			return nil, err
		}
		copy(t.AsInt64(), attr.Ints)
		return t, nil
	}
	return nil, fmt.Errorf("unsupported tensor attribute format")
}

// onnxTypeToTensorType converts ONNX data type to tensor.DataType.
func onnxTypeToTensorType(onnxType int) tensor.DataType {
	switch onnxType {
	case TensorProtoFloat:
		return tensor.Float32
	case TensorProtoDouble:
		return tensor.Float64
	case TensorProtoInt32:
		return tensor.Int32
	case TensorProtoInt64:
		return tensor.Int64
	case TensorProtoUint8:
		return tensor.Uint8
	case TensorProtoBool:
		return tensor.Bool
	default:
		return tensor.Float32 // default fallback
	}
}
