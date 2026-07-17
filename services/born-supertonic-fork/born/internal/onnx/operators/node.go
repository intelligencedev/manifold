//go:build !wasm

// Package operators provides ONNX operator implementations.
package operators

import "github.com/born-ml/born/internal/tensor"

// ONNX data types (TensorProto.DataType).
const (
	TensorProtoUndefined = 0
	TensorProtoFloat     = 1  // float32
	TensorProtoUint8     = 2  // uint8
	TensorProtoInt8      = 3  // int8
	TensorProtoUint16    = 4  // uint16
	TensorProtoInt16     = 5  // int16
	TensorProtoInt32     = 6  // int32
	TensorProtoInt64     = 7  // int64
	TensorProtoString    = 8  // string
	TensorProtoBool      = 9  // bool
	TensorProtoFloat16   = 10 // float16
	TensorProtoDouble    = 11 // float64
	TensorProtoUint32    = 12 // uint32
	TensorProtoUint64    = 13 // uint64
)

// ONNX auto_pad attribute values. NOTSET is the spec default (use explicit pads);
// VALID means no padding. SAME_UPPER/SAME_LOWER are not currently supported.
const (
	autoPadNotset = "NOTSET"
	autoPadValid  = "VALID"
)

// Node represents an ONNX operation node.
// This is a local copy of the relevant fields from onnx.NodeProto
// to avoid import cycles between onnx and operators packages.
type Node struct {
	Name       string      // Node name (optional)
	OpType     string      // Operation type (e.g., "Conv", "MatMul", "Relu")
	Inputs     []string    // Input tensor names
	Outputs    []string    // Output tensor names
	Attributes []Attribute // Operation attributes
	Domain     string      // Custom domain (empty for default)
}

// Attribute represents a node attribute.
type Attribute struct {
	Name    string            // Attribute name
	Type    int32             // Attribute type
	F       float32           // FLOAT value
	I       int64             // INT value
	S       []byte            // STRING value
	T       *tensor.RawTensor // TENSOR value
	Floats  []float32         // FLOATS array
	Ints    []int64           // INTS array
	Strings [][]byte          // STRINGS array
}

// GetAttrInt returns an integer attribute or default value.
func GetAttrInt(node *Node, name string, defaultVal int64) int64 {
	for i := range node.Attributes {
		if node.Attributes[i].Name == name {
			return node.Attributes[i].I
		}
	}
	return defaultVal
}

// GetAttrInts returns an integer array attribute.
func GetAttrInts(node *Node, name string) []int64 {
	for i := range node.Attributes {
		if node.Attributes[i].Name == name {
			return node.Attributes[i].Ints
		}
	}
	return nil
}

// GetAttrFloat returns a float attribute or default value.
func GetAttrFloat(node *Node, name string, defaultVal float32) float32 {
	for i := range node.Attributes {
		if node.Attributes[i].Name == name {
			return node.Attributes[i].F
		}
	}
	return defaultVal
}

// GetAttrFloats returns a float array attribute.
func GetAttrFloats(node *Node, name string) []float32 {
	for i := range node.Attributes {
		if node.Attributes[i].Name == name {
			return node.Attributes[i].Floats
		}
	}
	return nil
}

// GetAttrString returns a string attribute or default value.
func GetAttrString(node *Node, name, defaultVal string) string {
	for i := range node.Attributes {
		if node.Attributes[i].Name == name {
			return string(node.Attributes[i].S)
		}
	}
	return defaultVal
}
