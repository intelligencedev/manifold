package ops

import (
	"testing"

	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/tensor"
)

// TestCatOp_Backward_Simple tests basic cat backward with 2 tensors.
func TestCatOp_Backward_Simple(t *testing.T) {
	backend := cpu.New()

	// Create inputs: [1, 2] and [3, 4, 5]
	input1, err := tensor.NewRaw(tensor.Shape{2}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create input1: %v", err)
	}
	data1 := input1.AsFloat32()
	data1[0], data1[1] = 1, 2

	input2, err := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create input2: %v", err)
	}
	data2 := input2.AsFloat32()
	data2[0], data2[1], data2[2] = 3, 4, 5

	// Cat along dim 0
	output := backend.Cat([]*tensor.RawTensor{input1, input2}, 0)

	// Create CatOp
	op := NewCatOp([]*tensor.RawTensor{input1, input2}, 0, []int{2, 3}, output)

	// Create gradient output (all ones)
	gradOutput, err := tensor.NewRaw(tensor.Shape{5}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create gradOutput: %v", err)
	}
	gradOutData := gradOutput.AsFloat32()
	for i := range gradOutData {
		gradOutData[i] = 1.0
	}

	// Backward
	grads := op.Backward(gradOutput, backend)

	// Check we got 2 gradients
	if len(grads) != 2 {
		t.Fatalf("Expected 2 gradients, got %d", len(grads))
	}

	// Check shapes
	if !grads[0].Shape().Equal(tensor.Shape{2}) {
		t.Errorf("grad[0] shape = %v, expected [2]", grads[0].Shape())
	}
	if !grads[1].Shape().Equal(tensor.Shape{3}) {
		t.Errorf("grad[1] shape = %v, expected [3]", grads[1].Shape())
	}

	// Check values (all should be 1)
	grad1 := grads[0].AsFloat32()
	for i := 0; i < 2; i++ {
		if grad1[i] != 1.0 {
			t.Errorf("grad1[%d] = %f, expected 1.0", i, grad1[i])
		}
	}

	grad2 := grads[1].AsFloat32()
	for i := 0; i < 3; i++ {
		if grad2[i] != 1.0 {
			t.Errorf("grad2[%d] = %f, expected 1.0", i, grad2[i])
		}
	}
}

// TestCatOp_Backward_2D tests cat backward with 2D tensors.
func TestCatOp_Backward_2D(t *testing.T) {
	backend := cpu.New()

	// Create inputs: [2, 3] and [2, 2]
	input1, err := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create input1: %v", err)
	}
	data1 := input1.AsFloat32()
	for i := range data1 {
		data1[i] = float32(i)
	}

	input2, err := tensor.NewRaw(tensor.Shape{2, 2}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create input2: %v", err)
	}
	data2 := input2.AsFloat32()
	for i := range data2 {
		data2[i] = float32(i + 10)
	}

	// Cat along dim 1 (columns)
	output := backend.Cat([]*tensor.RawTensor{input1, input2}, 1)

	// Create CatOp
	op := NewCatOp([]*tensor.RawTensor{input1, input2}, 1, []int{3, 2}, output)

	// Create gradient output (sequential values)
	gradOutput, err := tensor.NewRaw(tensor.Shape{2, 5}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create gradOutput: %v", err)
	}
	gradOutData := gradOutput.AsFloat32()
	for i := range gradOutData {
		gradOutData[i] = float32(i + 1)
	}

	// Backward
	grads := op.Backward(gradOutput, backend)

	// Check shapes
	if !grads[0].Shape().Equal(tensor.Shape{2, 3}) {
		t.Errorf("grad[0] shape = %v, expected [2, 3]", grads[0].Shape())
	}
	if !grads[1].Shape().Equal(tensor.Shape{2, 2}) {
		t.Errorf("grad[1] shape = %v, expected [2, 2]", grads[1].Shape())
	}

	// Check values for grad[0]
	// gradOutput layout (row-major):
	// [[1, 2, 3, 4, 5],
	//  [6, 7, 8, 9, 10]]
	// grad[0] should get columns 0-2:
	// [[1, 2, 3],
	//  [6, 7, 8]]
	grad1 := grads[0].AsFloat32()
	expected1 := []float32{1, 2, 3, 6, 7, 8}
	for i, exp := range expected1 {
		if grad1[i] != exp {
			t.Errorf("grad1[%d] = %f, expected %f", i, grad1[i], exp)
		}
	}

	// grad[1] should get columns 3-4:
	// [[4, 5],
	//  [9, 10]]
	grad2 := grads[1].AsFloat32()
	expected2 := []float32{4, 5, 9, 10}
	for i, exp := range expected2 {
		if grad2[i] != exp {
			t.Errorf("grad2[%d] = %f, expected %f", i, grad2[i], exp)
		}
	}
}

// TestCatOp_Backward_MultipleTensors tests cat backward with 3+ tensors.
func TestCatOp_Backward_MultipleTensors(t *testing.T) {
	backend := cpu.New()

	// Create 4 inputs along dim 0
	inputs := make([]*tensor.RawTensor, 4)
	sizes := []int{1, 2, 1, 3}
	totalSize := 0
	for _, s := range sizes {
		totalSize += s
	}

	for i, size := range sizes {
		input, err := tensor.NewRaw(tensor.Shape{size}, tensor.Float32, backend.Device())
		if err != nil {
			t.Fatalf("Failed to create input %d: %v", i, err)
		}
		inputs[i] = input
	}

	// Cat along dim 0
	output := backend.Cat(inputs, 0)

	// Create CatOp
	op := NewCatOp(inputs, 0, sizes, output)

	// Create gradient output
	gradOutput, err := tensor.NewRaw(tensor.Shape{totalSize}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create gradOutput: %v", err)
	}
	gradOutData := gradOutput.AsFloat32()
	for i := range gradOutData {
		gradOutData[i] = float32(i + 1)
	}

	// Backward
	grads := op.Backward(gradOutput, backend)

	// Check we got 4 gradients
	if len(grads) != 4 {
		t.Fatalf("Expected 4 gradients, got %d", len(grads))
	}

	// Check sizes and values
	offset := 0
	for i, size := range sizes {
		if !grads[i].Shape().Equal(tensor.Shape{size}) {
			t.Errorf("grad[%d] shape = %v, expected [%d]", i, grads[i].Shape(), size)
		}

		gradData := grads[i].AsFloat32()
		for j := 0; j < size; j++ {
			expected := float32(offset + j + 1)
			if gradData[j] != expected {
				t.Errorf("grad[%d][%d] = %f, expected %f", i, j, gradData[j], expected)
			}
		}
		offset += size
	}
}

// TestCatOp_Backward_NegativeDim tests cat backward with negative dimension.
func TestCatOp_Backward_NegativeDim(t *testing.T) {
	backend := cpu.New()

	// Create 2D inputs
	input1, err := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create input1: %v", err)
	}

	input2, err := tensor.NewRaw(tensor.Shape{2, 2}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create input2: %v", err)
	}

	// Cat along dim -1 (last dimension, same as dim 1)
	output := backend.Cat([]*tensor.RawTensor{input1, input2}, -1)

	// Create CatOp with normalized dim (1)
	op := NewCatOp([]*tensor.RawTensor{input1, input2}, 1, []int{3, 2}, output)

	// Create gradient output
	gradOutput, err := tensor.NewRaw(tensor.Shape{2, 5}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create gradOutput: %v", err)
	}
	gradOutData := gradOutput.AsFloat32()
	for i := range gradOutData {
		gradOutData[i] = 1.0
	}

	// Backward
	grads := op.Backward(gradOutput, backend)

	// Check shapes
	if !grads[0].Shape().Equal(tensor.Shape{2, 3}) {
		t.Errorf("grad[0] shape = %v, expected [2, 3]", grads[0].Shape())
	}
	if !grads[1].Shape().Equal(tensor.Shape{2, 2}) {
		t.Errorf("grad[1] shape = %v, expected [2, 2]", grads[1].Shape())
	}
}
