package onnx

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// TestParseSimpleAdd tests parsing a simple Add operation.
func TestParseSimpleAdd(t *testing.T) {
	// Create minimal ONNX model: Z = X + Y
	data := buildSimpleAddModel()

	model, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Verify model structure
	if model.IRVersion != 7 {
		t.Errorf("Expected IR version 7, got %d", model.IRVersion)
	}

	if model.Graph == nil {
		t.Fatal("Graph is nil")
	}

	if len(model.Graph.Nodes) != 1 {
		t.Errorf("Expected 1 node, got %d", len(model.Graph.Nodes))
	}

	node := model.Graph.Nodes[0]
	if node.OpType != "Add" {
		t.Errorf("Expected OpType 'Add', got '%s'", node.OpType)
	}

	if len(node.Inputs) != 2 {
		t.Errorf("Expected 2 inputs, got %d", len(node.Inputs))
	}

	if len(node.Outputs) != 1 {
		t.Errorf("Expected 1 output, got %d", len(node.Outputs))
	}
}

// TestParseWithInitializer tests parsing a model with weight tensors.
func TestParseWithInitializer(t *testing.T) {
	// Create model with MatMul + initializer weight
	data := buildMatMulModel()

	model, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if model.Graph == nil {
		t.Fatal("Graph is nil")
	}

	if len(model.Graph.Initializers) != 1 {
		t.Errorf("Expected 1 initializer, got %d", len(model.Graph.Initializers))
	}

	init := model.Graph.Initializers[0]
	if init.Name != "W" {
		t.Errorf("Expected initializer name 'W', got '%s'", init.Name)
	}

	if init.DataType != TensorProtoFloat {
		t.Errorf("Expected data type float32, got %d", init.DataType)
	}

	if len(init.Dims) != 2 {
		t.Errorf("Expected 2 dims, got %d", len(init.Dims))
	}

	// Verify raw data exists
	expectedSize := 4 * 4 * 4 // 4x4 matrix, float32 = 4 bytes
	if len(init.RawData) != expectedSize {
		t.Errorf("Expected raw data size %d, got %d", expectedSize, len(init.RawData))
	}
}

// TestParseInputOutput tests parsing input/output specifications.
func TestParseInputOutput(t *testing.T) {
	data := buildSimpleAddModel()

	model, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(model.Graph.Inputs) != 2 {
		t.Errorf("Expected 2 inputs, got %d", len(model.Graph.Inputs))
	}

	if len(model.Graph.Outputs) != 1 {
		t.Errorf("Expected 1 output, got %d", len(model.Graph.Outputs))
	}

	// Check input type info
	input := model.Graph.Inputs[0]
	if input.Name != "X" {
		t.Errorf("Expected input name 'X', got '%s'", input.Name)
	}

	if input.Type == nil || input.Type.TensorType == nil {
		t.Fatal("Input type info is nil")
	}

	if input.Type.TensorType.ElemType != TensorProtoFloat {
		t.Errorf("Expected float32 type, got %d", input.Type.TensorType.ElemType)
	}
}

// TestParseOpsetVersion tests parsing opset version.
func TestParseOpsetVersion(t *testing.T) {
	data := buildSimpleAddModel()

	model, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(model.OpsetImport) != 1 {
		t.Errorf("Expected 1 opset import, got %d", len(model.OpsetImport))
	}

	opset := model.OpsetImport[0]
	if opset.Version != 13 {
		t.Errorf("Expected opset version 13, got %d", opset.Version)
	}
}

// TestParseAttributes tests parsing node attributes.
func TestParseAttributes(t *testing.T) {
	// Build Conv node with attributes
	data := buildConvModel()

	model, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(model.Graph.Nodes) != 1 {
		t.Fatalf("Expected 1 node, got %d", len(model.Graph.Nodes))
	}

	node := model.Graph.Nodes[0]
	if node.OpType != "Conv" {
		t.Errorf("Expected OpType 'Conv', got '%s'", node.OpType)
	}

	// Check for kernel_shape attribute
	var kernelShape *AttributeProto
	for i := range node.Attributes {
		if node.Attributes[i].Name == "kernel_shape" {
			kernelShape = &node.Attributes[i]
			break
		}
	}

	if kernelShape == nil {
		t.Fatal("kernel_shape attribute not found")
	}

	if len(kernelShape.Ints) != 2 {
		t.Errorf("Expected 2 ints in kernel_shape, got %d", len(kernelShape.Ints))
	}

	if kernelShape.Ints[0] != 3 || kernelShape.Ints[1] != 3 {
		t.Errorf("Expected kernel_shape [3, 3], got [%d, %d]",
			kernelShape.Ints[0], kernelShape.Ints[1])
	}
}

// TestParseFile tests parsing from file.
func TestParseFile(t *testing.T) {
	// Create temporary file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.onnx")

	data := buildSimpleAddModel()
	if err := os.WriteFile(tmpFile, data, 0o600); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	// Parse file
	model, err := ParseFile(tmpFile)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	if model.Graph == nil {
		t.Fatal("Graph is nil")
	}

	if len(model.Graph.Nodes) != 1 {
		t.Errorf("Expected 1 node, got %d", len(model.Graph.Nodes))
	}
}

// TestParseInvalidFile tests error handling for non-existent file.
func TestParseInvalidFile(t *testing.T) {
	_, err := ParseFile("/nonexistent/file.onnx")
	if err == nil {
		t.Error("Expected error for non-existent file, got nil")
	}
}

// TestParseEmptyData tests error handling for empty data.
func TestParseEmptyData(t *testing.T) {
	_, err := Parse([]byte{})
	// Should return model with defaults or error
	if err != nil {
		// Error is acceptable for empty data
		t.Logf("Empty data error (expected): %v", err)
	}
}

// Helper: buildSimpleAddModel creates a minimal ONNX model with Add operation.
func buildSimpleAddModel() []byte {
	buf := &protoBuilder{}

	// ModelProto
	buf.startMessage() // model start

	// field 1: ir_version = 7
	buf.writeTag(1, wireVarint)
	buf.writeVarint(7)

	// field 8: opset_import
	buf.writeTag(8, wireBytes)
	opsetData := &protoBuilder{}
	opsetData.startMessage()
	// domain (empty for default)
	opsetData.writeTag(1, wireBytes)
	opsetData.writeBytes([]byte(""))
	// version = 13
	opsetData.writeTag(2, wireVarint)
	opsetData.writeVarint(13)
	opsetData.endMessage()
	buf.writeBytes(opsetData.data[4:]) // skip length prefix

	// field 7: graph
	buf.writeTag(7, wireBytes)
	graphData := buildSimpleAddGraph()
	buf.writeBytes(graphData)

	buf.endMessage()
	return buf.data[4:] // skip length prefix
}

// buildSimpleAddGraph creates graph: Z = X + Y.
func buildSimpleAddGraph() []byte {
	buf := &protoBuilder{}
	buf.startMessage()

	// field 2: name
	buf.writeTag(2, wireBytes)
	buf.writeBytes([]byte("simple_add"))

	// field 1: node (Add)
	buf.writeTag(1, wireBytes)
	nodeData := &protoBuilder{}
	nodeData.startMessage()
	// input[0] = "X"
	nodeData.writeTag(1, wireBytes)
	nodeData.writeBytes([]byte("X"))
	// input[1] = "Y"
	nodeData.writeTag(1, wireBytes)
	nodeData.writeBytes([]byte("Y"))
	// output[0] = "Z"
	nodeData.writeTag(2, wireBytes)
	nodeData.writeBytes([]byte("Z"))
	// op_type = "Add"
	nodeData.writeTag(4, wireBytes)
	nodeData.writeBytes([]byte("Add"))
	nodeData.endMessage()
	buf.writeBytes(nodeData.data[4:])

	// field 11: input X
	buf.writeTag(11, wireBytes)
	buf.writeBytes(buildValueInfo("X", TensorProtoFloat, []int64{-1, 784}))

	// field 11: input Y
	buf.writeTag(11, wireBytes)
	buf.writeBytes(buildValueInfo("Y", TensorProtoFloat, []int64{-1, 784}))

	// field 12: output Z
	buf.writeTag(12, wireBytes)
	buf.writeBytes(buildValueInfo("Z", TensorProtoFloat, []int64{-1, 784}))

	buf.endMessage()
	return buf.data[4:]
}

// buildMatMulModel creates a model with MatMul and weight initializer.
func buildMatMulModel() []byte {
	buf := &protoBuilder{}
	buf.startMessage()

	// ir_version
	buf.writeTag(1, wireVarint)
	buf.writeVarint(7)

	// opset_import
	buf.writeTag(8, wireBytes)
	opsetData := &protoBuilder{}
	opsetData.startMessage()
	opsetData.writeTag(1, wireBytes)
	opsetData.writeBytes([]byte(""))
	opsetData.writeTag(2, wireVarint)
	opsetData.writeVarint(13)
	opsetData.endMessage()
	buf.writeBytes(opsetData.data[4:])

	// graph
	buf.writeTag(7, wireBytes)
	graphData := buildMatMulGraph()
	buf.writeBytes(graphData)

	buf.endMessage()
	return buf.data[4:]
}

// buildMatMulGraph creates graph: Y = MatMul(X, W).
func buildMatMulGraph() []byte {
	buf := &protoBuilder{}
	buf.startMessage()

	// name
	buf.writeTag(2, wireBytes)
	buf.writeBytes([]byte("matmul_graph"))

	// node (MatMul)
	buf.writeTag(1, wireBytes)
	nodeData := &protoBuilder{}
	nodeData.startMessage()
	nodeData.writeTag(1, wireBytes)
	nodeData.writeBytes([]byte("X"))
	nodeData.writeTag(1, wireBytes)
	nodeData.writeBytes([]byte("W"))
	nodeData.writeTag(2, wireBytes)
	nodeData.writeBytes([]byte("Y"))
	nodeData.writeTag(4, wireBytes)
	nodeData.writeBytes([]byte("MatMul"))
	nodeData.endMessage()
	buf.writeBytes(nodeData.data[4:])

	// initializer W (4x4 matrix)
	buf.writeTag(5, wireBytes)
	buf.writeBytes(buildTensorProto("W", TensorProtoFloat, []int64{4, 4}, make([]byte, 64)))

	// input X
	buf.writeTag(11, wireBytes)
	buf.writeBytes(buildValueInfo("X", TensorProtoFloat, []int64{-1, 4}))

	// output Y
	buf.writeTag(12, wireBytes)
	buf.writeBytes(buildValueInfo("Y", TensorProtoFloat, []int64{-1, 4}))

	buf.endMessage()
	return buf.data[4:]
}

// buildConvModel creates a model with Conv operation and attributes.
func buildConvModel() []byte {
	buf := &protoBuilder{}
	buf.startMessage()

	// ir_version
	buf.writeTag(1, wireVarint)
	buf.writeVarint(7)

	// opset_import
	buf.writeTag(8, wireBytes)
	opsetData := &protoBuilder{}
	opsetData.startMessage()
	opsetData.writeTag(1, wireBytes)
	opsetData.writeBytes([]byte(""))
	opsetData.writeTag(2, wireVarint)
	opsetData.writeVarint(13)
	opsetData.endMessage()
	buf.writeBytes(opsetData.data[4:])

	// graph
	buf.writeTag(7, wireBytes)
	graphData := buildConvGraph()
	buf.writeBytes(graphData)

	buf.endMessage()
	return buf.data[4:]
}

// buildConvGraph creates graph with Conv node.
func buildConvGraph() []byte {
	buf := &protoBuilder{}
	buf.startMessage()

	// name
	buf.writeTag(2, wireBytes)
	buf.writeBytes([]byte("conv_graph"))

	// node (Conv)
	buf.writeTag(1, wireBytes)
	nodeData := &protoBuilder{}
	nodeData.startMessage()
	// inputs
	nodeData.writeTag(1, wireBytes)
	nodeData.writeBytes([]byte("X"))
	nodeData.writeTag(1, wireBytes)
	nodeData.writeBytes([]byte("W"))
	// output
	nodeData.writeTag(2, wireBytes)
	nodeData.writeBytes([]byte("Y"))
	// op_type
	nodeData.writeTag(4, wireBytes)
	nodeData.writeBytes([]byte("Conv"))
	// attribute: kernel_shape = [3, 3]
	nodeData.writeTag(5, wireBytes)
	attrData := &protoBuilder{}
	attrData.startMessage()
	attrData.writeTag(1, wireBytes)
	attrData.writeBytes([]byte("kernel_shape"))
	attrData.writeTag(20, wireVarint)
	attrData.writeVarint(int64(AttributeProtoInts))
	attrData.writeTag(8, wireBytes) // ints (field 8 in ONNX AttributeProto)
	intsData := &protoBuilder{}
	intsData.writeVarint(3)
	intsData.writeVarint(3)
	attrData.writeBytes(intsData.data)
	attrData.endMessage()
	nodeData.writeBytes(attrData.data[4:])
	nodeData.endMessage()
	buf.writeBytes(nodeData.data[4:])

	buf.endMessage()
	return buf.data[4:]
}

// buildValueInfo creates ValueInfoProto.
//
//nolint:unparam // dtype kept for API consistency with ONNX spec
func buildValueInfo(name string, dtype int32, shape []int64) []byte {
	buf := &protoBuilder{}
	buf.startMessage()

	// name
	buf.writeTag(1, wireBytes)
	buf.writeBytes([]byte(name))

	// type
	buf.writeTag(2, wireBytes)
	typeData := &protoBuilder{}
	typeData.startMessage()
	// tensor_type
	typeData.writeTag(1, wireBytes)
	tensorTypeData := &protoBuilder{}
	tensorTypeData.startMessage()
	// elem_type
	tensorTypeData.writeTag(1, wireVarint)
	tensorTypeData.writeVarint(int64(dtype))
	// shape
	tensorTypeData.writeTag(2, wireBytes)
	shapeData := &protoBuilder{}
	shapeData.startMessage()
	for _, dim := range shape {
		shapeData.writeTag(1, wireBytes)
		dimData := &protoBuilder{}
		dimData.startMessage()
		if dim > 0 {
			dimData.writeTag(1, wireVarint)
			dimData.writeVarint(dim)
		} else {
			// dynamic dimension
			dimData.writeTag(2, wireBytes)
			dimData.writeBytes([]byte("batch"))
		}
		dimData.endMessage()
		shapeData.writeBytes(dimData.data[4:])
	}
	shapeData.endMessage()
	tensorTypeData.writeBytes(shapeData.data[4:])
	tensorTypeData.endMessage()
	typeData.writeBytes(tensorTypeData.data[4:])
	typeData.endMessage()
	buf.writeBytes(typeData.data[4:])

	buf.endMessage()
	return buf.data[4:]
}

// buildTensorProto creates TensorProto.
func buildTensorProto(name string, dtype int32, dims []int64, rawData []byte) []byte {
	buf := &protoBuilder{}
	buf.startMessage()

	// dims
	for _, dim := range dims {
		buf.writeTag(1, wireVarint)
		buf.writeVarint(dim)
	}

	// data_type
	buf.writeTag(2, wireVarint)
	buf.writeVarint(int64(dtype))

	// name (field 8 in ONNX TensorProto)
	buf.writeTag(8, wireBytes)
	buf.writeBytes([]byte(name))

	// raw_data
	buf.writeTag(9, wireBytes)
	buf.writeBytes(rawData)

	buf.endMessage()
	return buf.data[4:]
}

// protoBuilder helps construct protobuf messages.
type protoBuilder struct {
	data []byte
}

func (b *protoBuilder) startMessage() {
	// Reserve space for length prefix
	b.data = append(b.data, 0, 0, 0, 0)
}

func (b *protoBuilder) endMessage() {
	// Update length prefix
	length := len(b.data) - 4
	var lenBuf [4]byte
	n := binary.PutVarint(lenBuf[:], int64(length))
	copy(b.data[:n], lenBuf[:n])
}

func (b *protoBuilder) writeTag(fieldNum, wireType int) {
	tag := (fieldNum << 3) | wireType
	b.writeVarint(int64(tag))
}

func (b *protoBuilder) writeVarint(v int64) {
	for v >= 0x80 {
		b.data = append(b.data, byte(v)|0x80)
		v >>= 7
	}
	b.data = append(b.data, byte(v))
}

func (b *protoBuilder) writeBytes(data []byte) {
	b.writeVarint(int64(len(data)))
	b.data = append(b.data, data...)
}
