package ops_test

import (
	"math"
	"testing"

	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/tensor"
)

type signTestCase[T uint8 | int32 | int64 | float32 | float64] struct {
	name  string
	a     []T
	want  []T
	shape tensor.Shape
}

func TestSign_ForwardUint8(t *testing.T) {
	backend := cpu.New()

	// Test cases for int32
	tests := []signTestCase[uint8]{
		{"basic", []uint8{0, 1, math.MaxUint8}, []uint8{0, 1, 1}, tensor.Shape{3}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, _ := tensor.NewRaw(tt.shape, tensor.Uint8, backend.Device())
			copy(input.AsUint8(), tt.a)

			result := backend.Sign(input)

			if !result.Shape().Equal(tt.shape) {
				t.Errorf("Expected shape %v, got %v", tt.shape, result.Shape())
			}

			outputData := result.AsUint8()
			for i, v := range outputData {
				if math.Abs(float64(v-tt.want[i])) > epsilon {
					t.Errorf("sign(%d) = %d, want %d", tt.a[i], v, tt.want[i])
				}
			}
		})
	}
}

func TestSign_ForwardInt32(t *testing.T) {
	backend := cpu.New()

	// Test cases for int32
	tests := []signTestCase[int32]{
		{"basic", []int32{-1, 0, 1}, []int32{-1, 0, 1}, tensor.Shape{3}},
		{"edges", []int32{int32(math.MinInt32), int32(math.MaxInt32)}, []int32{-1, 1}, tensor.Shape{2}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, _ := tensor.NewRaw(tt.shape, tensor.Int32, backend.Device())
			copy(input.AsInt32(), tt.a)

			result := backend.Sign(input)

			if !result.Shape().Equal(tt.shape) {
				t.Errorf("Expected shape %v, got %v", tt.shape, result.Shape())
			}

			outputData := result.AsInt32()
			for i, v := range outputData {
				if math.Abs(float64(v-tt.want[i])) > epsilon {
					t.Errorf("sign(%d) = %d, want %d", tt.a[i], v, tt.want[i])
				}
			}
		})
	}
}

func TestSign_ForwardInt64(t *testing.T) {
	backend := cpu.New()

	// Test cases for int64
	tests := []signTestCase[int64]{
		{"basic", []int64{-1, 0, 1}, []int64{-1, 0, 1}, tensor.Shape{3}},
		{"edges", []int64{int64(math.MinInt64), int64(math.MaxInt64)}, []int64{-1, 1}, tensor.Shape{2}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, _ := tensor.NewRaw(tt.shape, tensor.Int64, backend.Device())
			copy(input.AsInt64(), tt.a)

			result := backend.Sign(input)

			if !result.Shape().Equal(tt.shape) {
				t.Errorf("Expected shape %v, got %v", tt.shape, result.Shape())
			}

			outputData := result.AsInt64()
			for i, v := range outputData {
				if math.Abs(float64(v-tt.want[i])) > epsilon {
					t.Errorf("sign(%d) = %d, want %d", tt.a[i], v, tt.want[i])
				}
			}
		})
	}
}

func TestSign_ForwardFloat32(t *testing.T) {
	backend := cpu.New()

	// Test cases for float32
	tests := []signTestCase[float32]{
		{"basic", []float32{-1.0, 0.0, 1.0}, []float32{-1.0, 0.0, 1.0}, tensor.Shape{3}},
		{"edges", []float32{float32(math.Inf(-1)), float32(math.Inf(1)), float32(math.NaN())}, []float32{-1.0, 1.0, float32(math.NaN())}, tensor.Shape{3}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, _ := tensor.NewRaw(tt.shape, tensor.Float32, backend.Device())
			copy(input.AsFloat32(), tt.a)

			result := backend.Sign(input)

			if !result.Shape().Equal(tt.shape) {
				t.Errorf("Expected shape %v, got %v", tt.shape, result.Shape())
			}

			outputData := result.AsFloat32()
			for i, v := range outputData {
				if math.IsNaN(float64(tt.want[i])) {
					if !math.IsNaN(float64(v)) {
						t.Errorf("sign(NaN) = %v, want NaN", v)
					}
					continue
				}
				if math.Abs(float64(v-tt.want[i])) > epsilon {
					t.Errorf("sign(%f) = %f, want %f", tt.a[i], v, tt.want[i])
				}
			}
		})
	}
}

func TestSign_ForwardFloat64(t *testing.T) {
	backend := cpu.New()

	// Test cases for float64
	tests := []signTestCase[float64]{
		{"basic", []float64{-1.0, 0.0, 1.0}, []float64{-1.0, 0.0, 1.0}, tensor.Shape{3}},
		{"edges", []float64{math.Inf(-1), math.Inf(1), math.NaN()}, []float64{-1.0, 1.0, math.NaN()}, tensor.Shape{3}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, _ := tensor.NewRaw(tt.shape, tensor.Float64, backend.Device())
			copy(input.AsFloat64(), tt.a)

			result := backend.Sign(input)

			if !result.Shape().Equal(tt.shape) {
				t.Errorf("Expected shape %v, got %v", tt.shape, result.Shape())
			}

			outputData := result.AsFloat64()
			for i, v := range outputData {
				if math.IsNaN(float64(tt.want[i])) {
					if !math.IsNaN(float64(v)) {
						t.Errorf("sign(NaN) = %v, want NaN", v)
					}
					continue
				}
				if math.Abs(v-tt.want[i]) > epsilon {
					t.Errorf("sign(%f) = %f, want %f", tt.a[i], v, tt.want[i])
				}
			}
		})
	}
}
