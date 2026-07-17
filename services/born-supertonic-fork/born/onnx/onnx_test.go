package onnx_test

import (
	"testing"

	"github.com/born-ml/born/internal/tensor"
	"github.com/born-ml/born/onnx"
)

// mockModel implements the onnx.Model interface for testing.
type mockModel struct {
	inputNames   []string
	outputNames  []string
	opsetVersion int64
	metadata     map[string]string
	forwardFunc  func(*tensor.RawTensor) (*tensor.RawTensor, error)
}

func (m *mockModel) Forward(input *tensor.RawTensor) (*tensor.RawTensor, error) {
	if m.forwardFunc != nil {
		return m.forwardFunc(input)
	}
	// Default: return input as-is.
	return input, nil
}

func (m *mockModel) ForwardNamed(inputs map[string]*tensor.RawTensor) (map[string]*tensor.RawTensor, error) {
	// Simple mock: return first input as output.
	outputs := make(map[string]*tensor.RawTensor)
	for name, t := range inputs {
		outputs[name+"_out"] = t
		break
	}
	return outputs, nil
}

func (m *mockModel) InputNames() []string {
	return m.inputNames
}

func (m *mockModel) OutputNames() []string {
	return m.outputNames
}

func (m *mockModel) OpsetVersion() int64 {
	return m.opsetVersion
}

func (m *mockModel) Metadata() map[string]string {
	return m.metadata
}

// TestModelInterface verifies that mockModel implements onnx.Model.
func TestModelInterface(_ *testing.T) {
	// This test ensures the interface is correctly defined.
	var _ onnx.Model = &mockModel{}
}

// TestMockModel demonstrates using a mock Model for testing.
func TestMockModel(t *testing.T) {
	mock := &mockModel{
		inputNames:   []string{"input"},
		outputNames:  []string{"output"},
		opsetVersion: 13,
		metadata: map[string]string{
			"producer_name":    "pytorch",
			"producer_version": "1.9.0",
		},
	}

	// Test InputNames.
	inputs := mock.InputNames()
	if len(inputs) != 1 || inputs[0] != "input" {
		t.Errorf("InputNames() = %v, want [input]", inputs)
	}

	// Test OutputNames.
	outputs := mock.OutputNames()
	if len(outputs) != 1 || outputs[0] != "output" {
		t.Errorf("OutputNames() = %v, want [output]", outputs)
	}

	// Test OpsetVersion.
	if version := mock.OpsetVersion(); version != 13 {
		t.Errorf("OpsetVersion() = %d, want 13", version)
	}

	// Test Metadata.
	meta := mock.Metadata()
	if meta["producer_name"] != "pytorch" {
		t.Errorf("Metadata[producer_name] = %s, want pytorch", meta["producer_name"])
	}

	// Test Forward.
	dummyTensor, _ := tensor.NewRaw(tensor.Shape{1, 3}, tensor.Float32, tensor.CPU)
	result, err := mock.Forward(dummyTensor)
	if err != nil {
		t.Fatalf("Forward() error = %v", err)
	}
	if result != dummyTensor {
		t.Error("Forward() should return input tensor")
	}
}

// TestMockModelForwardNamed demonstrates named input/output handling.
func TestMockModelForwardNamed(t *testing.T) {
	mock := &mockModel{
		inputNames:  []string{"x", "y"},
		outputNames: []string{"z"},
	}

	inputs := map[string]*tensor.RawTensor{
		"x": nil, // Simplified: nil tensors for demo.
		"y": nil,
	}

	outputs, err := mock.ForwardNamed(inputs)
	if err != nil {
		t.Fatalf("ForwardNamed() error = %v", err)
	}

	// Mock returns first input as output with "_out" suffix.
	if len(outputs) != 1 {
		t.Errorf("ForwardNamed() returned %d outputs, want 1", len(outputs))
	}
}

// TestMockModelCustomForward demonstrates custom forward logic.
func TestMockModelCustomForward(t *testing.T) {
	// Create mock with custom forward function.
	customTensor, _ := tensor.NewRaw(tensor.Shape{2, 2}, tensor.Float32, tensor.CPU)

	mock := &mockModel{
		forwardFunc: func(_ *tensor.RawTensor) (*tensor.RawTensor, error) {
			// Custom logic: always return customTensor.
			return customTensor, nil
		},
	}

	dummyInput, _ := tensor.NewRaw(tensor.Shape{1, 1}, tensor.Float32, tensor.CPU)
	result, err := mock.Forward(dummyInput)
	if err != nil {
		t.Fatalf("Forward() error = %v", err)
	}

	if result != customTensor {
		t.Error("Forward() should return customTensor")
	}
}

// TestModelInterfaceUsage demonstrates typical Model usage patterns.
func TestModelInterfaceUsage(t *testing.T) {
	// Function that works with any Model implementation.
	runInference := func(model onnx.Model, input *tensor.RawTensor) (*tensor.RawTensor, error) {
		// Check model metadata.
		if len(model.InputNames()) == 0 {
			t.Error("Model has no inputs")
		}

		// Run inference.
		return model.Forward(input)
	}

	// Use with mock.
	mock := &mockModel{
		inputNames: []string{"data"},
	}

	dummyInput, _ := tensor.NewRaw(tensor.Shape{1, 3}, tensor.Float32, tensor.CPU)
	_, err := runInference(mock, dummyInput)
	if err != nil {
		t.Errorf("runInference() error = %v", err)
	}
}

// TestModelInterfaceHidesInternalImpl verifies that the interface pattern
// successfully hides internal implementation details.
func TestModelInterfaceHidesInternalImpl(_ *testing.T) {
	// This test verifies the interface pattern at compile time.
	// The key benefit: external packages cannot depend on internal types.
	//
	// Before (type alias):
	//   type Model = internalonnx.Model  // exposes "internal" in pkg.go.dev
	//
	// After (interface):
	//   type Model interface { ... }     // no internal path visible
	//
	// This allows:
	// 1. Easy mocking in tests (see mockModel above)
	// 2. Multiple implementations (optimized versions, caching, etc.)
	// 3. Clean public API without internal path exposure

	var _ onnx.Model = &mockModel{}
}
