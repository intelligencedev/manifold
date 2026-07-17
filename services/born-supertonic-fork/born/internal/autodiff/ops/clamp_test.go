package ops

import (
	"math"
	"testing"

	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/tensor"
)

const epsilon = 1e-5

// floatAlmostEqual checks if two float64 values are approximately equal within a given epsilon, treating NaNs as equal.
func floatAlmostEqual(a, b float64) bool {
	if math.IsNaN(a) {
		return math.IsNaN(b)
	}
	if math.IsNaN(b) {
		return math.IsNaN(a)
	}
	return math.Abs(a-b) <= epsilon
}

type clampTestCase[T int32 | int64 | float32 | float64] struct {
	name  string
	a     []T
	want  []T
	shape tensor.Shape
	min   T
	max   T
}

func TestClamp_ForwardInt32(t *testing.T) {
	backend := cpu.New()
	tests := []clampTestCase[int32]{
		{"basic", []int32{-2, -1, 0, 1, 2}, []int32{-1, -1, 0, 1, 1}, tensor.Shape{5}, -1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, _ := tensor.NewRaw(tt.shape, tensor.Int32, backend.Device())
			copy(input.AsInt32(), tt.a)

			result := backend.Clamp(input, tt.min, tt.max)

			if !result.Shape().Equal(tt.shape) {
				t.Errorf("Expected shape %v, got %v", tt.shape, result.Shape())
			}

			outputData := result.AsInt32()
			for i, v := range outputData {
				if v != tt.want[i] {
					t.Errorf("clamp(%d) = %d, want %d", tt.a[i], v, tt.want[i])
				}
			}
		})
	}
}

func TestClamp_ForwardInt64(t *testing.T) {
	backend := cpu.New()
	tests := []clampTestCase[int64]{
		{"basic", []int64{-2, -1, 0, 1, 2}, []int64{-1, -1, 0, 1, 1}, tensor.Shape{5}, -1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, _ := tensor.NewRaw(tt.shape, tensor.Int64, backend.Device())
			copy(input.AsInt64(), tt.a)

			result := backend.Clamp(input, tt.min, tt.max)

			if !result.Shape().Equal(tt.shape) {
				t.Errorf("Expected shape %v, got %v", tt.shape, result.Shape())
			}

			outputData := result.AsInt64()
			for i, v := range outputData {
				if v != tt.want[i] {
					t.Errorf("clamp(%d) = %d, want %d", tt.a[i], v, tt.want[i])
				}
			}
		})
	}
}

func TestClamp_ForwardFloat32(t *testing.T) {
	backend := cpu.New()
	tests := []clampTestCase[float32]{
		{"basic", []float32{-2, -1, 0, 1, 2}, []float32{-1, -1, 0, 1, 1}, tensor.Shape{5}, -1, 1},
		{"edges", []float32{float32(math.Inf(-1)), float32(math.Inf(1)), float32(math.NaN())}, []float32{-1, 1, float32(math.NaN())}, tensor.Shape{3}, float32(-1), float32(1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, _ := tensor.NewRaw(tt.shape, tensor.Float32, backend.Device())
			copy(input.AsFloat32(), tt.a)

			result := backend.Clamp(input, tt.min, tt.max)

			if !result.Shape().Equal(tt.shape) {
				t.Errorf("Expected shape %v, got %v", tt.shape, result.Shape())
			}

			outputData := result.AsFloat32()
			for i, v := range outputData {
				if !floatAlmostEqual(float64(v), float64(tt.want[i])) {
					t.Errorf("clamp(%f) = %f, want %f", tt.a[i], v, tt.want[i])
				}
			}
		})
	}
}

func TestClamp_ForwardFloat64(t *testing.T) {
	backend := cpu.New()
	tests := []clampTestCase[float64]{
		{"basic", []float64{-2, -1, 0, 1, 2}, []float64{-1, -1, 0, 1, 1}, tensor.Shape{5}, -1, 1},
		{"edges", []float64{math.Inf(-1), math.Inf(1), math.NaN()}, []float64{-1, 1, math.NaN()}, tensor.Shape{3}, -1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, _ := tensor.NewRaw(tt.shape, tensor.Float64, backend.Device())
			copy(input.AsFloat64(), tt.a)

			result := backend.Clamp(input, tt.min, tt.max)

			if !result.Shape().Equal(tt.shape) {
				t.Errorf("Expected shape %v, got %v", tt.shape, result.Shape())
			}

			outputData := result.AsFloat64()
			for i, v := range outputData {
				if !floatAlmostEqual(v, tt.want[i]) {
					t.Errorf("clamp(%f) = %f, want %f", tt.a[i], v, tt.want[i])
				}
			}
		})
	}
}

// TestClamp_NaNBoundsPanic verifies that NaN bounds cause panics in forward pass.
func TestClamp_NaNBoundsPanic_Float32(t *testing.T) {
	backend := cpu.New()
	input, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, backend.Device())
	copy(input.AsFloat32(), []float32{-1, 0, 1})

	tests := []struct {
		name  string
		min   float32
		max   float32
		isMin bool // true if NaN is in min, false if in max
	}{
		{"nan_min", float32(math.NaN()), 1, true},
		{"nan_max", 0, float32(math.NaN()), false},
		{"both_nan", float32(math.NaN()), float32(math.NaN()), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("Expected panic for NaN bounds, but none occurred")
				}
			}()
			backend.Clamp(input, tt.min, tt.max)
		})
	}
}

func TestClamp_NaNBoundsPanic_Float64(t *testing.T) {
	backend := cpu.New()
	input, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float64, backend.Device())
	copy(input.AsFloat64(), []float64{-1, 0, 1})

	tests := []struct {
		name  string
		min   float64
		max   float64
		isMin bool // true if NaN is in min, false if in max
	}{
		{"nan_min", math.NaN(), 1, true},
		{"nan_max", 0, math.NaN(), false},
		{"both_nan", math.NaN(), math.NaN(), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("Expected panic for NaN bounds, but none occurred")
				}
			}()
			backend.Clamp(input, tt.min, tt.max)
		})
	}
}

func TestClamp_BackwardFloat32(t *testing.T) {
	backend := cpu.New()

	tests := []struct {
		name       string
		input      []float32
		outputGrad []float32
		min        float32
		max        float32
		want       []float32 // Expected gradient
	}{
		{
			name:       "inside_bounds",
			input:      []float32{-0.5, 0, 0.5, 1, 1.5},
			outputGrad: []float32{2, 2, 2, 2, 2},
			min:        0,
			max:        1,
			want:       []float32{0, 2, 2, 2, 0}, // Only values in [0,1] pass gradient
		},
		{
			name:       "below_min",
			input:      []float32{-2, -1},
			outputGrad: []float32{3, 3},
			min:        0,
			max:        1,
			want:       []float32{0, 0}, // All below min: no gradient
		},
		{
			name:       "above_max",
			input:      []float32{1.5, 2},
			outputGrad: []float32{4, 4},
			min:        0,
			max:        1,
			want:       []float32{0, 0}, // All above max: no gradient
		},
		{
			name:       "at_boundaries",
			input:      []float32{0, 0.5, 1},
			outputGrad: []float32{1, 1, 1},
			min:        0,
			max:        1,
			want:       []float32{1, 1, 1}, // At boundaries: gradient passes
		},
		{
			name:       "mixed_values",
			input:      []float32{-1, 0, 0.5, 1, 2},
			outputGrad: []float32{2, 2, 2, 2, 2},
			min:        0,
			max:        1,
			want:       []float32{0, 2, 2, 2, 0},
		},
		{
			name:       "nan_input",
			input:      []float32{float32(math.NaN()), 0, 1, float32(math.NaN())},
			outputGrad: []float32{1, 1, 1, 1},
			min:        0,
			max:        1,
			want:       []float32{0, 1, 1, 0}, // NaN comparisons return false, so gradient is zero
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, _ := tensor.NewRaw(tensor.Shape{len(tt.input)}, tensor.Float32, backend.Device())
			copy(input.AsFloat32(), tt.input)

			outputGrad, _ := tensor.NewRaw(tensor.Shape{len(tt.outputGrad)}, tensor.Float32, backend.Device())
			copy(outputGrad.AsFloat32(), tt.outputGrad)

			// Create clamp operation
			output := backend.Clamp(input, tt.min, tt.max)
			op := NewClampOp(input, tt.min, tt.max, output)

			// Compute backward
			grads := op.Backward(outputGrad, backend)
			inputGrad := grads[0]

			// Verify shape
			if !inputGrad.Shape().Equal(tensor.Shape{len(tt.want)}) {
				t.Errorf("Expected shape %v, got %v", tensor.Shape{len(tt.want)}, inputGrad.Shape())
			}

			// Verify values
			gradData := inputGrad.AsFloat32()
			for i, v := range gradData {
				if !floatAlmostEqual(float64(v), float64(tt.want[i])) {
					t.Errorf("grad[%d] = %f, want %f (input=%f, grad_out=%f)", i, v, tt.want[i], tt.input[i], tt.outputGrad[i])
				}
			}
		})
	}
}

func TestClamp_BackwardFloat64(t *testing.T) {
	backend := cpu.New()

	tests := []struct {
		name       string
		input      []float64
		outputGrad []float64
		min        float64
		max        float64
		want       []float64 // Expected gradient
	}{
		{
			name:       "inside_bounds",
			input:      []float64{-0.5, 0, 0.5, 1, 1.5},
			outputGrad: []float64{2, 2, 2, 2, 2},
			min:        0,
			max:        1,
			want:       []float64{0, 2, 2, 2, 0},
		},
		{
			name:       "below_min",
			input:      []float64{-2, -1},
			outputGrad: []float64{3, 3},
			min:        0,
			max:        1,
			want:       []float64{0, 0},
		},
		{
			name:       "above_max",
			input:      []float64{1.5, 2},
			outputGrad: []float64{4, 4},
			min:        0,
			max:        1,
			want:       []float64{0, 0},
		},
		{
			name:       "at_boundaries",
			input:      []float64{0, 0.5, 1},
			outputGrad: []float64{1, 1, 1},
			min:        0,
			max:        1,
			want:       []float64{1, 1, 1},
		},
		{
			name:       "negative_bounds",
			input:      []float64{-2, -1, 0, 1},
			outputGrad: []float64{1, 1, 1, 1},
			min:        -1,
			max:        0,
			want:       []float64{0, 1, 1, 0},
		},
		{
			name:       "zero_gradient",
			input:      []float64{-0.5, 0, 0.5, 1, 1.5},
			outputGrad: []float64{0, 0, 0, 0, 0},
			min:        0,
			max:        1,
			want:       []float64{0, 0, 0, 0, 0},
		},
		{
			name:       "nan_input",
			input:      []float64{math.NaN(), 0, 1, math.NaN()},
			outputGrad: []float64{1, 1, 1, 1},
			min:        0,
			max:        1,
			want:       []float64{0, 1, 1, 0}, // NaN comparisons return false, so gradient is zero
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, _ := tensor.NewRaw(tensor.Shape{len(tt.input)}, tensor.Float64, backend.Device())
			copy(input.AsFloat64(), tt.input)

			outputGrad, _ := tensor.NewRaw(tensor.Shape{len(tt.outputGrad)}, tensor.Float64, backend.Device())
			copy(outputGrad.AsFloat64(), tt.outputGrad)

			// Create clamp operation
			output := backend.Clamp(input, tt.min, tt.max)
			op := NewClampOp(input, tt.min, tt.max, output)

			// Compute backward
			grads := op.Backward(outputGrad, backend)
			inputGrad := grads[0]

			// Verify shape
			if !inputGrad.Shape().Equal(tensor.Shape{len(tt.want)}) {
				t.Errorf("Expected shape %v, got %v", tensor.Shape{len(tt.want)}, inputGrad.Shape())
			}

			// Verify values
			gradData := inputGrad.AsFloat64()
			for i, v := range gradData {
				if !floatAlmostEqual(v, tt.want[i]) {
					t.Errorf("grad[%d] = %f, want %f (input=%f, grad_out=%f)", i, v, tt.want[i], tt.input[i], tt.outputGrad[i])
				}
			}
		})
	}
}

// TestClamp_BackwardInt32 tests the backward pass gradient computation for int32.
func TestClamp_BackwardInt32(t *testing.T) {
	backend := cpu.New()

	tests := []struct {
		name       string
		input      []int32
		outputGrad []int32
		min        int32
		max        int32
		want       []int32 // Expected gradient
	}{
		{
			name:       "inside_bounds",
			input:      []int32{-1, 0, 1, 2, 3},
			outputGrad: []int32{2, 2, 2, 2, 2},
			min:        0,
			max:        2,
			want:       []int32{0, 2, 2, 2, 0},
		},
		{
			name:       "at_boundaries",
			input:      []int32{0, 1, 2},
			outputGrad: []int32{5, 5, 5},
			min:        0,
			max:        2,
			want:       []int32{5, 5, 5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, _ := tensor.NewRaw(tensor.Shape{len(tt.input)}, tensor.Int32, backend.Device())
			copy(input.AsInt32(), tt.input)

			outputGrad, _ := tensor.NewRaw(tensor.Shape{len(tt.outputGrad)}, tensor.Int32, backend.Device())
			copy(outputGrad.AsInt32(), tt.outputGrad)

			// Create clamp operation
			output := backend.Clamp(input, tt.min, tt.max)
			op := NewClampOp(input, tt.min, tt.max, output)

			// Compute backward
			grads := op.Backward(outputGrad, backend)
			inputGrad := grads[0]

			// Verify values
			gradData := inputGrad.AsInt32()
			for i, v := range gradData {
				if v != tt.want[i] {
					t.Errorf("grad[%d] = %d, want %d (input=%d, grad_out=%d)", i, v, tt.want[i], tt.input[i], tt.outputGrad[i])
				}
			}
		})
	}
}

// TestClamp_BackwardInt64 tests the backward pass gradient computation for int64.
func TestClamp_BackwardInt64(t *testing.T) {
	backend := cpu.New()

	tests := []struct {
		name       string
		input      []int64
		outputGrad []int64
		min        int64
		max        int64
		want       []int64 // Expected gradient
	}{
		{
			name:       "inside_bounds",
			input:      []int64{-1, 0, 1, 2, 3},
			outputGrad: []int64{2, 2, 2, 2, 2},
			min:        0,
			max:        2,
			want:       []int64{0, 2, 2, 2, 0},
		},
		{
			name:       "at_boundaries",
			input:      []int64{0, 1, 2},
			outputGrad: []int64{5, 5, 5},
			min:        0,
			max:        2,
			want:       []int64{5, 5, 5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, _ := tensor.NewRaw(tensor.Shape{len(tt.input)}, tensor.Int64, backend.Device())
			copy(input.AsInt64(), tt.input)

			outputGrad, _ := tensor.NewRaw(tensor.Shape{len(tt.outputGrad)}, tensor.Int64, backend.Device())
			copy(outputGrad.AsInt64(), tt.outputGrad)

			// Create clamp operation
			output := backend.Clamp(input, tt.min, tt.max)
			op := NewClampOp(input, tt.min, tt.max, output)

			// Compute backward
			grads := op.Backward(outputGrad, backend)
			inputGrad := grads[0]

			// Verify values
			gradData := inputGrad.AsInt64()
			for i, v := range gradData {
				if v != tt.want[i] {
					t.Errorf("grad[%d] = %d, want %d (input=%d, grad_out=%d)", i, v, tt.want[i], tt.input[i], tt.outputGrad[i])
				}
			}
		})
	}
}
