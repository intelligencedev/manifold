package ops

import (
	"math"
	"testing"

	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/tensor"
)

func TestSumDimOp_Forward(t *testing.T) {
	backend := cpu.New()

	// Create input [2, 3, 4]
	x, _ := tensor.NewRaw(tensor.Shape{2, 3, 4}, tensor.Float32, backend.Device())
	xData := x.AsFloat32()
	for i := range xData {
		xData[i] = float32(i + 1)
	}

	// Forward: sum along last dim
	output := backend.SumDim(x, -1, true)

	// Create op
	op := NewSumDimOp(x, output, -1, true)

	// Check output shape
	if !op.Output().Shape().Equal(tensor.Shape{2, 3, 1}) {
		t.Errorf("Expected output shape [2, 3, 1], got %v", op.Output().Shape())
	}
}

func TestSumDimOp_Backward_KeepDim(t *testing.T) {
	backend := cpu.New()

	// Input [2, 3]
	x, _ := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Float32, backend.Device())
	xData := x.AsFloat32()
	for i := range xData {
		xData[i] = float32(i + 1)
	}

	// Forward: sum along dim 1 with keepDim=true -> [2, 1]
	output := backend.SumDim(x, 1, true)

	// Create op
	op := NewSumDimOp(x, output, 1, true)

	// Backward with gradient of ones
	outputGrad, _ := tensor.NewRaw(output.Shape(), tensor.Float32, backend.Device())
	outputGradData := outputGrad.AsFloat32()
	for i := range outputGradData {
		outputGradData[i] = 1.0
	}

	grads := op.Backward(outputGrad, backend)

	// Check gradient shape matches input
	if !grads[0].Shape().Equal(x.Shape()) {
		t.Errorf("Expected grad shape %v, got %v", x.Shape(), grads[0].Shape())
	}

	// For sum, gradient should be all ones (broadcast back)
	gradData := grads[0].AsFloat32()
	for i, g := range gradData {
		if g != 1.0 {
			t.Errorf("Expected gradient 1.0 at index %d, got %v", i, g)
		}
	}
}

func TestSumDimOp_Backward_NoKeepDim(t *testing.T) {
	backend := cpu.New()

	// Input [2, 3]
	x, _ := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Float32, backend.Device())
	xData := x.AsFloat32()
	for i := range xData {
		xData[i] = float32(i + 1)
	}

	// Forward: sum along dim 1 with keepDim=false -> [2]
	output := backend.SumDim(x, 1, false)

	// Create op
	op := NewSumDimOp(x, output, 1, false)

	// Backward with gradient of ones
	outputGrad, _ := tensor.NewRaw(output.Shape(), tensor.Float32, backend.Device())
	outputGradData := outputGrad.AsFloat32()
	for i := range outputGradData {
		outputGradData[i] = 1.0
	}

	grads := op.Backward(outputGrad, backend)

	// Check gradient shape matches input
	if !grads[0].Shape().Equal(x.Shape()) {
		t.Errorf("Expected grad shape %v, got %v", x.Shape(), grads[0].Shape())
	}

	// For sum, gradient should be all ones (broadcast back)
	gradData := grads[0].AsFloat32()
	for i, g := range gradData {
		if g != 1.0 {
			t.Errorf("Expected gradient 1.0 at index %d, got %v", i, g)
		}
	}
}

func TestMeanDimOp_Backward_KeepDim(t *testing.T) {
	backend := cpu.New()

	// Input [2, 4]
	x, _ := tensor.NewRaw(tensor.Shape{2, 4}, tensor.Float32, backend.Device())
	xData := x.AsFloat32()
	for i := range xData {
		xData[i] = float32(i + 1)
	}

	// Forward: mean along dim 1 with keepDim=true -> [2, 1]
	output := backend.MeanDim(x, 1, true)

	// Create op
	op := NewMeanDimOp(x, output, 1, true)

	// Backward with gradient of ones
	outputGrad, _ := tensor.NewRaw(output.Shape(), tensor.Float32, backend.Device())
	outputGradData := outputGrad.AsFloat32()
	for i := range outputGradData {
		outputGradData[i] = 1.0
	}

	grads := op.Backward(outputGrad, backend)

	// Check gradient shape matches input
	if !grads[0].Shape().Equal(x.Shape()) {
		t.Errorf("Expected grad shape %v, got %v", x.Shape(), grads[0].Shape())
	}

	// For mean, gradient should be 1/dimSize (broadcast back)
	// dimSize = 4, so each gradient should be 0.25
	gradData := grads[0].AsFloat32()
	expected := float32(0.25)
	for i, g := range gradData {
		if math.Abs(float64(g-expected)) > 1e-6 {
			t.Errorf("Expected gradient %v at index %d, got %v", expected, i, g)
		}
	}
}

func TestMeanDimOp_Backward_NoKeepDim(t *testing.T) {
	backend := cpu.New()

	// Input [2, 4]
	x, _ := tensor.NewRaw(tensor.Shape{2, 4}, tensor.Float32, backend.Device())
	xData := x.AsFloat32()
	for i := range xData {
		xData[i] = float32(i + 1)
	}

	// Forward: mean along dim 1 with keepDim=false -> [2]
	output := backend.MeanDim(x, 1, false)

	// Create op
	op := NewMeanDimOp(x, output, 1, false)

	// Backward with gradient of ones
	outputGrad, _ := tensor.NewRaw(output.Shape(), tensor.Float32, backend.Device())
	outputGradData := outputGrad.AsFloat32()
	for i := range outputGradData {
		outputGradData[i] = 1.0
	}

	grads := op.Backward(outputGrad, backend)

	// Check gradient shape matches input
	if !grads[0].Shape().Equal(x.Shape()) {
		t.Errorf("Expected grad shape %v, got %v", x.Shape(), grads[0].Shape())
	}

	// For mean, gradient should be 1/dimSize
	gradData := grads[0].AsFloat32()
	expected := float32(0.25)
	for i, g := range gradData {
		if math.Abs(float64(g-expected)) > 1e-6 {
			t.Errorf("Expected gradient %v at index %d, got %v", expected, i, g)
		}
	}
}

func TestSumDimOp_GradientCheck(t *testing.T) {
	backend := cpu.New()

	// Input [3, 4]
	x, _ := tensor.NewRaw(tensor.Shape{3, 4}, tensor.Float32, backend.Device())
	xData := x.AsFloat32()
	for i := range xData {
		xData[i] = float32(i+1) * 0.1
	}

	// Test both keepDim=true and keepDim=false
	for _, keepDim := range []bool{true, false} {
		// Forward
		output := backend.SumDim(x, 1, keepDim)
		op := NewSumDimOp(x, output, 1, keepDim)

		// Backward with ones
		outputGrad, _ := tensor.NewRaw(output.Shape(), tensor.Float32, backend.Device())
		outputGradData := outputGrad.AsFloat32()
		for i := range outputGradData {
			outputGradData[i] = 1.0
		}

		grads := op.Backward(outputGrad, backend)

		// Numerical gradient check
		epsilon := float32(1e-4)
		for i := range xData {
			// Save original value
			original := xData[i]

			// Forward with x + epsilon
			xData[i] = original + epsilon
			outputPlus := backend.SumDim(x, 1, keepDim)
			outputPlusData := outputPlus.AsFloat32()

			// Forward with x - epsilon
			xData[i] = original - epsilon
			outputMinus := backend.SumDim(x, 1, keepDim)
			outputMinusData := outputMinus.AsFloat32()

			// Restore original
			xData[i] = original

			// Numerical gradient
			var numericalGrad float32
			for j := range outputPlusData {
				numericalGrad += (outputPlusData[j] - outputMinusData[j]) / (2 * epsilon)
			}

			// Compare with analytical gradient
			analyticalGrad := grads[0].AsFloat32()[i]
			diff := math.Abs(float64(numericalGrad - analyticalGrad))
			// Tolerance of 0.002 for float32 precision (epsilon=1e-4)
			if diff > 0.002 {
				t.Errorf("Gradient mismatch at index %d (keepDim=%v): numerical=%v, analytical=%v, diff=%v",
					i, keepDim, numericalGrad, analyticalGrad, diff)
			}
		}
	}
}

func TestMeanDimOp_GradientCheck(t *testing.T) {
	backend := cpu.New()

	// Input [3, 4]
	x, _ := tensor.NewRaw(tensor.Shape{3, 4}, tensor.Float32, backend.Device())
	xData := x.AsFloat32()
	for i := range xData {
		xData[i] = float32(i+1) * 0.1
	}

	// Test both keepDim=true and keepDim=false
	for _, keepDim := range []bool{true, false} {
		// Forward
		output := backend.MeanDim(x, 1, keepDim)
		op := NewMeanDimOp(x, output, 1, keepDim)

		// Backward with ones
		outputGrad, _ := tensor.NewRaw(output.Shape(), tensor.Float32, backend.Device())
		outputGradData := outputGrad.AsFloat32()
		for i := range outputGradData {
			outputGradData[i] = 1.0
		}

		grads := op.Backward(outputGrad, backend)

		// Numerical gradient check
		epsilon := float32(1e-4)
		for i := range xData {
			// Save original value
			original := xData[i]

			// Forward with x + epsilon
			xData[i] = original + epsilon
			outputPlus := backend.MeanDim(x, 1, keepDim)
			outputPlusData := outputPlus.AsFloat32()

			// Forward with x - epsilon
			xData[i] = original - epsilon
			outputMinus := backend.MeanDim(x, 1, keepDim)
			outputMinusData := outputMinus.AsFloat32()

			// Restore original
			xData[i] = original

			// Numerical gradient
			var numericalGrad float32
			for j := range outputPlusData {
				numericalGrad += (outputPlusData[j] - outputMinusData[j]) / (2 * epsilon)
			}

			// Compare with analytical gradient
			analyticalGrad := grads[0].AsFloat32()[i]
			diff := math.Abs(float64(numericalGrad - analyticalGrad))
			// Tolerance of 0.002 for float32 precision (epsilon=1e-4)
			if diff > 0.002 {
				t.Errorf("Gradient mismatch at index %d (keepDim=%v): numerical=%v, analytical=%v, diff=%v",
					i, keepDim, numericalGrad, analyticalGrad, diff)
			}
		}
	}
}

func TestSumDimOp_3D(t *testing.T) {
	backend := cpu.New()

	// Input [2, 3, 4]
	x, _ := tensor.NewRaw(tensor.Shape{2, 3, 4}, tensor.Float32, backend.Device())
	xData := x.AsFloat32()
	for i := range xData {
		xData[i] = 1.0
	}

	// Test sum along different dimensions
	dims := []int{0, 1, 2, -1, -2}
	for _, dim := range dims {
		output := backend.SumDim(x, dim, true)
		op := NewSumDimOp(x, output, dim, true)

		outputGrad, _ := tensor.NewRaw(output.Shape(), tensor.Float32, backend.Device())
		outputGradData := outputGrad.AsFloat32()
		for i := range outputGradData {
			outputGradData[i] = 1.0
		}

		grads := op.Backward(outputGrad, backend)

		// Gradient should match input shape
		if !grads[0].Shape().Equal(x.Shape()) {
			t.Errorf("Dim %d: gradient shape %v doesn't match input shape %v", dim, grads[0].Shape(), x.Shape())
		}

		// All gradients should be 1.0
		gradData := grads[0].AsFloat32()
		for i, g := range gradData {
			if g != 1.0 {
				t.Errorf("Dim %d: expected gradient 1.0 at index %d, got %v", dim, i, g)
			}
		}
	}
}
