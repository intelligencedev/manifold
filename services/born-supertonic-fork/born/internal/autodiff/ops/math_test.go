package ops

import (
	"math"
	"testing"

	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/tensor"
)

const (
	epsilonGrad = 1e-4
	tolerance   = 0.1 // 10% relative error tolerance for numerical gradients
)

// numericalGradient computes numerical gradient using finite differences.
// This assumes the loss is sum of all elements in the output (matching grad_output of all ones).
func numericalGradient(
	fn func(*tensor.RawTensor) *tensor.RawTensor,
	input *tensor.RawTensor,
	backend tensor.Backend,
) *tensor.RawTensor {
	grad, err := tensor.NewRaw(input.Shape(), input.DType(), backend.Device())
	if err != nil {
		panic(err)
	}

	eps := float64(epsilonGrad)

	switch input.DType() {
	case tensor.Float32:
		inputData := input.AsFloat32()
		gradData := grad.AsFloat32()

		for i := range inputData {
			// f(x + h)
			original := inputData[i]
			inputData[i] = original + float32(eps)
			fPlus := fn(input)
			fPlusVal := sumElements(fPlus)

			// f(x - h)
			inputData[i] = original - float32(eps)
			fMinus := fn(input)
			fMinusVal := sumElements(fMinus)

			// (f(x+h) - f(x-h)) / (2h)
			gradData[i] = float32((fPlusVal - fMinusVal) / (2.0 * eps))

			// Restore original value
			inputData[i] = original
		}

	case tensor.Float64:
		inputData := input.AsFloat64()
		gradData := grad.AsFloat64()

		for i := range inputData {
			original := inputData[i]
			inputData[i] = original + eps
			fPlus := fn(input)
			fPlusVal := sumElements(fPlus)

			inputData[i] = original - eps
			fMinus := fn(input)
			fMinusVal := sumElements(fMinus)

			gradData[i] = (fPlusVal - fMinusVal) / (2.0 * eps)
			inputData[i] = original
		}
	}

	return grad
}

// sumElements sums all elements of a tensor.
func sumElements(t *tensor.RawTensor) float64 {
	var sum float64
	switch t.DType() {
	case tensor.Float32:
		for _, v := range t.AsFloat32() {
			sum += float64(v)
		}
	case tensor.Float64:
		for _, v := range t.AsFloat64() {
			sum += v
		}
	}
	return sum
}

// compareGradients checks if analytical and numerical gradients match.
func compareGradients(t *testing.T, analytical, numerical *tensor.RawTensor, name string) {
	t.Helper()

	if !analytical.Shape().Equal(numerical.Shape()) {
		t.Fatalf("%s: gradient shapes don't match: %v vs %v",
			name, analytical.Shape(), numerical.Shape())
	}

	switch analytical.DType() {
	case tensor.Float32:
		aData := analytical.AsFloat32()
		nData := numerical.AsFloat32()

		for i := range aData {
			diff := math.Abs(float64(aData[i] - nData[i]))
			if diff > tolerance {
				t.Errorf("%s: gradient[%d] mismatch: analytical=%f, numerical=%f, diff=%f",
					name, i, aData[i], nData[i], diff)
			}
		}

	case tensor.Float64:
		aData := analytical.AsFloat64()
		nData := numerical.AsFloat64()

		for i := range aData {
			diff := math.Abs(aData[i] - nData[i])
			if diff > tolerance {
				t.Errorf("%s: gradient[%d] mismatch: analytical=%f, numerical=%f, diff=%f",
					name, i, aData[i], nData[i], diff)
			}
		}
	}
}

func TestExpGradient(t *testing.T) {
	backend := cpu.New()

	tests := []struct {
		name  string
		input []float32
		shape tensor.Shape
	}{
		{
			name:  "positive values",
			input: []float32{0.5, 1.0, 1.5},
			shape: tensor.Shape{3},
		},
		{
			name:  "negative values",
			input: []float32{-1.5, -1.0, -0.5},
			shape: tensor.Shape{3},
		},
		{
			name:  "2D tensor",
			input: []float32{0, 1, -1, 2},
			shape: tensor.Shape{2, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, _ := tensor.NewRaw(tt.shape, tensor.Float32, backend.Device())
			copy(input.AsFloat32(), tt.input)

			// Forward pass
			output := backend.Exp(input)
			op := NewExpOp(input, output)

			// Analytical gradient
			outputGrad := createScalar(output.Shape(), output.DType(), 1.0, backend.Device())
			analyticalGrad := op.Backward(outputGrad, backend)[0]

			// Numerical gradient
			numericalGrad := numericalGradient(func(x *tensor.RawTensor) *tensor.RawTensor {
				return backend.Exp(x)
			}, input, backend)

			compareGradients(t, analyticalGrad, numericalGrad, "exp")
		})
	}
}

func TestSqrtGradient(t *testing.T) {
	backend := cpu.New()

	tests := []struct {
		name  string
		input []float32
		shape tensor.Shape
	}{
		{
			name:  "positive values",
			input: []float32{1.0, 4.0, 9.0},
			shape: tensor.Shape{3},
		},
		{
			name:  "2D tensor",
			input: []float32{1, 2, 3, 4},
			shape: tensor.Shape{2, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, _ := tensor.NewRaw(tt.shape, tensor.Float32, backend.Device())
			copy(input.AsFloat32(), tt.input)

			// Forward pass
			output := backend.Sqrt(input)
			op := NewSqrtOp(input, output)

			// Analytical gradient
			outputGrad := createScalar(output.Shape(), output.DType(), 1.0, backend.Device())
			analyticalGrad := op.Backward(outputGrad, backend)[0]

			// Numerical gradient
			numericalGrad := numericalGradient(func(x *tensor.RawTensor) *tensor.RawTensor {
				return backend.Sqrt(x)
			}, input, backend)

			compareGradients(t, analyticalGrad, numericalGrad, "sqrt")
		})
	}
}

func TestRsqrtGradient(t *testing.T) {
	backend := cpu.New()

	tests := []struct {
		name  string
		input []float32
		shape tensor.Shape
	}{
		{
			name:  "positive values",
			input: []float32{1.0, 4.0, 9.0},
			shape: tensor.Shape{3},
		},
		{
			name:  "2D tensor",
			input: []float32{1, 2, 3, 4},
			shape: tensor.Shape{2, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, _ := tensor.NewRaw(tt.shape, tensor.Float32, backend.Device())
			copy(input.AsFloat32(), tt.input)

			// Forward pass
			output := backend.Rsqrt(input)
			op := NewRsqrtOp(input, output)

			// Analytical gradient
			outputGrad := createScalar(output.Shape(), output.DType(), 1.0, backend.Device())
			analyticalGrad := op.Backward(outputGrad, backend)[0]

			// Numerical gradient
			numericalGrad := numericalGradient(func(x *tensor.RawTensor) *tensor.RawTensor {
				return backend.Rsqrt(x)
			}, input, backend)

			compareGradients(t, analyticalGrad, numericalGrad, "rsqrt")
		})
	}
}

func TestCosGradient(t *testing.T) {
	backend := cpu.New()

	tests := []struct {
		name  string
		input []float32
		shape tensor.Shape
	}{
		{
			name:  "various angles",
			input: []float32{0, float32(math.Pi / 4), float32(math.Pi / 2)},
			shape: tensor.Shape{3},
		},
		{
			name:  "2D tensor",
			input: []float32{0, float32(math.Pi / 6), float32(math.Pi / 3), float32(math.Pi / 2)},
			shape: tensor.Shape{2, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, _ := tensor.NewRaw(tt.shape, tensor.Float32, backend.Device())
			copy(input.AsFloat32(), tt.input)

			// Forward pass
			output := backend.Cos(input)
			op := NewCosOp(input, output)

			// Analytical gradient
			outputGrad := createScalar(output.Shape(), output.DType(), 1.0, backend.Device())
			analyticalGrad := op.Backward(outputGrad, backend)[0]

			// Numerical gradient
			numericalGrad := numericalGradient(func(x *tensor.RawTensor) *tensor.RawTensor {
				return backend.Cos(x)
			}, input, backend)

			compareGradients(t, analyticalGrad, numericalGrad, "cos")
		})
	}
}

func TestSinGradient(t *testing.T) {
	backend := cpu.New()

	tests := []struct {
		name  string
		input []float32
		shape tensor.Shape
	}{
		{
			name:  "various angles",
			input: []float32{0, float32(math.Pi / 4), float32(math.Pi / 2)},
			shape: tensor.Shape{3},
		},
		{
			name:  "2D tensor",
			input: []float32{0, float32(math.Pi / 6), float32(math.Pi / 3), float32(math.Pi / 2)},
			shape: tensor.Shape{2, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, _ := tensor.NewRaw(tt.shape, tensor.Float32, backend.Device())
			copy(input.AsFloat32(), tt.input)

			// Forward pass
			output := backend.Sin(input)
			op := NewSinOp(input, output)

			// Analytical gradient
			outputGrad := createScalar(output.Shape(), output.DType(), 1.0, backend.Device())
			analyticalGrad := op.Backward(outputGrad, backend)[0]

			// Numerical gradient
			numericalGrad := numericalGradient(func(x *tensor.RawTensor) *tensor.RawTensor {
				return backend.Sin(x)
			}, input, backend)

			compareGradients(t, analyticalGrad, numericalGrad, "sin")
		})
	}
}
