package ops

import (
	"testing"

	"github.com/born-ml/born/internal/tensor"
)

// TestTransposeOp_BackwardPropagatesGradients tests that TransposeOp correctly
// propagates gradients back through transpose operations.
//
// This is a CRITICAL test that catches the bug where Transpose was not being
// recorded on the gradient tape, causing parameters to not update during training.
func TestTransposeOp_BackwardPropagatesGradients(t *testing.T) {
	backend := tensor.NewMockBackend()

	// Create input: [2, 3] matrix
	input, _ := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Float32, tensor.CPU)
	inputData := input.AsFloat32()
	for i := range inputData {
		inputData[i] = float32(i + 1)
	}

	// Forward transpose: [2, 3] -> [3, 2]
	output := backend.Transpose(input, 1, 0)

	// Create TransposeOp
	op := NewTransposeOp(input, output, []int{1, 0})

	// Create output gradient: [3, 2]
	outputGrad, _ := tensor.NewRaw(tensor.Shape{3, 2}, tensor.Float32, tensor.CPU)
	outputGradData := outputGrad.AsFloat32()
	for i := range outputGradData {
		outputGradData[i] = float32(i + 10)
	}

	// Backward pass
	inputGrads := op.Backward(outputGrad, backend)

	if len(inputGrads) != 1 {
		t.Fatalf("Expected 1 input gradient, got %d", len(inputGrads))
	}

	inputGrad := inputGrads[0]

	// Check shape: should be [2, 3] (same as input)
	if !inputGrad.Shape().Equal(tensor.Shape{2, 3}) {
		t.Fatalf("Expected inputGrad shape [2 3], got %v", inputGrad.Shape())
	}

	// Verify gradient values are correctly transposed back
	// outputGrad: [[10, 11], [12, 13], [14, 15]] (shape [3, 2])
	// inputGrad:  [[10, 12, 14], [11, 13, 15]] (shape [2, 3])
	expected := []float32{10, 12, 14, 11, 13, 15}
	gradData := inputGrad.AsFloat32()

	for i, exp := range expected {
		if gradData[i] != exp {
			t.Errorf("inputGrad[%d]: expected %.1f, got %.1f", i, exp, gradData[i])
		}
	}
}

// TestTransposeOp_WithMatMulIntegration tests the integration of TransposeOp
// with MatMulOp, which is the real-world use case in Linear layers.
//
// This test ensures that gradients flow correctly through the pattern:
//
//	weight -> Transpose -> MatMul
//
// which is used in every Linear layer during training.
func TestTransposeOp_WithMatMulIntegration(t *testing.T) {
	backend := tensor.NewMockBackend()

	// Simulate Linear layer forward pass:
	// weight [2, 3], input [1, 3] -> weight.T @ input.T = output [2, 1]

	// Weight matrix [2, 3]
	weight, _ := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Float32, tensor.CPU)
	weightData := weight.AsFloat32()
	for i := range weightData {
		weightData[i] = float32(i + 1)
	}

	// Input [1, 3]
	input, _ := tensor.NewRaw(tensor.Shape{1, 3}, tensor.Float32, tensor.CPU)
	inputData := input.AsFloat32()
	inputData[0], inputData[1], inputData[2] = 1.0, 2.0, 3.0

	// Forward: weight.T [3, 2] @ input.T [3, 1] = output [2, 1]
	// But we do: input [1, 3] @ weight.T [3, 2] = output [1, 2]
	weightT := backend.Transpose(weight, 1, 0) // [3, 2]
	output := backend.MatMul(input, weightT)   // [1, 3] @ [3, 2] = [1, 2]

	// Create ops
	transposeOp := NewTransposeOp(weight, weightT, []int{1, 0})
	matmulOp := NewMatMulOp(input, weightT, output)

	// Backward from output [1, 2]
	outputGrad, _ := tensor.NewRaw(tensor.Shape{1, 2}, tensor.Float32, tensor.CPU)
	outputGradData := outputGrad.AsFloat32()
	outputGradData[0], outputGradData[1] = 1.0, 1.0

	// MatMul backward
	matmulGrads := matmulOp.Backward(outputGrad, backend)
	inputGrad := matmulGrads[0]   // Gradient for input
	weightTGrad := matmulGrads[1] // Gradient for weightT

	// Transpose backward
	transposeGrads := transposeOp.Backward(weightTGrad, backend)
	weightGrad := transposeGrads[0] // Gradient for original weight

	// Verify weightGrad has correct shape
	if !weightGrad.Shape().Equal(tensor.Shape{2, 3}) {
		t.Fatalf("Expected weightGrad shape [2 3], got %v", weightGrad.Shape())
	}

	// Verify weightGrad is non-zero (parameters should update!)
	weightGradData := weightGrad.AsFloat32()
	nonZero := 0
	for _, g := range weightGradData {
		if g != 0 {
			nonZero++
		}
	}

	if nonZero == 0 {
		t.Error("weightGrad has all zeros - parameters would not update!")
	}

	// Verify inputGrad is computed (should have same shape as input)
	if len(inputGrad.Shape()) == 0 {
		t.Error("inputGrad has empty shape")
	}

	t.Logf("SUCCESS: Gradients flow correctly through Transpose->MatMul pattern")
	t.Logf("  weightGrad has %d/%d non-zero elements", nonZero, len(weightGradData))
}
