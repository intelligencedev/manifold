package cpu

import (
	"math"
	"testing"

	"github.com/born-ml/born/internal/tensor"
)

const epsilon = 1e-5

func TestExp(t *testing.T) {
	backend := New()

	tests := []struct {
		name  string
		input []float32
		shape tensor.Shape
	}{
		{
			name:  "positive values",
			input: []float32{0, 1, 2, 3},
			shape: tensor.Shape{4},
		},
		{
			name:  "negative values",
			input: []float32{-3, -2, -1, 0},
			shape: tensor.Shape{4},
		},
		{
			name:  "zero",
			input: []float32{0},
			shape: tensor.Shape{1},
		},
		{
			name:  "2D tensor",
			input: []float32{0, 1, -1, 2},
			shape: tensor.Shape{2, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x, err := tensor.NewRaw(tt.shape, tensor.Float32, backend.Device())
			if err != nil {
				t.Fatalf("Failed to create tensor: %v", err)
			}
			copy(x.AsFloat32(), tt.input)

			result := backend.Exp(x)

			if !result.Shape().Equal(tt.shape) {
				t.Errorf("Expected shape %v, got %v", tt.shape, result.Shape())
			}

			output := result.AsFloat32()
			for i, v := range tt.input {
				expected := float32(math.Exp(float64(v)))
				if math.Abs(float64(output[i]-expected)) > epsilon {
					t.Errorf("exp(%f) = %f, expected %f", v, output[i], expected)
				}
			}
		})
	}
}

func TestSqrt(t *testing.T) {
	backend := New()

	tests := []struct {
		name  string
		input []float32
		shape tensor.Shape
	}{
		{
			name:  "positive values",
			input: []float32{1, 4, 9, 16},
			shape: tensor.Shape{4},
		},
		{
			name:  "zero",
			input: []float32{0},
			shape: tensor.Shape{1},
		},
		{
			name:  "2D tensor",
			input: []float32{1, 2, 3, 4},
			shape: tensor.Shape{2, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x, err := tensor.NewRaw(tt.shape, tensor.Float32, backend.Device())
			if err != nil {
				t.Fatalf("Failed to create tensor: %v", err)
			}
			copy(x.AsFloat32(), tt.input)

			result := backend.Sqrt(x)

			if !result.Shape().Equal(tt.shape) {
				t.Errorf("Expected shape %v, got %v", tt.shape, result.Shape())
			}

			output := result.AsFloat32()
			for i, v := range tt.input {
				expected := float32(math.Sqrt(float64(v)))
				if math.Abs(float64(output[i]-expected)) > epsilon {
					t.Errorf("sqrt(%f) = %f, expected %f", v, output[i], expected)
				}
			}
		})
	}
}

func TestSqrtNegativePanic(t *testing.T) {
	backend := New()
	x, _ := tensor.NewRaw(tensor.Shape{2}, tensor.Float32, backend.Device())
	copy(x.AsFloat32(), []float32{-1, 1})

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic for negative value")
		}
	}()

	backend.Sqrt(x)
}

func TestRsqrt(t *testing.T) {
	backend := New()

	tests := []struct {
		name  string
		input []float32
		shape tensor.Shape
	}{
		{
			name:  "positive values",
			input: []float32{1, 4, 9, 16},
			shape: tensor.Shape{4},
		},
		{
			name:  "2D tensor",
			input: []float32{1, 2, 3, 4},
			shape: tensor.Shape{2, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x, err := tensor.NewRaw(tt.shape, tensor.Float32, backend.Device())
			if err != nil {
				t.Fatalf("Failed to create tensor: %v", err)
			}
			copy(x.AsFloat32(), tt.input)

			result := backend.Rsqrt(x)

			if !result.Shape().Equal(tt.shape) {
				t.Errorf("Expected shape %v, got %v", tt.shape, result.Shape())
			}

			output := result.AsFloat32()
			for i, v := range tt.input {
				expected := float32(1.0 / math.Sqrt(float64(v)))
				if math.Abs(float64(output[i]-expected)) > epsilon {
					t.Errorf("rsqrt(%f) = %f, expected %f", v, output[i], expected)
				}
			}
		})
	}
}

func TestRsqrtNonPositivePanic(t *testing.T) {
	backend := New()

	tests := []struct {
		name  string
		input []float32
	}{
		{"negative", []float32{-1, 1}},
		{"zero", []float32{0, 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x, _ := tensor.NewRaw(tensor.Shape{2}, tensor.Float32, backend.Device())
			copy(x.AsFloat32(), tt.input)

			defer func() {
				if r := recover(); r == nil {
					t.Errorf("Expected panic for %s value", tt.name)
				}
			}()

			backend.Rsqrt(x)
		})
	}
}

func TestCos(t *testing.T) {
	backend := New()

	tests := []struct {
		name  string
		input []float32
		shape tensor.Shape
	}{
		{
			name:  "zero to pi",
			input: []float32{0, float32(math.Pi / 2), float32(math.Pi), float32(3 * math.Pi / 2)},
			shape: tensor.Shape{4},
		},
		{
			name:  "negative values",
			input: []float32{float32(-math.Pi), float32(-math.Pi / 2), 0},
			shape: tensor.Shape{3},
		},
		{
			name:  "2D tensor",
			input: []float32{0, float32(math.Pi / 4), float32(math.Pi / 2), float32(math.Pi)},
			shape: tensor.Shape{2, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x, err := tensor.NewRaw(tt.shape, tensor.Float32, backend.Device())
			if err != nil {
				t.Fatalf("Failed to create tensor: %v", err)
			}
			copy(x.AsFloat32(), tt.input)

			result := backend.Cos(x)

			if !result.Shape().Equal(tt.shape) {
				t.Errorf("Expected shape %v, got %v", tt.shape, result.Shape())
			}

			output := result.AsFloat32()
			for i, v := range tt.input {
				expected := float32(math.Cos(float64(v)))
				if math.Abs(float64(output[i]-expected)) > epsilon {
					t.Errorf("cos(%f) = %f, expected %f", v, output[i], expected)
				}
			}
		})
	}
}

func TestSin(t *testing.T) {
	backend := New()

	tests := []struct {
		name  string
		input []float32
		shape tensor.Shape
	}{
		{
			name:  "zero to pi",
			input: []float32{0, float32(math.Pi / 2), float32(math.Pi), float32(3 * math.Pi / 2)},
			shape: tensor.Shape{4},
		},
		{
			name:  "negative values",
			input: []float32{float32(-math.Pi), float32(-math.Pi / 2), 0},
			shape: tensor.Shape{3},
		},
		{
			name:  "2D tensor",
			input: []float32{0, float32(math.Pi / 4), float32(math.Pi / 2), float32(math.Pi)},
			shape: tensor.Shape{2, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x, err := tensor.NewRaw(tt.shape, tensor.Float32, backend.Device())
			if err != nil {
				t.Fatalf("Failed to create tensor: %v", err)
			}
			copy(x.AsFloat32(), tt.input)

			result := backend.Sin(x)

			if !result.Shape().Equal(tt.shape) {
				t.Errorf("Expected shape %v, got %v", tt.shape, result.Shape())
			}

			output := result.AsFloat32()
			for i, v := range tt.input {
				expected := float32(math.Sin(float64(v)))
				if math.Abs(float64(output[i]-expected)) > epsilon {
					t.Errorf("sin(%f) = %f, expected %f", v, output[i], expected)
				}
			}
		})
	}
}

func TestMathFloat64(t *testing.T) {
	backend := New()

	t.Run("Exp float64", func(t *testing.T) {
		x, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float64, backend.Device())
		copy(x.AsFloat64(), []float64{0, 1, -1})

		result := backend.Exp(x)
		output := result.AsFloat64()

		expected := []float64{math.Exp(0), math.Exp(1), math.Exp(-1)}
		for i := range output {
			if math.Abs(output[i]-expected[i]) > epsilon {
				t.Errorf("exp(%f) = %f, expected %f", []float64{0, 1, -1}[i], output[i], expected[i])
			}
		}
	})

	t.Run("Sqrt float64", func(t *testing.T) {
		x, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float64, backend.Device())
		copy(x.AsFloat64(), []float64{1, 4, 9})

		result := backend.Sqrt(x)
		output := result.AsFloat64()

		expected := []float64{1, 2, 3}
		for i := range output {
			if math.Abs(output[i]-expected[i]) > epsilon {
				t.Errorf("sqrt(%f) = %f, expected %f", []float64{1, 4, 9}[i], output[i], expected[i])
			}
		}
	})
}
