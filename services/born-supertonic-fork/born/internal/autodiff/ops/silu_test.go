package ops

import (
	"math"
	"testing"

	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/tensor"
)

// TestSiLUOpBackward tests SiLU backward pass (gradient computation).
func TestSiLUOpBackward(t *testing.T) {
	backend := cpu.New()

	// Test case: x = 1.0
	// Forward: y = x * sigmoid(x) = 1.0 * sigmoid(1.0) ≈ 1.0 * 0.7311 ≈ 0.7311
	// Backward: dy/dx = sigmoid(x) * (1 + x * (1 - sigmoid(x)))
	//                 ≈ 0.7311 * (1 + 1.0 * (1 - 0.7311))
	//                 ≈ 0.7311 * 1.2689 ≈ 0.9277

	input, err := tensor.NewRaw(tensor.Shape{1}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create input: %v", err)
	}
	input.AsFloat32()[0] = 1.0

	output, err := tensor.NewRaw(tensor.Shape{1}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create output: %v", err)
	}

	// Compute forward pass manually
	x := input.AsFloat32()[0]
	sigmoid := float32(1.0 / (1.0 + math.Exp(float64(-x))))
	output.AsFloat32()[0] = x * sigmoid

	// Create operation
	op := NewSiLUOp(input, output)

	// Output gradient (dy/dy = 1)
	outputGrad, err := tensor.NewRaw(tensor.Shape{1}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create output gradient: %v", err)
	}
	outputGrad.AsFloat32()[0] = 1.0

	// Backward pass
	inputGrads := op.Backward(outputGrad, backend)

	if len(inputGrads) != 1 {
		t.Fatalf("Expected 1 input gradient, got %d", len(inputGrads))
	}

	// Check gradient value
	expectedGrad := float32(0.9277)
	gotGrad := inputGrads[0].AsFloat32()[0]

	if math.Abs(float64(gotGrad-expectedGrad)) > 0.01 {
		t.Errorf("SiLU gradient = %v, expected ≈ %v", gotGrad, expectedGrad)
	}
}

// TestSiLUOpInputs tests that SiLUOp returns correct inputs.
func TestSiLUOpInputs(t *testing.T) {
	backend := cpu.New()

	input, _ := tensor.NewRaw(tensor.Shape{1}, tensor.Float32, backend.Device())
	output, _ := tensor.NewRaw(tensor.Shape{1}, tensor.Float32, backend.Device())

	op := NewSiLUOp(input, output)

	inputs := op.Inputs()
	if len(inputs) != 1 {
		t.Errorf("Expected 1 input, got %d", len(inputs))
	}

	if inputs[0] != input {
		t.Error("Input tensor mismatch")
	}
}

// TestSiLUOpOutput tests that SiLUOp returns correct output.
func TestSiLUOpOutput(t *testing.T) {
	backend := cpu.New()

	input, _ := tensor.NewRaw(tensor.Shape{1}, tensor.Float32, backend.Device())
	output, _ := tensor.NewRaw(tensor.Shape{1}, tensor.Float32, backend.Device())

	op := NewSiLUOp(input, output)

	if op.Output() != output {
		t.Error("Output tensor mismatch")
	}
}

// TestSiLUOpBackwardZero tests SiLU gradient at x=0.
func TestSiLUOpBackwardZero(t *testing.T) {
	backend := cpu.New()

	// At x=0: y = 0 * sigmoid(0) = 0 * 0.5 = 0
	// dy/dx = sigmoid(0) * (1 + 0 * (1 - sigmoid(0))) = 0.5

	input, _ := tensor.NewRaw(tensor.Shape{1}, tensor.Float32, backend.Device())
	input.AsFloat32()[0] = 0.0

	output, _ := tensor.NewRaw(tensor.Shape{1}, tensor.Float32, backend.Device())
	output.AsFloat32()[0] = 0.0

	op := NewSiLUOp(input, output)

	outputGrad, _ := tensor.NewRaw(tensor.Shape{1}, tensor.Float32, backend.Device())
	outputGrad.AsFloat32()[0] = 1.0

	inputGrads := op.Backward(outputGrad, backend)

	expectedGrad := float32(0.5)
	gotGrad := inputGrads[0].AsFloat32()[0]

	if math.Abs(float64(gotGrad-expectedGrad)) > 0.01 {
		t.Errorf("SiLU gradient at x=0: got %v, expected %v", gotGrad, expectedGrad)
	}
}

// TestSiLUOpBackwardNegative tests SiLU gradient for negative values.
func TestSiLUOpBackwardNegative(t *testing.T) {
	backend := cpu.New()

	// Test x = -1.0
	x := float32(-1.0)
	sigmoid := float32(1.0 / (1.0 + math.Exp(float64(-x))))
	y := x * sigmoid

	input, _ := tensor.NewRaw(tensor.Shape{1}, tensor.Float32, backend.Device())
	input.AsFloat32()[0] = x

	output, _ := tensor.NewRaw(tensor.Shape{1}, tensor.Float32, backend.Device())
	output.AsFloat32()[0] = y

	op := NewSiLUOp(input, output)

	outputGrad, _ := tensor.NewRaw(tensor.Shape{1}, tensor.Float32, backend.Device())
	outputGrad.AsFloat32()[0] = 1.0

	inputGrads := op.Backward(outputGrad, backend)

	// Expected: sigmoid(-1) * (1 + (-1) * (1 - sigmoid(-1)))
	//         ≈ 0.2689 * (1 - (1 - 0.2689))
	//         ≈ 0.2689 * 0.2689 ≈ 0.0723
	expectedGrad := float32(0.0723)
	gotGrad := inputGrads[0].AsFloat32()[0]

	if math.Abs(float64(gotGrad-expectedGrad)) > 0.01 {
		t.Errorf("SiLU gradient at x=-1: got %v, expected ≈ %v", gotGrad, expectedGrad)
	}
}
