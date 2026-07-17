//go:build !wasm

package onnx

import (
	"testing"

	"github.com/born-ml/born/internal/tensor"
)

func TestListSupportedOps(t *testing.T) {
	ops := ListSupportedOps()

	// Should have at least basic ops
	if len(ops) < 10 {
		t.Errorf("Expected at least 10 supported ops, got %d", len(ops))
	}

	// Check for essential operators
	essentialOps := []string{"Add", "MatMul", "Relu", "Reshape", "Softmax"}
	opsMap := make(map[string]bool)
	for _, op := range ops {
		opsMap[op] = true
	}

	for _, essential := range essentialOps {
		if !opsMap[essential] {
			t.Errorf("Missing essential operator: %s", essential)
		}
	}

	t.Logf("Supported operators (%d): %v", len(ops), ops)
}

func TestTopologicalSort(t *testing.T) {
	// Create test nodes with dependencies:
	// A -> B -> C
	//      B -> D
	nodes := []NodeProto{
		{Name: "C", Inputs: []string{"b_out"}, Outputs: []string{"c_out"}},
		{Name: "A", Inputs: []string{"input"}, Outputs: []string{"a_out"}},
		{Name: "D", Inputs: []string{"b_out"}, Outputs: []string{"d_out"}},
		{Name: "B", Inputs: []string{"a_out"}, Outputs: []string{"b_out"}},
	}

	sorted := topologicalSort(nodes)

	// Build position map
	positions := make(map[string]int)
	for i, node := range sorted {
		positions[node.Name] = i
	}

	// A must come before B
	if positions["A"] >= positions["B"] {
		t.Error("A should come before B")
	}

	// B must come before C and D
	if positions["B"] >= positions["C"] {
		t.Error("B should come before C")
	}
	if positions["B"] >= positions["D"] {
		t.Error("B should come before D")
	}

	t.Logf("Topological order: %v", func() []string {
		names := make([]string, len(sorted))
		for i, n := range sorted {
			names[i] = n.Name
		}
		return names
	}())
}

func TestTensorFromProto(t *testing.T) {
	// Test float32 tensor
	proto := &TensorProto{
		Name:      "test_tensor",
		DataType:  TensorProtoFloat,
		Dims:      []int64{2, 3},
		FloatData: []float32{1, 2, 3, 4, 5, 6},
	}

	result, err := tensorFromProto(proto)
	if err != nil {
		t.Fatalf("Failed to create tensor: %v", err)
	}

	if !result.Shape().Equal([]int{2, 3}) {
		t.Errorf("Wrong shape: %v", result.Shape())
	}

	data := result.AsFloat32()
	expected := []float32{1, 2, 3, 4, 5, 6}
	for i, v := range expected {
		if data[i] != v {
			t.Errorf("data[%d] = %v, expected %v", i, data[i], v)
		}
	}
}

func TestTensorFromProtoRawData(t *testing.T) {
	// Test raw binary data (little-endian float32)
	rawData := []byte{
		0x00, 0x00, 0x80, 0x3f, // 1.0
		0x00, 0x00, 0x00, 0x40, // 2.0
		0x00, 0x00, 0x40, 0x40, // 3.0
	}

	proto := &TensorProto{
		Name:     "raw_tensor",
		DataType: TensorProtoFloat,
		Dims:     []int64{3},
		RawData:  rawData,
	}

	result, err := tensorFromProto(proto)
	if err != nil {
		t.Fatalf("Failed to create tensor: %v", err)
	}

	data := result.AsFloat32()
	expected := []float32{1.0, 2.0, 3.0}
	for i, v := range expected {
		if data[i] != v {
			t.Errorf("data[%d] = %v, expected %v", i, data[i], v)
		}
	}
}

func TestModelCompileSimple(t *testing.T) {
	// Create a simple model: Add(a, b) -> output
	proto := &ModelProto{
		IRVersion: 7,
		OpsetImport: []OperatorSetID{
			{Domain: "", Version: 17},
		},
		Graph: &GraphProto{
			Inputs: []ValueInfoProto{
				{Name: "a"},
				{Name: "b"},
			},
			Outputs: []ValueInfoProto{
				{Name: "output"},
			},
			Nodes: []NodeProto{
				{
					Name:    "add_node",
					OpType:  "Add",
					Inputs:  []string{"a", "b"},
					Outputs: []string{"output"},
				},
			},
		},
	}

	backend := tensor.NewMockBackend()
	model, err := LoadFromProto(proto, backend, DefaultLoadOptions())
	if err != nil {
		t.Fatalf("Failed to load model: %v", err)
	}

	// Verify structure
	if len(model.InputNames()) != 2 {
		t.Errorf("Expected 2 inputs, got %d", len(model.InputNames()))
	}
	if len(model.OutputNames()) != 1 {
		t.Errorf("Expected 1 output, got %d", len(model.OutputNames()))
	}
	if model.OpsetVersion() != 17 {
		t.Errorf("Expected opset 17, got %d", model.OpsetVersion())
	}
}

func TestModelForward(t *testing.T) {
	// Create a model: Add(a, b) -> output
	proto := &ModelProto{
		IRVersion: 7,
		OpsetImport: []OperatorSetID{
			{Domain: "", Version: 17},
		},
		Graph: &GraphProto{
			Inputs: []ValueInfoProto{
				{Name: "input"},
			},
			Outputs: []ValueInfoProto{
				{Name: "output"},
			},
			Initializers: []TensorProto{
				{
					Name:      "weight",
					DataType:  TensorProtoFloat,
					Dims:      []int64{3},
					FloatData: []float32{1, 1, 1},
				},
			},
			Nodes: []NodeProto{
				{
					Name:    "add_node",
					OpType:  "Add",
					Inputs:  []string{"input", "weight"},
					Outputs: []string{"output"},
				},
			},
		},
	}

	backend := tensor.NewMockBackend()
	model, err := LoadFromProto(proto, backend, DefaultLoadOptions())
	if err != nil {
		t.Fatalf("Failed to load model: %v", err)
	}

	// Create input
	input, err := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatalf("Failed to create input: %v", err)
	}
	inputData := input.AsFloat32()
	inputData[0] = 2
	inputData[1] = 3
	inputData[2] = 4

	// Run inference
	output, err := model.Forward(input)
	if err != nil {
		t.Fatalf("Forward failed: %v", err)
	}

	// Check result: [2,3,4] + [1,1,1] = [3,4,5]
	outputData := output.AsFloat32()
	expected := []float32{3, 4, 5}
	for i, v := range expected {
		if outputData[i] != v {
			t.Errorf("output[%d] = %v, expected %v", i, outputData[i], v)
		}
	}
}

func TestModelForwardNamed(t *testing.T) {
	// Create a model with multiple inputs: Add(a, b) -> output
	proto := &ModelProto{
		IRVersion: 7,
		OpsetImport: []OperatorSetID{
			{Domain: "", Version: 17},
		},
		Graph: &GraphProto{
			Inputs: []ValueInfoProto{
				{Name: "a"},
				{Name: "b"},
			},
			Outputs: []ValueInfoProto{
				{Name: "output"},
			},
			Nodes: []NodeProto{
				{
					Name:    "add_node",
					OpType:  "Add",
					Inputs:  []string{"a", "b"},
					Outputs: []string{"output"},
				},
			},
		},
	}

	backend := tensor.NewMockBackend()
	model, err := LoadFromProto(proto, backend, DefaultLoadOptions())
	if err != nil {
		t.Fatalf("Failed to load model: %v", err)
	}

	// Create inputs
	a, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, tensor.CPU)
	b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, tensor.CPU)

	aData := a.AsFloat32()
	bData := b.AsFloat32()
	for i := 0; i < 3; i++ {
		aData[i] = float32(i + 1)   // [1, 2, 3]
		bData[i] = float32(i+1) * 2 // [2, 4, 6]
	}

	// Run inference
	outputs, err := model.ForwardNamed(map[string]*tensor.RawTensor{
		"a": a,
		"b": b,
	})
	if err != nil {
		t.Fatalf("ForwardNamed failed: %v", err)
	}

	// Check result: [1,2,3] + [2,4,6] = [3,6,9]
	output := outputs["output"]
	outputData := output.AsFloat32()
	expected := []float32{3, 6, 9}
	for i, v := range expected {
		if outputData[i] != v {
			t.Errorf("output[%d] = %v, expected %v", i, outputData[i], v)
		}
	}
}

func TestModelChainedOps(t *testing.T) {
	// Create a model: input -> Add(+1) -> Mul(*2) -> output
	proto := &ModelProto{
		IRVersion: 7,
		OpsetImport: []OperatorSetID{
			{Domain: "", Version: 17},
		},
		Graph: &GraphProto{
			Inputs: []ValueInfoProto{
				{Name: "input"},
			},
			Outputs: []ValueInfoProto{
				{Name: "output"},
			},
			Initializers: []TensorProto{
				{
					Name:      "one",
					DataType:  TensorProtoFloat,
					Dims:      []int64{1},
					FloatData: []float32{1},
				},
				{
					Name:      "two",
					DataType:  TensorProtoFloat,
					Dims:      []int64{1},
					FloatData: []float32{2},
				},
			},
			Nodes: []NodeProto{
				{
					Name:    "add_node",
					OpType:  "Add",
					Inputs:  []string{"input", "one"},
					Outputs: []string{"added"},
				},
				{
					Name:    "mul_node",
					OpType:  "Mul",
					Inputs:  []string{"added", "two"},
					Outputs: []string{"output"},
				},
			},
		},
	}

	backend := tensor.NewMockBackend()
	model, err := LoadFromProto(proto, backend, DefaultLoadOptions())
	if err != nil {
		t.Fatalf("Failed to load model: %v", err)
	}

	// Create input [3]
	input, _ := tensor.NewRaw(tensor.Shape{1}, tensor.Float32, tensor.CPU)
	input.AsFloat32()[0] = 3

	// Run inference: (3 + 1) * 2 = 8
	output, err := model.Forward(input)
	if err != nil {
		t.Fatalf("Forward failed: %v", err)
	}

	result := output.AsFloat32()[0]
	if result != 8 {
		t.Errorf("output = %v, expected 8", result)
	}
}

func TestValidateOperators(t *testing.T) {
	graph := &GraphProto{
		Nodes: []NodeProto{
			{OpType: "Add"},
			{OpType: "UnsupportedOp"},
			{OpType: "Relu"},
		},
	}

	err := validateOperators(graph, nil)
	// Should fail with nil registry
	if err == nil {
		t.Error("Expected error with nil registry")
	}
}
