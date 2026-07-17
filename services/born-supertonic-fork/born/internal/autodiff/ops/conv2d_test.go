package ops

import (
	"math"
	"testing"

	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/tensor"
)

// TestConv2DOp_BackwardGradients tests Conv2D backward pass gradients.
func TestConv2DOp_BackwardGradients(t *testing.T) {
	backend := cpu.New()

	// Input: [1, 1, 3, 3]
	input, _ := tensor.NewRaw(tensor.Shape{1, 1, 3, 3}, tensor.Float32, tensor.CPU)
	inputData := input.AsFloat32()
	for i := 0; i < 9; i++ {
		inputData[i] = float32(i + 1)
	}

	// Kernel: [1, 1, 2, 2]
	kernel, _ := tensor.NewRaw(tensor.Shape{1, 1, 2, 2}, tensor.Float32, tensor.CPU)
	kernelData := kernel.AsFloat32()
	kernelData[0], kernelData[1] = 1.0, 2.0
	kernelData[2], kernelData[3] = 3.0, 4.0

	// Forward
	output := backend.Conv2D(input, kernel, 1, 0)

	// Create operation
	op := NewConv2DOp(input, kernel, output, 1, 0)

	// Output gradient (all ones)
	outputGrad, _ := tensor.NewRaw(output.Shape(), tensor.Float32, tensor.CPU)
	outputGradData := outputGrad.AsFloat32()
	for i := range outputGradData {
		outputGradData[i] = 1.0
	}

	// Backward
	grads := op.Backward(outputGrad, backend)

	// Check we got 2 gradients (input and kernel)
	if len(grads) != 2 {
		t.Fatalf("Expected 2 gradients, got %d", len(grads))
	}

	inputGrad := grads[0]
	kernelGrad := grads[1]

	// Verify shapes
	if !inputGrad.Shape().Equal(input.Shape()) {
		t.Errorf("inputGrad shape %v != input shape %v", inputGrad.Shape(), input.Shape())
	}
	if !kernelGrad.Shape().Equal(kernel.Shape()) {
		t.Errorf("kernelGrad shape %v != kernel shape %v", kernelGrad.Shape(), kernel.Shape())
	}

	// Verify gradients are non-zero (parameters should update!)
	inputGradData := inputGrad.AsFloat32()
	kernelGradData := kernelGrad.AsFloat32()

	inputNonZero := 0
	for _, g := range inputGradData {
		if g != 0.0 {
			inputNonZero++
		}
	}

	kernelNonZero := 0
	for _, g := range kernelGradData {
		if g != 0.0 {
			kernelNonZero++
		}
	}

	if inputNonZero == 0 {
		t.Error("inputGrad has all zeros - input would not update!")
	}
	if kernelNonZero == 0 {
		t.Error("kernelGrad has all zeros - kernel would not update!")
	}

	t.Logf("SUCCESS: Conv2D gradients flow correctly")
	t.Logf("  inputGrad: %d/%d non-zero", inputNonZero, len(inputGradData))
	t.Logf("  kernelGrad: %d/%d non-zero", kernelNonZero, len(kernelGradData))
}

// TestConv2DOp_NumericalGradient verifies gradients using finite differences.
func TestConv2DOp_NumericalGradient(t *testing.T) {
	backend := cpu.New()
	epsilon := float32(1e-4)
	tolerance := float32(0.05) // 5% tolerance for numerical gradients

	// Small test case for faster numerical gradient computation
	// Input: [1, 1, 3, 3]
	input, _ := tensor.NewRaw(tensor.Shape{1, 1, 3, 3}, tensor.Float32, tensor.CPU)
	inputData := input.AsFloat32()
	for i := 0; i < 9; i++ {
		inputData[i] = float32(i%3 + 1) // Pattern: [1,2,3,1,2,3,1,2,3]
	}

	// Kernel: [1, 1, 2, 2]
	kernel, _ := tensor.NewRaw(tensor.Shape{1, 1, 2, 2}, tensor.Float32, tensor.CPU)
	kernelData := kernel.AsFloat32()
	for i := 0; i < 4; i++ {
		kernelData[i] = float32(i + 1) // [1, 2, 3, 4]
	}

	// Forward
	output := backend.Conv2D(input, kernel, 1, 0)

	// Dummy loss: sum of all outputs
	loss := float32(0.0)
	outputData := output.AsFloat32()
	for _, v := range outputData {
		loss += v
	}

	// Output gradient (all ones for sum loss)
	outputGrad, _ := tensor.NewRaw(output.Shape(), tensor.Float32, tensor.CPU)
	outputGradData := outputGrad.AsFloat32()
	for i := range outputGradData {
		outputGradData[i] = 1.0
	}

	// Analytical gradients
	op := NewConv2DOp(input, kernel, output, 1, 0)
	grads := op.Backward(outputGrad, backend)
	inputGrad := grads[0]
	kernelGrad := grads[1]

	// Numerical gradient for input (check first few elements)
	t.Log("Checking input gradients (numerical vs analytical):")
	for i := 0; i < minInt(4, len(inputData)); i++ {
		// Forward with input[i] + epsilon
		original := inputData[i]
		inputData[i] = original + epsilon
		outputPlus := backend.Conv2D(input, kernel, 1, 0)
		lossPlus := float32(0.0)
		for _, v := range outputPlus.AsFloat32() {
			lossPlus += v
		}

		// Forward with input[i] - epsilon
		inputData[i] = original - epsilon
		outputMinus := backend.Conv2D(input, kernel, 1, 0)
		lossMinus := float32(0.0)
		for _, v := range outputMinus.AsFloat32() {
			lossMinus += v
		}

		// Restore
		inputData[i] = original

		// Numerical gradient
		numericalGrad := (lossPlus - lossMinus) / (2 * epsilon)
		analyticalGrad := inputGrad.AsFloat32()[i]

		diff := math.Abs(float64(numericalGrad - analyticalGrad))
		if diff > float64(tolerance) {
			t.Errorf("input[%d]: numerical=%.6f, analytical=%.6f, diff=%.6f",
				i, numericalGrad, analyticalGrad, diff)
		} else {
			t.Logf("  input[%d]: ✓ numerical=%.6f, analytical=%.6f", i, numericalGrad, analyticalGrad)
		}
	}

	// Numerical gradient for kernel
	t.Log("Checking kernel gradients (numerical vs analytical):")
	for i := 0; i < len(kernelData); i++ {
		original := kernelData[i]
		kernelData[i] = original + epsilon
		outputPlus := backend.Conv2D(input, kernel, 1, 0)
		lossPlus := float32(0.0)
		for _, v := range outputPlus.AsFloat32() {
			lossPlus += v
		}

		kernelData[i] = original - epsilon
		outputMinus := backend.Conv2D(input, kernel, 1, 0)
		lossMinus := float32(0.0)
		for _, v := range outputMinus.AsFloat32() {
			lossMinus += v
		}

		kernelData[i] = original

		numericalGrad := (lossPlus - lossMinus) / (2 * epsilon)
		analyticalGrad := kernelGrad.AsFloat32()[i]

		diff := math.Abs(float64(numericalGrad - analyticalGrad))
		if diff > float64(tolerance) {
			t.Errorf("kernel[%d]: numerical=%.6f, analytical=%.6f, diff=%.6f",
				i, numericalGrad, analyticalGrad, diff)
		} else {
			t.Logf("  kernel[%d]: ✓ numerical=%.6f, analytical=%.6f", i, numericalGrad, analyticalGrad)
		}
	}
}

// TestConv2DOp_WithPaddingAndStride tests gradients with padding and stride.
func TestConv2DOp_WithPaddingAndStride(t *testing.T) {
	backend := cpu.New()

	// Input: [1, 1, 4, 4]
	input, _ := tensor.NewRaw(tensor.Shape{1, 1, 4, 4}, tensor.Float32, tensor.CPU)
	inputData := input.AsFloat32()
	for i := 0; i < 16; i++ {
		inputData[i] = float32(i + 1)
	}

	// Kernel: [1, 1, 3, 3]
	kernel, _ := tensor.NewRaw(tensor.Shape{1, 1, 3, 3}, tensor.Float32, tensor.CPU)
	kernelData := kernel.AsFloat32()
	for i := 0; i < 9; i++ {
		kernelData[i] = 1.0
	}

	// Forward with stride=2, padding=1
	output := backend.Conv2D(input, kernel, 2, 1)

	// Create operation
	op := NewConv2DOp(input, kernel, output, 2, 1)

	// Output gradient
	outputGrad, _ := tensor.NewRaw(output.Shape(), tensor.Float32, tensor.CPU)
	outputGradData := outputGrad.AsFloat32()
	for i := range outputGradData {
		outputGradData[i] = 1.0
	}

	// Backward
	grads := op.Backward(outputGrad, backend)

	// Verify shapes
	if !grads[0].Shape().Equal(input.Shape()) {
		t.Errorf("inputGrad shape mismatch")
	}
	if !grads[1].Shape().Equal(kernel.Shape()) {
		t.Errorf("kernelGrad shape mismatch")
	}

	// Verify non-zero gradients
	inputGradData := grads[0].AsFloat32()
	kernelGradData := grads[1].AsFloat32()

	nonZeroInput := 0
	for _, g := range inputGradData {
		if g != 0.0 {
			nonZeroInput++
		}
	}

	nonZeroKernel := 0
	for _, g := range kernelGradData {
		if g != 0.0 {
			nonZeroKernel++
		}
	}

	if nonZeroInput == 0 {
		t.Error("inputGrad has all zeros with stride/padding!")
	}
	if nonZeroKernel == 0 {
		t.Error("kernelGrad has all zeros with stride/padding!")
	}

	t.Logf("Gradients computed correctly with stride=2, padding=1")
}

// minInt returns minimum of two ints.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
