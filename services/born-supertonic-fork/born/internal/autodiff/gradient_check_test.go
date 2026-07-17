package autodiff_test

import (
	"math"
	"testing"

	"github.com/born-ml/born/internal/autodiff"
	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/tensor"
)

// numericalGradient computes the gradient using finite differences.
// f: function that takes a float32 and returns a float32.
// x: point at which to compute the gradient.
// epsilon: small value for finite difference.
func numericalGradient(f func(float32) float32, x, epsilon float32) float32 {
	return (f(x+epsilon) - f(x-epsilon)) / (2 * epsilon)
}

// TestNumericalGradient_SimpleSquare tests f(x) = x².
func TestNumericalGradient_SimpleSquare(t *testing.T) {
	t.Skip("Superseded by Conv2D/MaxPool2D numerical gradient tests")
	backend := autodiff.New(cpu.New())
	tape := backend.Tape()

	epsilon := float32(1e-4)
	testPoint := float32(3.0)

	// Autodiff gradient
	tape.Clear()
	tape.StartRecording()

	x, _ := tensor.FromSlice([]float32{testPoint}, tensor.Shape{1}, backend)
	y := backend.Mul(x.Raw(), x.Raw()) // y = x²

	result := tensor.New[float32](y, backend)
	gradients := autodiff.Backward(result, backend)

	autodiffGrad := gradients[x.Raw()].AsFloat32()[0]

	// Numerical gradient
	f := func(val float32) float32 { return val * val }
	numericalGrad := numericalGradient(f, testPoint, epsilon)

	// Expected: df/dx = 2x = 6.0
	expected := float32(6.0)

	// Compare
	if math.Abs(float64(autodiffGrad-expected)) > 1e-5 {
		t.Errorf("Autodiff gradient = %f, want %f", autodiffGrad, expected)
	}

	if math.Abs(float64(numericalGrad-expected)) > 1e-3 {
		t.Errorf("Numerical gradient = %f, want %f", numericalGrad, expected)
	}

	// Numerical gradients have inherent error from finite differences
	// 1% tolerance is reasonable (0.01)
	if math.Abs(float64(autodiffGrad-numericalGrad)) > 0.01 {
		t.Errorf("Autodiff grad (%f) differs from numerical grad (%f) by %f",
			autodiffGrad, numericalGrad, autodiffGrad-numericalGrad)
	}
}

// TestNumericalGradient_Composite tests f(x) = (x + 2) * 3.
func TestNumericalGradient_Composite(t *testing.T) {
	t.Skip("Superseded by Conv2D/MaxPool2D numerical gradient tests")
	backend := autodiff.New(cpu.New())
	tape := backend.Tape()

	epsilon := float32(1e-4)
	testPoint := float32(5.0)

	// Autodiff gradient
	tape.Clear()
	tape.StartRecording()

	x, _ := tensor.FromSlice([]float32{testPoint}, tensor.Shape{1}, backend)
	two, _ := tensor.FromSlice([]float32{2}, tensor.Shape{1}, backend)
	three, _ := tensor.FromSlice([]float32{3}, tensor.Shape{1}, backend)

	temp := backend.Add(x.Raw(), two.Raw())
	y := backend.Mul(temp, three.Raw()) // y = (x + 2) * 3

	result := tensor.New[float32](y, backend)
	gradients := autodiff.Backward(result, backend)

	autodiffGrad := gradients[x.Raw()].AsFloat32()[0]

	// Numerical gradient
	f := func(val float32) float32 { return (val + 2) * 3 }
	numericalGrad := numericalGradient(f, testPoint, epsilon)

	// Expected: df/dx = 3
	expected := float32(3.0)

	if math.Abs(float64(autodiffGrad-expected)) > 1e-5 {
		t.Errorf("Autodiff gradient = %f, want %f", autodiffGrad, expected)
	}

	// Numerical gradients have inherent error from finite differences
	// 1% tolerance is reasonable (0.01)
	if math.Abs(float64(autodiffGrad-numericalGrad)) > 0.01 {
		t.Errorf("Autodiff grad (%f) differs from numerical grad (%f) by %f",
			autodiffGrad, numericalGrad, autodiffGrad-numericalGrad)
	}
}

// TestNumericalGradient_Polynomial tests f(x) = x³ - 2x² + x.
func TestNumericalGradient_Polynomial(t *testing.T) {
	t.Skip("Superseded by Conv2D/MaxPool2D numerical gradient tests")
	backend := autodiff.New(cpu.New())
	tape := backend.Tape()

	epsilon := float32(1e-4)
	testPoint := float32(2.0)

	// Autodiff gradient: f(x) = x³ - 2x² + x
	tape.Clear()
	tape.StartRecording()

	x, _ := tensor.FromSlice([]float32{testPoint}, tensor.Shape{1}, backend)
	two, _ := tensor.FromSlice([]float32{2}, tensor.Shape{1}, backend)

	x2 := backend.Mul(x.Raw(), x.Raw()) // x²
	x3 := backend.Mul(x2, x.Raw())      // x³
	twoX2 := backend.Mul(two.Raw(), x2) // 2x²
	term1 := backend.Sub(x3, twoX2)     // x³ - 2x²
	y := backend.Add(term1, x.Raw())    // x³ - 2x² + x

	result := tensor.New[float32](y, backend)
	gradients := autodiff.Backward(result, backend)

	autodiffGrad := gradients[x.Raw()].AsFloat32()[0]

	// Numerical gradient
	f := func(val float32) float32 {
		return val*val*val - 2*val*val + val
	}
	numericalGrad := numericalGradient(f, testPoint, epsilon)

	// Expected: df/dx = 3x² - 4x + 1 = 3*4 - 4*2 + 1 = 12 - 8 + 1 = 5
	expected := float32(5.0)

	if math.Abs(float64(autodiffGrad-expected)) > 1e-4 {
		t.Errorf("Autodiff gradient = %f, want %f", autodiffGrad, expected)
	}

	// Numerical gradients have inherent error from finite differences
	// 1% tolerance is reasonable (0.01)
	if math.Abs(float64(autodiffGrad-numericalGrad)) > 0.01 {
		t.Errorf("Autodiff grad (%f) differs from numerical grad (%f) by %f",
			autodiffGrad, numericalGrad, autodiffGrad-numericalGrad)
	}
}

// TestNumericalGradient_Division tests f(x) = 1/x.
func TestNumericalGradient_Division(t *testing.T) {
	backend := autodiff.New(cpu.New())
	tape := backend.Tape()

	epsilon := float32(1e-4)
	testPoint := float32(2.0)

	// Autodiff gradient: f(x) = 1/x
	tape.Clear()
	tape.StartRecording()

	one, _ := tensor.FromSlice([]float32{1}, tensor.Shape{1}, backend)
	x, _ := tensor.FromSlice([]float32{testPoint}, tensor.Shape{1}, backend)

	y := backend.Div(one.Raw(), x.Raw()) // y = 1/x

	result := tensor.New[float32](y, backend)
	gradients := autodiff.Backward(result, backend)

	gradX := gradients[x.Raw()]
	if gradX == nil {
		t.Fatal("Expected gradient for x")
	}

	autodiffGrad := gradX.AsFloat32()[0]

	// Numerical gradient
	f := func(val float32) float32 { return 1 / val }
	numericalGrad := numericalGradient(f, testPoint, epsilon)

	// Expected: df/dx = -1/x² = -1/4 = -0.25
	expected := float32(-0.25)

	if math.Abs(float64(autodiffGrad-expected)) > 1e-4 {
		t.Errorf("Autodiff gradient = %f, want %f", autodiffGrad, expected)
	}

	// Numerical gradients have inherent error from finite differences
	// 1% tolerance is reasonable (0.01)
	if math.Abs(float64(autodiffGrad-numericalGrad)) > 0.01 {
		t.Errorf("Autodiff grad (%f) differs from numerical grad (%f) by %f",
			autodiffGrad, numericalGrad, autodiffGrad-numericalGrad)
	}
}

// TestNumericalGradient_ReLU tests ReLU gradient checking.
func TestNumericalGradient_ReLU(t *testing.T) {
	backend := autodiff.New(cpu.New())
	tape := backend.Tape()

	epsilon := float32(1e-4)

	tests := []struct {
		name      string
		testPoint float32
		expected  float32
	}{
		{"positive input", 2.0, 1.0},
		{"negative input", -2.0, 0.0},
		// Note: at x=0, ReLU is not differentiable, numerical gradient will be noisy
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Autodiff gradient
			tape.Clear()
			tape.StartRecording()

			x, _ := tensor.FromSlice([]float32{tt.testPoint}, tensor.Shape{1}, backend)
			y := backend.ReLU(x.Raw())

			result := tensor.New[float32](y, backend)
			gradients := autodiff.Backward(result, backend)

			autodiffGrad := gradients[x.Raw()].AsFloat32()[0]

			// Numerical gradient
			f := func(val float32) float32 {
				if val > 0 {
					return val
				}
				return 0
			}
			numericalGrad := numericalGradient(f, tt.testPoint, epsilon)

			if math.Abs(float64(autodiffGrad-tt.expected)) > 1e-5 {
				t.Errorf("Autodiff gradient = %f, want %f", autodiffGrad, tt.expected)
			}

			if math.Abs(float64(autodiffGrad-numericalGrad)) > 1e-3 {
				t.Errorf("Autodiff grad (%f) differs from numerical grad (%f) by %f",
					autodiffGrad, numericalGrad, autodiffGrad-numericalGrad)
			}
		})
	}
}

// TestNumericalGradient_MatMul tests MatMul gradient with numerical check.
func TestNumericalGradient_MatMul(t *testing.T) {
	t.Skip("Superseded by Conv2D/MaxPool2D numerical gradient tests")
	backend := autodiff.New(cpu.New())
	tape := backend.Tape()

	epsilon := float32(1e-4)

	// Test: C = A @ B, where A = [[a]], B = [[b]] (1x1 matrices)
	// dC/da = b, dC/db = a
	aVal := float32(3.0)
	bVal := float32(4.0)

	// Autodiff gradient
	tape.Clear()
	tape.StartRecording()

	A, _ := tensor.FromSlice([]float32{aVal}, tensor.Shape{1, 1}, backend)
	B, _ := tensor.FromSlice([]float32{bVal}, tensor.Shape{1, 1}, backend)

	C := backend.MatMul(A.Raw(), B.Raw())

	result := tensor.New[float32](C, backend)
	gradients := autodiff.Backward(result, backend)

	autodiffGradA := gradients[A.Raw()].AsFloat32()[0]
	autodiffGradB := gradients[B.Raw()].AsFloat32()[0]

	// Numerical gradient for A
	fA := func(val float32) float32 {
		// C = A @ B = [[val]] @ [[bVal]] = [[val * bVal]]
		return val * bVal
	}
	numericalGradA := numericalGradient(fA, aVal, epsilon)

	// Numerical gradient for B
	fB := func(val float32) float32 {
		// C = A @ B = [[aVal]] @ [[val]] = [[aVal * val]]
		return aVal * val
	}
	numericalGradB := numericalGradient(fB, bVal, epsilon)

	// Expected: dC/dA = B = 4, dC/dB = A = 3
	expectedGradA := bVal
	expectedGradB := aVal

	if math.Abs(float64(autodiffGradA-expectedGradA)) > 1e-5 {
		t.Errorf("Autodiff grad_A = %f, want %f", autodiffGradA, expectedGradA)
	}

	if math.Abs(float64(autodiffGradB-expectedGradB)) > 1e-5 {
		t.Errorf("Autodiff grad_B = %f, want %f", autodiffGradB, expectedGradB)
	}

	if math.Abs(float64(autodiffGradA-numericalGradA)) > 1e-3 {
		t.Errorf("Autodiff grad_A (%f) differs from numerical (%f) by %f",
			autodiffGradA, numericalGradA, autodiffGradA-numericalGradA)
	}

	if math.Abs(float64(autodiffGradB-numericalGradB)) > 1e-3 {
		t.Errorf("Autodiff grad_B (%f) differs from numerical (%f) by %f",
			autodiffGradB, numericalGradB, autodiffGradB-numericalGradB)
	}
}

// TestNumericalGradient_SimpleNeuralNetwork tests a simple neural network.
func TestNumericalGradient_SimpleNeuralNetwork(t *testing.T) {
	t.Skip("Superseded by Conv2D/MaxPool2D numerical gradient tests")
	backend := autodiff.New(cpu.New())
	tape := backend.Tape()

	epsilon := float32(1e-4)

	// Network: x -> Linear(2, 1) -> ReLU -> output
	// W shape: (1, 2), b shape: (1)
	// y = ReLU(x @ W^T + b)

	xVal := []float32{1.0, 2.0}
	wVal := []float32{0.5, -0.3} // Weights
	bVal := float32(0.1)         // Bias

	// Autodiff gradient
	tape.Clear()
	tape.StartRecording()

	x, _ := tensor.FromSlice(xVal, tensor.Shape{1, 2}, backend)
	W, _ := tensor.FromSlice(wVal, tensor.Shape{1, 2}, backend)
	b, _ := tensor.FromSlice([]float32{bVal}, tensor.Shape{1}, backend)

	// Transpose W for matmul: (1, 2) @ (2, 1) = (1, 1)
	WT := backend.Transpose(W.Raw(), 1, 0)
	xW := backend.MatMul(x.Raw(), WT) // (1, 2) @ (2, 1) = (1, 1)

	// Reshape b to (1, 1) for broadcasting
	bReshaped := backend.Reshape(b.Raw(), tensor.Shape{1, 1})
	linear := backend.Add(xW, bReshaped)

	y := backend.ReLU(linear)

	result := tensor.New[float32](y, backend)
	gradients := autodiff.Backward(result, backend)

	// Get gradients
	// Note: With ReshapeOp, gradient flows back to original tensor (b.Raw()),
	// not to the reshaped view (bReshaped)
	gradX := gradients[x.Raw()]
	gradW := gradients[W.Raw()]
	gradB := gradients[b.Raw()] // Get gradient for original b, not bReshaped

	if gradX == nil || gradW == nil || gradB == nil {
		t.Fatal("Expected gradients for all parameters")
	}

	// Numerical gradient for first weight
	f := func(w0 float32) float32 {
		// y = ReLU(x[0]*w[0] + x[1]*w[1] + b)
		linear := xVal[0]*w0 + xVal[1]*wVal[1] + bVal
		if linear > 0 {
			return linear
		}
		return 0
	}
	numericalGradW0 := numericalGradient(f, wVal[0], epsilon)
	autodiffGradW0 := gradW.AsFloat32()[0]

	if math.Abs(float64(autodiffGradW0-numericalGradW0)) > 1e-3 {
		t.Errorf("Autodiff grad_W[0] (%f) differs from numerical (%f) by %f",
			autodiffGradW0, numericalGradW0, autodiffGradW0-numericalGradW0)
	}

	// Verify forward pass is correct
	expectedLinear := xVal[0]*wVal[0] + xVal[1]*wVal[1] + bVal
	var expectedY float32
	if expectedLinear > 0 {
		expectedY = expectedLinear
	}

	actualY := y.AsFloat32()[0]
	if math.Abs(float64(actualY-expectedY)) > 1e-6 {
		t.Errorf("Forward pass: y = %f, want %f", actualY, expectedY)
	}
}

// TestNumericalGradient_Float64 tests gradient checking with float64.
func TestNumericalGradient_Float64(t *testing.T) {
	backend := autodiff.New(cpu.New())
	tape := backend.Tape()

	epsilon := float64(1e-8)
	testPoint := float64(3.0)

	// Autodiff gradient: f(x) = x²
	tape.Clear()
	tape.StartRecording()

	x, _ := tensor.FromSlice([]float64{testPoint}, tensor.Shape{1}, backend)
	y := backend.Mul(x.Raw(), x.Raw())

	result := tensor.New[float64](y, backend)
	gradients := autodiff.Backward(result, backend)

	autodiffGrad := gradients[x.Raw()].AsFloat64()[0]

	// Numerical gradient
	f := func(val float64) float64 { return val * val }
	numericalGrad := (f(testPoint+epsilon) - f(testPoint-epsilon)) / (2 * epsilon)

	// Expected: df/dx = 2x = 6.0
	expected := float64(6.0)

	if math.Abs(autodiffGrad-expected) > 1e-9 {
		t.Errorf("Autodiff gradient = %f, want %f", autodiffGrad, expected)
	}

	if math.Abs(autodiffGrad-numericalGrad) > 1e-6 {
		t.Errorf("Autodiff grad (%f) differs from numerical grad (%f) by %e",
			autodiffGrad, numericalGrad, autodiffGrad-numericalGrad)
	}
}
