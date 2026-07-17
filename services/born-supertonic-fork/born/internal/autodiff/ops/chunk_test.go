package ops

import (
	"testing"

	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/tensor"
)

// TestChunkOp_BackwardMulti_Simple tests basic chunk backward with BackwardMulti.
func TestChunkOp_BackwardMulti_Simple(t *testing.T) {
	backend := cpu.New()

	// Input: [1, 2, 3, 4, 5, 6]
	input, err := tensor.NewRaw(tensor.Shape{6}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create input: %v", err)
	}
	inputData := input.AsFloat32()
	for i := range inputData {
		inputData[i] = float32(i + 1)
	}

	// Chunk into 3 parts along dim 0
	outputs := backend.Chunk(input, 3, 0)

	// Create ChunkOp
	op := NewChunkOp(input, 3, 0, outputs)

	// Create gradients for all outputs (all ones)
	gradOutputs := make([]*tensor.RawTensor, 3)
	for i := 0; i < 3; i++ {
		gradOut, err := tensor.NewRaw(tensor.Shape{2}, tensor.Float32, backend.Device())
		if err != nil {
			t.Fatalf("Failed to create gradOutput %d: %v", i, err)
		}
		gradOutData := gradOut.AsFloat32()
		gradOutData[0], gradOutData[1] = 1.0, 1.0
		gradOutputs[i] = gradOut
	}

	// BackwardMulti
	grads := op.BackwardMulti(gradOutputs, backend)

	// Check we got 1 gradient
	if len(grads) != 1 {
		t.Fatalf("Expected 1 gradient, got %d", len(grads))
	}

	// Check shape
	if !grads[0].Shape().Equal(tensor.Shape{6}) {
		t.Errorf("grad shape = %v, expected [6]", grads[0].Shape())
	}

	// Check values (all should be 1)
	gradData := grads[0].AsFloat32()
	for i := 0; i < 6; i++ {
		if gradData[i] != 1.0 {
			t.Errorf("grad[%d] = %f, expected 1.0", i, gradData[i])
		}
	}
}

// TestChunkOp_BackwardMulti_2D tests chunk backward with 2D tensor.
func TestChunkOp_BackwardMulti_2D(t *testing.T) {
	backend := cpu.New()

	// Input: [[1, 2, 3, 4, 5, 6],
	//         [7, 8, 9, 10, 11, 12]]
	input, err := tensor.NewRaw(tensor.Shape{2, 6}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create input: %v", err)
	}
	inputData := input.AsFloat32()
	for i := range inputData {
		inputData[i] = float32(i + 1)
	}

	// Chunk into 3 parts along dim 1 (columns)
	outputs := backend.Chunk(input, 3, 1)

	// Create ChunkOp
	op := NewChunkOp(input, 3, 1, outputs)

	// Create gradients for all outputs (sequential values)
	gradOutputs := make([]*tensor.RawTensor, 3)
	for i := 0; i < 3; i++ {
		gradOut, err := tensor.NewRaw(tensor.Shape{2, 2}, tensor.Float32, backend.Device())
		if err != nil {
			t.Fatalf("Failed to create gradOutput %d: %v", i, err)
		}
		gradOutData := gradOut.AsFloat32()
		for j := range gradOutData {
			gradOutData[j] = float32(i*4 + j + 1)
		}
		gradOutputs[i] = gradOut
	}

	// BackwardMulti
	grads := op.BackwardMulti(gradOutputs, backend)

	// Check shape
	if !grads[0].Shape().Equal(tensor.Shape{2, 6}) {
		t.Errorf("grad shape = %v, expected [2, 6]", grads[0].Shape())
	}

	// Check values
	// gradOutputs[0] = [[1, 2], [3, 4]]
	// gradOutputs[1] = [[5, 6], [7, 8]]
	// gradOutputs[2] = [[9, 10], [11, 12]]
	// Concatenated: [[1, 2, 5, 6, 9, 10],
	//                [3, 4, 7, 8, 11, 12]]
	gradData := grads[0].AsFloat32()
	expected := []float32{1, 2, 5, 6, 9, 10, 3, 4, 7, 8, 11, 12}
	for i, exp := range expected {
		if gradData[i] != exp {
			t.Errorf("grad[%d] = %f, expected %f", i, gradData[i], exp)
		}
	}
}

// TestChunkOp_BackwardMulti_3D tests chunk backward with 3D tensor.
func TestChunkOp_BackwardMulti_3D(t *testing.T) {
	backend := cpu.New()

	// Input: [2, 3, 6] tensor
	input, err := tensor.NewRaw(tensor.Shape{2, 3, 6}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create input: %v", err)
	}
	inputData := input.AsFloat32()
	for i := range inputData {
		inputData[i] = float32(i)
	}

	// Chunk into 2 parts along dim 2
	outputs := backend.Chunk(input, 2, 2)

	// Create ChunkOp
	op := NewChunkOp(input, 2, 2, outputs)

	// Create gradients for all outputs (all ones)
	gradOutputs := make([]*tensor.RawTensor, 2)
	for i := 0; i < 2; i++ {
		gradOut, err := tensor.NewRaw(tensor.Shape{2, 3, 3}, tensor.Float32, backend.Device())
		if err != nil {
			t.Fatalf("Failed to create gradOutput %d: %v", i, err)
		}
		gradOutData := gradOut.AsFloat32()
		for j := range gradOutData {
			gradOutData[j] = 1.0
		}
		gradOutputs[i] = gradOut
	}

	// BackwardMulti
	grads := op.BackwardMulti(gradOutputs, backend)

	// Check shape
	if !grads[0].Shape().Equal(tensor.Shape{2, 3, 6}) {
		t.Errorf("grad shape = %v, expected [2, 3, 6]", grads[0].Shape())
	}

	// Check all values are 1
	gradData := grads[0].AsFloat32()
	for i, val := range gradData {
		if val != 1.0 {
			t.Errorf("grad[%d] = %f, expected 1.0", i, val)
		}
	}
}

// TestChunkOp_BackwardMulti_NegativeDim tests chunk backward with negative dimension.
func TestChunkOp_BackwardMulti_NegativeDim(t *testing.T) {
	backend := cpu.New()

	// Input: [[1, 2, 3, 4],
	//         [5, 6, 7, 8]]
	input, err := tensor.NewRaw(tensor.Shape{2, 4}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create input: %v", err)
	}
	inputData := input.AsFloat32()
	for i := range inputData {
		inputData[i] = float32(i + 1)
	}

	// Chunk into 2 parts along dim -1 (last dimension)
	outputs := backend.Chunk(input, 2, -1)

	// Create ChunkOp with normalized dim (1)
	op := NewChunkOp(input, 2, 1, outputs)

	// Create gradients for all outputs
	gradOutputs := make([]*tensor.RawTensor, 2)
	for i := 0; i < 2; i++ {
		gradOut, err := tensor.NewRaw(tensor.Shape{2, 2}, tensor.Float32, backend.Device())
		if err != nil {
			t.Fatalf("Failed to create gradOutput %d: %v", i, err)
		}
		gradOutData := gradOut.AsFloat32()
		for j := range gradOutData {
			gradOutData[j] = float32(i*4 + j + 1)
		}
		gradOutputs[i] = gradOut
	}

	// BackwardMulti
	grads := op.BackwardMulti(gradOutputs, backend)

	// Check shape
	if !grads[0].Shape().Equal(tensor.Shape{2, 4}) {
		t.Errorf("grad shape = %v, expected [2, 4]", grads[0].Shape())
	}

	// Check values
	// gradOutputs[0] = [[1, 2], [3, 4]]
	// gradOutputs[1] = [[5, 6], [7, 8]]
	// Concatenated along dim 1: [[1, 2, 5, 6],
	//                             [3, 4, 7, 8]]
	gradData := grads[0].AsFloat32()
	expected := []float32{1, 2, 5, 6, 3, 4, 7, 8}
	for i, exp := range expected {
		if gradData[i] != exp {
			t.Errorf("grad[%d] = %f, expected %f", i, gradData[i], exp)
		}
	}
}

// TestChunkOp_Backward_Panics tests that Backward panics for multi-output ops.
func TestChunkOp_Backward_Panics(t *testing.T) {
	backend := cpu.New()

	// Input
	input, err := tensor.NewRaw(tensor.Shape{4}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create input: %v", err)
	}

	// Chunk into 2 parts
	outputs := backend.Chunk(input, 2, 0)

	// Create ChunkOp
	op := NewChunkOp(input, 2, 0, outputs)

	// Create single gradient output
	gradOutput, err := tensor.NewRaw(tensor.Shape{2}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create gradOutput: %v", err)
	}

	// Calling Backward should panic
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected Backward to panic for multi-output op")
		}
	}()

	op.Backward(gradOutput, backend)
}

// TestChunkOp_BackwardMulti_WrongNumberOfGradients tests error handling.
func TestChunkOp_BackwardMulti_WrongNumberOfGradients(t *testing.T) {
	backend := cpu.New()

	// Input
	input, err := tensor.NewRaw(tensor.Shape{6}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create input: %v", err)
	}

	// Chunk into 3 parts
	outputs := backend.Chunk(input, 3, 0)

	// Create ChunkOp
	op := NewChunkOp(input, 3, 0, outputs)

	// Create only 2 gradients (should be 3)
	gradOutputs := make([]*tensor.RawTensor, 2)
	for i := 0; i < 2; i++ {
		gradOut, err := tensor.NewRaw(tensor.Shape{2}, tensor.Float32, backend.Device())
		if err != nil {
			t.Fatalf("Failed to create gradOutput: %v", err)
		}
		gradOutputs[i] = gradOut
	}

	// Calling BackwardMulti should panic
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected BackwardMulti to panic with wrong number of gradients")
		}
	}()

	op.BackwardMulti(gradOutputs, backend)
}
