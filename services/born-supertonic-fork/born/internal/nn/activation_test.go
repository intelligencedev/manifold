package nn

import (
	"math"
	"testing"

	"github.com/born-ml/born/internal/autodiff"
	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/tensor"
)

// TestSiLUForward tests SiLU forward pass.
func TestSiLUForward(t *testing.T) {
	backend := autodiff.New(cpu.New())
	silu := NewSiLU[*autodiff.AutodiffBackend[*cpu.CPUBackend]]()

	// Test data: [-2, -1, 0, 1, 2]
	input, err := tensor.FromSlice[float32](
		[]float32{-2.0, -1.0, 0.0, 1.0, 2.0},
		tensor.Shape{5},
		backend,
	)
	if err != nil {
		t.Fatalf("Failed to create input tensor: %v", err)
	}

	// Forward pass
	output := silu.Forward(input)

	// Expected: x * sigmoid(x)
	// For x=-2: -2 * sigmoid(-2) = -2 * 0.1192 ≈ -0.2384
	// For x=-1: -1 * sigmoid(-1) = -1 * 0.2689 ≈ -0.2689
	// For x=0:   0 * sigmoid(0)  = 0 * 0.5    = 0
	// For x=1:   1 * sigmoid(1)  = 1 * 0.7311 ≈ 0.7311
	// For x=2:   2 * sigmoid(2)  = 2 * 0.8808 ≈ 1.7616

	expected := []float32{-0.2384, -0.2689, 0.0, 0.7311, 1.7616}
	outputData := output.Data()

	for i, exp := range expected {
		got := outputData[i]
		if math.Abs(float64(got-exp)) > 0.001 {
			t.Errorf("SiLU(%v) = %v, expected %v", input.Data()[i], got, exp)
		}
	}
}

// TestSiLUShape tests that SiLU preserves input shape.
func TestSiLUShape(t *testing.T) {
	backend := autodiff.New(cpu.New())
	silu := NewSiLU[*autodiff.AutodiffBackend[*cpu.CPUBackend]]()

	// Test 2D tensor
	input := tensor.Randn[float32](tensor.Shape{3, 4}, backend)
	output := silu.Forward(input)

	if len(output.Shape()) != 2 || output.Shape()[0] != 3 || output.Shape()[1] != 4 {
		t.Errorf("SiLU changed shape: input %v -> output %v", input.Shape(), output.Shape())
	}
}

// TestSiLUGradient tests SiLU backward pass using gradient checking.
func TestSiLUGradient(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// Test point: x = 1.0
	x, err := tensor.FromSlice[float32]([]float32{1.0}, tensor.Shape{1}, backend)
	if err != nil {
		t.Fatalf("Failed to create input: %v", err)
	}

	// Mark as requiring gradient
	backend.Tape().StartRecording()

	// Forward: y = SiLU(x)
	siluBackend, ok := any(backend).(SiLUBackend)
	if !ok {
		t.Fatal("Backend doesn't support SiLU")
	}
	_ = siluBackend.SiLU(x.Raw())

	// Create output gradient (dy/dy = 1)
	outputGrad := tensor.Ones[float32](tensor.Shape{1}, backend)

	// Backward pass
	grads := backend.Tape().Backward(outputGrad.Raw(), backend)

	// Get gradient for input
	xGrad, exists := grads[x.Raw()]
	if !exists {
		t.Fatal("No gradient computed for input")
	}

	// Numerical gradient check
	// For SiLU: dy/dx = sigmoid(x) * (1 + x * (1 - sigmoid(x)))
	// At x=1: sigmoid(1) ≈ 0.7311
	// dy/dx ≈ 0.7311 * (1 + 1 * (1 - 0.7311))
	//       ≈ 0.7311 * (1 + 0.2689)
	//       ≈ 0.7311 * 1.2689 ≈ 0.9277

	expectedGrad := float32(0.9277)
	gotGrad := xGrad.AsFloat32()[0]

	if math.Abs(float64(gotGrad-expectedGrad)) > 0.01 {
		t.Errorf("SiLU gradient = %v, expected %v", gotGrad, expectedGrad)
	}
}

// TestSiLUZero tests SiLU at x=0 (special case).
func TestSiLUZero(t *testing.T) {
	backend := autodiff.New(cpu.New())
	silu := NewSiLU[*autodiff.AutodiffBackend[*cpu.CPUBackend]]()

	input, err := tensor.FromSlice[float32]([]float32{0.0}, tensor.Shape{1}, backend)
	if err != nil {
		t.Fatalf("Failed to create input: %v", err)
	}

	output := silu.Forward(input)

	// SiLU(0) = 0 * sigmoid(0) = 0 * 0.5 = 0
	if output.Data()[0] != 0.0 {
		t.Errorf("SiLU(0) = %v, expected 0.0", output.Data()[0])
	}
}

// TestSiLUPositive tests SiLU on positive values.
func TestSiLUPositive(t *testing.T) {
	backend := autodiff.New(cpu.New())
	silu := NewSiLU[*autodiff.AutodiffBackend[*cpu.CPUBackend]]()

	input, err := tensor.FromSlice[float32]([]float32{5.0}, tensor.Shape{1}, backend)
	if err != nil {
		t.Fatalf("Failed to create input: %v", err)
	}

	output := silu.Forward(input)

	// For large positive x, sigmoid(x) ≈ 1, so SiLU(x) ≈ x
	// SiLU(5) ≈ 5 * 0.9933 ≈ 4.966
	expected := float32(4.966)
	got := output.Data()[0]

	if math.Abs(float64(got-expected)) > 0.01 {
		t.Errorf("SiLU(5.0) = %v, expected ≈ %v", got, expected)
	}
}

// TestSiLUNegative tests SiLU on negative values.
func TestSiLUNegative(t *testing.T) {
	backend := autodiff.New(cpu.New())
	silu := NewSiLU[*autodiff.AutodiffBackend[*cpu.CPUBackend]]()

	input, err := tensor.FromSlice[float32]([]float32{-5.0}, tensor.Shape{1}, backend)
	if err != nil {
		t.Fatalf("Failed to create input: %v", err)
	}

	output := silu.Forward(input)

	// For large negative x, sigmoid(x) ≈ 0, so SiLU(x) ≈ 0
	// SiLU(-5) ≈ -5 * 0.0067 ≈ -0.0335
	expected := float32(-0.0335)
	got := output.Data()[0]

	if math.Abs(float64(got-expected)) > 0.01 {
		t.Errorf("SiLU(-5.0) = %v, expected ≈ %v", got, expected)
	}
}
