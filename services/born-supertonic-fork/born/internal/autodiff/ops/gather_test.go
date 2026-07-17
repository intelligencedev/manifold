package ops

import (
	"math"
	"testing"

	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/tensor"
)

// TestGatherOp_Backward_1D tests basic gather backward with 1D tensor.
func TestGatherOp_Backward_1D(t *testing.T) {
	backend := cpu.New()

	// Input: [10, 20, 30, 40]
	input, err := tensor.NewRaw(tensor.Shape{4}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create input: %v", err)
	}
	inputData := input.AsFloat32()
	inputData[0], inputData[1], inputData[2], inputData[3] = 10, 20, 30, 40

	// Index: [2, 0, 3] (gather indices 2, 0, 3)
	index, err := tensor.NewRaw(tensor.Shape{3}, tensor.Int32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create index: %v", err)
	}
	indexData := index.AsInt32()
	indexData[0], indexData[1], indexData[2] = 2, 0, 3

	// Forward: output = [30, 10, 40]
	output := backend.Gather(input, 0, index)

	// Create GatherOp
	op := NewGatherOp(input, 0, index, output)

	// Create gradient output: [1, 2, 3]
	gradOutput, err := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create gradOutput: %v", err)
	}
	gradOutData := gradOutput.AsFloat32()
	gradOutData[0], gradOutData[1], gradOutData[2] = 1, 2, 3

	// Backward
	grads := op.Backward(gradOutput, backend)

	// Check we got 1 gradient
	if len(grads) != 1 {
		t.Fatalf("Expected 1 gradient, got %d", len(grads))
	}

	// Check shape
	if !grads[0].Shape().Equal(tensor.Shape{4}) {
		t.Errorf("grad shape = %v, expected [4]", grads[0].Shape())
	}

	// Check values
	// gradOutput[0]=1 goes to input[2] -> grad[2]=1
	// gradOutput[1]=2 goes to input[0] -> grad[0]=2
	// gradOutput[2]=3 goes to input[3] -> grad[3]=3
	// input[1] is not gathered -> grad[1]=0
	gradData := grads[0].AsFloat32()
	expected := []float32{2, 0, 1, 3}
	for i, exp := range expected {
		if gradData[i] != exp {
			t.Errorf("grad[%d] = %f, expected %f", i, gradData[i], exp)
		}
	}
}

// TestGatherOp_Backward_2D tests gather backward with 2D tensor along dim 1.
func TestGatherOp_Backward_2D(t *testing.T) {
	backend := cpu.New()

	// Input: [[10, 20, 30],
	//         [40, 50, 60]]
	input, err := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create input: %v", err)
	}
	inputData := input.AsFloat32()
	for i := range inputData {
		inputData[i] = float32((i + 1) * 10)
	}

	// Index: [[2, 0],
	//         [1, 2]] (gather along dim 1)
	index, err := tensor.NewRaw(tensor.Shape{2, 2}, tensor.Int32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create index: %v", err)
	}
	indexData := index.AsInt32()
	indexData[0], indexData[1] = 2, 0 // row 0: gather cols 2, 0
	indexData[2], indexData[3] = 1, 2 // row 1: gather cols 1, 2

	// Forward: output = [[30, 10],
	//                    [50, 60]]
	output := backend.Gather(input, 1, index)

	// Create GatherOp
	op := NewGatherOp(input, 1, index, output)

	// Create gradient output (all ones)
	gradOutput, err := tensor.NewRaw(tensor.Shape{2, 2}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create gradOutput: %v", err)
	}
	gradOutData := gradOutput.AsFloat32()
	for i := range gradOutData {
		gradOutData[i] = 1.0
	}

	// Backward
	grads := op.Backward(gradOutput, backend)

	// Check shape
	if !grads[0].Shape().Equal(tensor.Shape{2, 3}) {
		t.Errorf("grad shape = %v, expected [2, 3]", grads[0].Shape())
	}

	// Check values
	// Row 0: gathered cols [2, 0] -> grads at [0, 2]
	// Row 1: gathered cols [1, 2] -> grads at [1, 2]
	// Expected: [[1, 0, 1],
	//            [0, 1, 1]]
	gradData := grads[0].AsFloat32()
	expected := []float32{1, 0, 1, 0, 1, 1}
	for i, exp := range expected {
		if gradData[i] != exp {
			t.Errorf("grad[%d] = %f, expected %f", i, gradData[i], exp)
		}
	}
}

// TestGatherOp_Backward_DuplicateIndices tests gradient accumulation.
func TestGatherOp_Backward_DuplicateIndices(t *testing.T) {
	backend := cpu.New()

	// Input: [10, 20, 30]
	input, err := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create input: %v", err)
	}

	// Index: [0, 1, 0, 2, 0] (index 0 appears 3 times)
	index, err := tensor.NewRaw(tensor.Shape{5}, tensor.Int32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create index: %v", err)
	}
	indexData := index.AsInt32()
	indexData[0], indexData[1], indexData[2] = 0, 1, 0
	indexData[3], indexData[4] = 2, 0

	// Forward
	output := backend.Gather(input, 0, index)

	// Create GatherOp
	op := NewGatherOp(input, 0, index, output)

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

	// Check values
	// Index 0 appears 3 times -> grad[0] = 3
	// Index 1 appears 1 time -> grad[1] = 1
	// Index 2 appears 1 time -> grad[2] = 1
	gradData := grads[0].AsFloat32()
	expected := []float32{3, 1, 1}
	for i, exp := range expected {
		if gradData[i] != exp {
			t.Errorf("grad[%d] = %f, expected %f", i, gradData[i], exp)
		}
	}
}

// TestGatherOp_Backward_3D tests gather backward with 3D tensor.
func TestGatherOp_Backward_3D(t *testing.T) {
	backend := cpu.New()

	// Input: [2, 3, 4] tensor
	input, err := tensor.NewRaw(tensor.Shape{2, 3, 4}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create input: %v", err)
	}
	inputData := input.AsFloat32()
	for i := range inputData {
		inputData[i] = float32(i)
	}

	// Index: [2, 3, 2] (gather along dim 2)
	index, err := tensor.NewRaw(tensor.Shape{2, 3, 2}, tensor.Int32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create index: %v", err)
	}
	indexData := index.AsInt32()
	for i := range indexData {
		indexData[i] = int32(i % 4) // indices in range [0, 3]
	}

	// Forward
	output := backend.Gather(input, 2, index)

	// Create GatherOp
	op := NewGatherOp(input, 2, index, output)

	// Create gradient output (all ones)
	gradOutput, err := tensor.NewRaw(tensor.Shape{2, 3, 2}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create gradOutput: %v", err)
	}
	gradOutData := gradOutput.AsFloat32()
	for i := range gradOutData {
		gradOutData[i] = 1.0
	}

	// Backward
	grads := op.Backward(gradOutput, backend)

	// Check shape
	if !grads[0].Shape().Equal(tensor.Shape{2, 3, 4}) {
		t.Errorf("grad shape = %v, expected [2, 3, 4]", grads[0].Shape())
	}

	// Values should be accumulated based on index pattern
	gradData := grads[0].AsFloat32()
	// All gradients should be >= 0
	for i, val := range gradData {
		if val < 0 {
			t.Errorf("grad[%d] = %f, expected >= 0", i, val)
		}
	}
}

// TestGatherOp_Backward_NegativeDim tests gather backward with negative dimension.
func TestGatherOp_Backward_NegativeDim(t *testing.T) {
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

	// Index: [[0, 2],
	//         [1, 3]]
	index, err := tensor.NewRaw(tensor.Shape{2, 2}, tensor.Int32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create index: %v", err)
	}
	indexData := index.AsInt32()
	indexData[0], indexData[1] = 0, 2
	indexData[2], indexData[3] = 1, 3

	// Forward with dim=-1 (last dimension)
	output := backend.Gather(input, -1, index)

	// Create GatherOp with normalized dim (1)
	op := NewGatherOp(input, 1, index, output)

	// Create gradient output (all ones)
	gradOutput, err := tensor.NewRaw(tensor.Shape{2, 2}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create gradOutput: %v", err)
	}
	gradOutData := gradOutput.AsFloat32()
	for i := range gradOutData {
		gradOutData[i] = 1.0
	}

	// Backward
	grads := op.Backward(gradOutput, backend)

	// Check shape
	if !grads[0].Shape().Equal(tensor.Shape{2, 4}) {
		t.Errorf("grad shape = %v, expected [2, 4]", grads[0].Shape())
	}

	// Check values
	// Row 0: gathered cols [0, 2] -> grads at [0, 2]
	// Row 1: gathered cols [1, 3] -> grads at [1, 3]
	// Expected: [[1, 0, 1, 0],
	//            [0, 1, 0, 1]]
	gradData := grads[0].AsFloat32()
	expected := []float32{1, 0, 1, 0, 0, 1, 0, 1}
	for i, exp := range expected {
		if math.Abs(float64(gradData[i]-exp)) > 1e-6 {
			t.Errorf("grad[%d] = %f, expected %f", i, gradData[i], exp)
		}
	}
}

// TestGatherOp_Backward_Int64Indices tests gather backward with int64 indices.
func TestGatherOp_Backward_Int64Indices(t *testing.T) {
	backend := cpu.New()

	// Input: [10, 20, 30, 40, 50]
	input, err := tensor.NewRaw(tensor.Shape{5}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create input: %v", err)
	}
	inputData := input.AsFloat32()
	for i := range inputData {
		inputData[i] = float32((i + 1) * 10)
	}

	// Index as int64: [4, 2, 0]
	index, err := tensor.NewRaw(tensor.Shape{3}, tensor.Int64, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create index: %v", err)
	}
	indexData := index.AsInt64()
	indexData[0], indexData[1], indexData[2] = 4, 2, 0

	// Forward using CPU (which accepts int64)
	// We'll manually create the output for this test
	output, err := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create output: %v", err)
	}
	outputData := output.AsFloat32()
	outputData[0], outputData[1], outputData[2] = 50, 30, 10

	// Create GatherOp
	op := NewGatherOp(input, 0, index, output)

	// Create gradient output
	gradOutput, err := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create gradOutput: %v", err)
	}
	gradOutData := gradOutput.AsFloat32()
	gradOutData[0], gradOutData[1], gradOutData[2] = 1, 2, 3

	// Backward - should handle int64 indices
	grads := op.Backward(gradOutput, backend)

	// Check values
	// gradOutput[0]=1 goes to input[4] -> grad[4]=1
	// gradOutput[1]=2 goes to input[2] -> grad[2]=2
	// gradOutput[2]=3 goes to input[0] -> grad[0]=3
	gradData := grads[0].AsFloat32()
	expected := []float32{3, 0, 2, 0, 1}
	for i, exp := range expected {
		if gradData[i] != exp {
			t.Errorf("grad[%d] = %f, expected %f", i, gradData[i], exp)
		}
	}
}

// TestGatherOp_Backward_BoundaryIndices tests gather with max valid indices.
func TestGatherOp_Backward_BoundaryIndices(t *testing.T) {
	backend := cpu.New()

	// Input: [2, 10] tensor
	input, err := tensor.NewRaw(tensor.Shape{2, 10}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create input: %v", err)
	}
	inputData := input.AsFloat32()
	for i := range inputData {
		inputData[i] = float32(i)
	}

	// Index with max valid values (9 for dim size 10)
	index, err := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Int32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create index: %v", err)
	}
	indexData := index.AsInt32()
	indexData[0], indexData[1], indexData[2] = 0, 9, 5 // row 0: boundary test
	indexData[3], indexData[4], indexData[5] = 9, 0, 9 // row 1: multiple 9s

	// Forward
	output := backend.Gather(input, 1, index)

	// Create GatherOp
	op := NewGatherOp(input, 1, index, output)

	// Create gradient output (all ones)
	gradOutput, err := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create gradOutput: %v", err)
	}
	gradOutData := gradOutput.AsFloat32()
	for i := range gradOutData {
		gradOutData[i] = 1.0
	}

	// Backward - should not panic with boundary indices
	grads := op.Backward(gradOutput, backend)

	// Check shape
	if !grads[0].Shape().Equal(tensor.Shape{2, 10}) {
		t.Errorf("grad shape = %v, expected [2, 10]", grads[0].Shape())
	}

	// Check boundary values
	gradData := grads[0].AsFloat32()
	// Row 0: indices [0, 9, 5] -> grads at positions 0, 9, 5
	if gradData[0] != 1 {
		t.Errorf("grad[0] = %f, expected 1", gradData[0])
	}
	if gradData[9] != 1 {
		t.Errorf("grad[9] = %f, expected 1 (boundary)", gradData[9])
	}
	// Row 1: index 9 appears twice -> grad[19] = 2
	if gradData[19] != 2 {
		t.Errorf("grad[19] = %f, expected 2 (accumulated)", gradData[19])
	}
}

// TestGatherOp_Backward_Dim0_2D tests gather along dim 0 for 2D tensor.
func TestGatherOp_Backward_Dim0_2D(t *testing.T) {
	backend := cpu.New()

	// Input: [[1, 2, 3],
	//         [4, 5, 6],
	//         [7, 8, 9]]
	input, err := tensor.NewRaw(tensor.Shape{3, 3}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create input: %v", err)
	}
	inputData := input.AsFloat32()
	for i := range inputData {
		inputData[i] = float32(i + 1)
	}

	// Index: [[2, 0, 1],
	//         [0, 2, 0]] (gather along dim 0)
	index, err := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Int32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create index: %v", err)
	}
	indexData := index.AsInt32()
	indexData[0], indexData[1], indexData[2] = 2, 0, 1
	indexData[3], indexData[4], indexData[5] = 0, 2, 0

	// Forward
	output := backend.Gather(input, 0, index)

	// Create GatherOp
	op := NewGatherOp(input, 0, index, output)

	// Create gradient output (all ones)
	gradOutput, err := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create gradOutput: %v", err)
	}
	gradOutData := gradOutput.AsFloat32()
	for i := range gradOutData {
		gradOutData[i] = 1.0
	}

	// Backward
	grads := op.Backward(gradOutput, backend)

	// Check shape
	if !grads[0].Shape().Equal(tensor.Shape{3, 3}) {
		t.Errorf("grad shape = %v, expected [3, 3]", grads[0].Shape())
	}

	// Verify gradients sum to number of gathered elements (6)
	gradData := grads[0].AsFloat32()
	var sum float32
	for _, v := range gradData {
		sum += v
	}
	if sum != 6 {
		t.Errorf("grad sum = %f, expected 6", sum)
	}
}
