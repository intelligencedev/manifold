package ops_test

import (
	"math"
	"testing"

	"github.com/born-ml/born/internal/autodiff/ops"
	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/tensor"
)

const epsilon = 1e-5

func TestErf_ForwardFloat64(t *testing.T) {
	backend := cpu.New()

	// Test cases for float64
	float64Tests := []struct {
		name  string
		a     []float64
		want  []float64
		shape tensor.Shape
	}{
		{"basic", []float64{-2.0, -1.0, 0.0, 1.0, 2.0}, []float64{-0.9953222650189527, -0.8427007929497148, 0.0, 0.8427007929497148, 0.9953222650189527}, tensor.Shape{5}},
		{"edges", []float64{math.Inf(-1), math.Inf(1), math.NaN()}, []float64{-1.0, 1.0, math.NaN()}, tensor.Shape{3}},
	}

	for _, tt := range float64Tests {
		t.Run(tt.name, func(t *testing.T) {
			input, _ := tensor.NewRaw(tt.shape, tensor.Float64, backend.Device())
			copy(input.AsFloat64(), tt.a)

			result := backend.Erf(input)

			if !result.Shape().Equal(tt.shape) {
				t.Errorf("Expected shape %v, got %v", tt.shape, result.Shape())
			}

			outputData := result.AsFloat64()
			for i, v := range outputData {
				if math.Abs(v-tt.want[i]) > epsilon {
					t.Errorf("erf(%f) = %f, want %f", tt.a[i], v, tt.want[i])
				}
			}
		})
	}
}

func TestErf_ForwardFloat32(t *testing.T) {
	backend := cpu.New()

	// test cases for float32
	float32Tests := []struct {
		name  string
		a     []float32
		want  []float32
		shape tensor.Shape
	}{
		{"basic", []float32{-2.0, -1.0, 0.0, 1.0, 2.0}, []float32{-0.9953222650189527, -0.8427007929497148, 0.0, 0.8427007929497148, 0.9953222650189527}, tensor.Shape{5}},
		{"edge cases", []float32{float32(math.Inf(-1)), float32(math.Inf(1)), float32(math.NaN())}, []float32{-1.0, 1.0, float32(math.NaN())}, tensor.Shape{3}},
	}

	for _, tt := range float32Tests {
		t.Run(tt.name, func(t *testing.T) {
			input, _ := tensor.NewRaw(tt.shape, tensor.Float32, backend.Device())
			copy(input.AsFloat32(), tt.a)

			result := backend.Erf(input)

			if !result.Shape().Equal(tt.shape) {
				t.Errorf("Expected shape %v, got %v", tt.shape, result.Shape())
			}

			outputData := result.AsFloat32()
			for i, v := range outputData {
				if math.Abs(float64(v-tt.want[i])) > epsilon {
					t.Errorf("erf(%f) = %f, want %f", tt.a[i], v, tt.want[i])
				}
			}
		})
	}
}

func TestErfOp_BackwardFloat64(t *testing.T) {
	backend := cpu.New()

	// Test cases for float64
	basicValues64 := []float64{-2.0, -1.0, 0.0, 1.0, 2.0}
	edgeValues64 := []float64{math.Inf(-1), math.Inf(1), math.NaN()}

	float64Tests := []struct {
		name       string
		a          []float64
		gradOutput []float64
		want       []float64
		shape      tensor.Shape
	}{
		{"basic grad 0", basicValues64, []float64{0.0, 0.0, 0.0, 0.0, 0.0},
			[]float64{0.0, 0.0, 0.0, 0.0, 0.0}, tensor.Shape{5}},
		{"basic grad 1", basicValues64, []float64{1.0, 1.0, 1.0, 1.0, 1.0},
			[]float64{0.020667, 0.415107, 1.128379, 0.415107, 0.020667}, tensor.Shape{5}},
		{"basic grad 2", basicValues64, []float64{2.0, 2.0, 2.0, 2.0, 2.0},
			[]float64{0.041334, 0.830214, 2.256758, 0.830214, 0.041334}, tensor.Shape{5}},
		{"edges grad 0", edgeValues64, []float64{0.0, 0.0, 0.0},
			[]float64{0.0, 0.0, math.NaN()}, tensor.Shape{3}},
		{"edges grad 1", edgeValues64, []float64{1.0, 1.0, 1.0},
			[]float64{0.0, 0.0, math.NaN()}, tensor.Shape{3}},
		{"edges grad 2", edgeValues64, []float64{2.0, 2.0, 2.0},
			[]float64{0.0, 0.0, math.NaN()}, tensor.Shape{3}},
	}

	for _, tt := range float64Tests {
		t.Run(tt.name, func(t *testing.T) {
			input, _ := tensor.NewRaw(tt.shape, tensor.Float64, backend.Device())
			copy(input.AsFloat64(), tt.a)

			resultRaw := backend.Erf(input)

			op := ops.NewErfOp(input, resultRaw)

			outputGrad, _ := tensor.NewRaw(tt.shape, tensor.Float64, backend.Device())
			copy(outputGrad.AsFloat64(), tt.gradOutput)

			inputGrads := op.Backward(outputGrad, backend)

			gradA := inputGrads[0]
			if gradA == nil {
				t.Fatalf("Expected gradient for input tensor, got nil")
			}

			if !gradA.Shape().Equal(tt.shape) {
				t.Errorf("Expected shape %v, got %v", tt.shape, gradA.Shape())
			}

			outputData := gradA.AsFloat64()
			for i, v := range outputData {
				if math.Abs(v-tt.want[i]) > epsilon {
					t.Errorf("grad_a(%f) = %f, want %f", tt.a[i], v, tt.want[i])
				}
			}
		})
	}
}

func TestErfOp_BackwardFloat32(t *testing.T) {
	backend := cpu.New()

	// Test cases for float32
	basicValues32 := []float32{-2.0, -1.0, 0.0, 1.0, 2.0}
	edgeValues32 := []float32{float32(math.Inf(-1)), float32(math.Inf(1)), float32(math.NaN())}

	float32Tests := []struct {
		name       string
		a          []float32
		gradOutput []float32
		want       []float32
		shape      tensor.Shape
	}{
		{"basic grad 0", basicValues32, []float32{0.0, 0.0, 0.0, 0.0, 0.0},
			[]float32{0.0, 0.0, 0.0, 0.0, 0.0}, tensor.Shape{5}},
		{"basic grad 1", basicValues32, []float32{1.0, 1.0, 1.0, 1.0, 1.0},
			[]float32{0.020667, 0.415107, 1.128379, 0.415107, 0.020667}, tensor.Shape{5}},
		{"basic grad 2", basicValues32, []float32{2.0, 2.0, 2.0, 2.0, 2.0},
			[]float32{0.041334, 0.830214, 2.256758, 0.830214, 0.041334}, tensor.Shape{5}},
		{"edges grad 0", edgeValues32, []float32{0.0, 0.0, 0.0},
			[]float32{0.0, 0.0, float32(math.NaN())}, tensor.Shape{3}},
		{"edges grad 1", edgeValues32, []float32{1.0, 1.0, 1.0},
			[]float32{0.0, 0.0, float32(math.NaN())}, tensor.Shape{3}},
		{"edges grad 2", edgeValues32, []float32{2.0, 2.0, 2.0},
			[]float32{0.0, 0.0, float32(math.NaN())}, tensor.Shape{3}},
	}

	for _, tt := range float32Tests {
		t.Run(tt.name, func(t *testing.T) {
			input, _ := tensor.NewRaw(tt.shape, tensor.Float32, backend.Device())
			copy(input.AsFloat32(), tt.a)

			resultRaw := backend.Erf(input)

			op := ops.NewErfOp(input, resultRaw)

			outputGrad, _ := tensor.NewRaw(tt.shape, tensor.Float32, backend.Device())
			copy(outputGrad.AsFloat32(), tt.gradOutput)

			inputGrads := op.Backward(outputGrad, backend)

			gradA := inputGrads[0]
			if gradA == nil {
				t.Fatalf("Expected gradient for input tensor, got nil")
			}

			if !gradA.Shape().Equal(tt.shape) {
				t.Errorf("Expected shape %v, got %v", tt.shape, gradA.Shape())
			}

			outputData := gradA.AsFloat32()
			for i, v := range outputData {
				if math.Abs(float64(v-tt.want[i])) > epsilon {
					t.Errorf("grad_a(%f) = %f, want %f", tt.a[i], v, tt.want[i])
				}
			}
		})
	}
}
