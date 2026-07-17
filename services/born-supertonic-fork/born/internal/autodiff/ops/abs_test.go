package ops_test

import (
	"math"
	"testing"

	"github.com/born-ml/born/internal/autodiff/ops"
	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/tensor"
)

func TestAbs_ForwardFloat64(t *testing.T) {
	backend := cpu.New()

	float64Tests := []struct {
		name  string
		a     []float64
		want  []float64
		shape tensor.Shape
	}{
		{"basic", []float64{-2.0, -1.0, 0.0, 1.0, 2.0}, []float64{2.0, 1.0, 0.0, 1.0, 2.0}, tensor.Shape{5}},
		{"edges", []float64{math.Inf(-1), math.Inf(1), math.NaN()}, []float64{math.Inf(1), math.Inf(1), math.NaN()}, tensor.Shape{3}},
		{"2d", []float64{-1.0, -2.0, 3.0, -4.0}, []float64{1.0, 2.0, 3.0, 4.0}, tensor.Shape{2, 2}},
	}

	for _, tt := range float64Tests {
		t.Run(tt.name, func(t *testing.T) {
			input, _ := tensor.NewRaw(tt.shape, tensor.Float64, backend.Device())
			copy(input.AsFloat64(), tt.a)

			result := backend.Abs(input)

			if !result.Shape().Equal(tt.shape) {
				t.Errorf("Expected shape %v, got %v", tt.shape, result.Shape())
			}

			outputData := result.AsFloat64()
			for i, v := range outputData {
				if math.IsNaN(tt.want[i]) {
					if !math.IsNaN(float64(v)) {
						t.Errorf("abs(NaN) = %v, want NaN", v)
					}
					continue
				}
				if math.Abs(v-tt.want[i]) > epsilon {
					t.Errorf("abs(%f) = %f, want %f", tt.a[i], v, tt.want[i])
				}
			}
		})
	}
}

func TestAbs_ForwardFloat32(t *testing.T) {
	backend := cpu.New()

	float32Tests := []struct {
		name  string
		a     []float32
		want  []float32
		shape tensor.Shape
	}{
		{"basic", []float32{-2.0, -1.0, 0.0, 1.0, 2.0}, []float32{2.0, 1.0, 0.0, 1.0, 2.0}, tensor.Shape{5}},
		{"edges", []float32{float32(math.Inf(-1)), float32(math.Inf(1)), float32(math.NaN())}, []float32{float32(math.Inf(1)), float32(math.Inf(1)), float32(math.NaN())}, tensor.Shape{3}},
		{"2d", []float32{-1.0, -2.0, 3.0, -4.0}, []float32{1.0, 2.0, 3.0, 4.0}, tensor.Shape{2, 2}},
	}

	for _, tt := range float32Tests {
		t.Run(tt.name, func(t *testing.T) {
			input, _ := tensor.NewRaw(tt.shape, tensor.Float32, backend.Device())
			copy(input.AsFloat32(), tt.a)

			result := backend.Abs(input)

			if !result.Shape().Equal(tt.shape) {
				t.Errorf("Expected shape %v, got %v", tt.shape, result.Shape())
			}

			outputData := result.AsFloat32()
			for i, v := range outputData {
				if math.IsNaN(float64(tt.want[i])) {
					if !math.IsNaN(float64(v)) {
						t.Errorf("abs(NaN) = %v, want NaN", v)
					}
					continue
				}
				if math.Abs(float64(v-tt.want[i])) > epsilon {
					t.Errorf("abs(%f) = %f, want %f", tt.a[i], v, tt.want[i])
				}
			}
		})
	}
}

func TestAbs_ForwardInt32(t *testing.T) {
	backend := cpu.New()

	int32Tests := []struct {
		name  string
		a     []int32
		want  []int32
		shape tensor.Shape
	}{
		{"basic", []int32{-2, -1, 0, 1, 2}, []int32{2, 1, 0, 1, 2}, tensor.Shape{5}},
		{"edges", []int32{math.MinInt32, math.MaxInt32}, []int32{math.MinInt32, math.MaxInt32}, tensor.Shape{2}},
		{"zero", []int32{0, 0, 0}, []int32{0, 0, 0}, tensor.Shape{3}},
		{"2d", []int32{-1, -2, 3, -4}, []int32{1, 2, 3, 4}, tensor.Shape{2, 2}},
	}

	for _, tt := range int32Tests {
		t.Run(tt.name, func(t *testing.T) {
			input, _ := tensor.NewRaw(tt.shape, tensor.Int32, backend.Device())
			copy(input.AsInt32(), tt.a)

			result := backend.Abs(input)

			if !result.Shape().Equal(tt.shape) {
				t.Errorf("Expected shape %v, got %v", tt.shape, result.Shape())
			}

			outputData := result.AsInt32()
			for i, v := range outputData {
				if v != tt.want[i] {
					t.Errorf("abs(%d) = %d, want %d", tt.a[i], v, tt.want[i])
				}
			}
		})
	}
}

func TestAbs_ForwardInt64(t *testing.T) {
	backend := cpu.New()

	int64Tests := []struct {
		name  string
		a     []int64
		want  []int64
		shape tensor.Shape
	}{
		{"basic", []int64{-2, -1, 0, 1, 2}, []int64{2, 1, 0, 1, 2}, tensor.Shape{5}},
		{"edges", []int64{math.MinInt64, math.MaxInt64}, []int64{math.MinInt64, math.MaxInt64}, tensor.Shape{2}},
		{"zero", []int64{0, 0, 0}, []int64{0, 0, 0}, tensor.Shape{3}},
		{"2d", []int64{-1, -2, 3, -4}, []int64{1, 2, 3, 4}, tensor.Shape{2, 2}},
	}

	for _, tt := range int64Tests {
		t.Run(tt.name, func(t *testing.T) {
			input, _ := tensor.NewRaw(tt.shape, tensor.Int64, backend.Device())
			copy(input.AsInt64(), tt.a)

			result := backend.Abs(input)

			if !result.Shape().Equal(tt.shape) {
				t.Errorf("Expected shape %v, got %v", tt.shape, result.Shape())
			}

			outputData := result.AsInt64()
			for i, v := range outputData {
				if v != tt.want[i] {
					t.Errorf("abs(%d) = %d, want %d", tt.a[i], v, tt.want[i])
				}
			}
		})
	}
}

func TestAbs_ForwardUint8(t *testing.T) {
	backend := cpu.New()

	uint8Tests := []struct {
		name  string
		a     []uint8
		want  []uint8
		shape tensor.Shape
	}{
		{"basic", []uint8{0, 1, 2, 3, 4}, []uint8{0, 1, 2, 3, 4}, tensor.Shape{5}},
		{"zero", []uint8{0, 0, 0}, []uint8{0, 0, 0}, tensor.Shape{3}},
		{"2d", []uint8{1, 2, 3, 4}, []uint8{1, 2, 3, 4}, tensor.Shape{2, 2}},
	}

	for _, tt := range uint8Tests {
		t.Run(tt.name, func(t *testing.T) {
			input, _ := tensor.NewRaw(tt.shape, tensor.Uint8, backend.Device())
			copy(input.AsUint8(), tt.a)

			result := backend.Abs(input)

			if !result.Shape().Equal(tt.shape) {
				t.Errorf("Expected shape %v, got %v", tt.shape, result.Shape())
			}

			outputData := result.AsUint8()
			for i, v := range outputData {
				if v != tt.want[i] {
					t.Errorf("abs(%d) = %d, want %d", tt.a[i], v, tt.want[i])
				}
			}
		})
	}
}

func TestAbsOp_BackwardFloat64(t *testing.T) {
	backend := cpu.New()

	basicValues := []float64{-2.0, -1.0, 0.0, 1.0, 2.0}
	positiveValues := []float64{1.0, 2.0, 3.0}
	negativeValues := []float64{-1.0, -2.0, -3.0}
	edgeValues := []float64{math.Inf(-1), math.Inf(1), math.NaN()}

	float64Tests := []struct {
		name       string
		a          []float64
		gradOutput []float64
		want       []float64
		shape      tensor.Shape
	}{
		{"basic grad 0", basicValues, []float64{0.0, 0.0, 0.0, 0.0, 0.0},
			[]float64{0.0, 0.0, 0.0, 0.0, 0.0}, tensor.Shape{5}},
		{"basic grad 1", basicValues, []float64{1.0, 1.0, 1.0, 1.0, 1.0},
			[]float64{-1.0, -1.0, 0.0, 1.0, 1.0}, tensor.Shape{5}},
		{"basic grad 2", basicValues, []float64{2.0, 2.0, 2.0, 2.0, 2.0},
			[]float64{-2.0, -2.0, 0.0, 2.0, 2.0}, tensor.Shape{5}},
		{"positive grad 1", positiveValues, []float64{1.0, 1.0, 1.0},
			[]float64{1.0, 1.0, 1.0}, tensor.Shape{3}},
		{"negative grad 1", negativeValues, []float64{1.0, 1.0, 1.0},
			[]float64{-1.0, -1.0, -1.0}, tensor.Shape{3}},
		{"edges grad 0", edgeValues, []float64{0.0, 0.0, 0.0},
			[]float64{0.0, 0.0, math.NaN()}, tensor.Shape{3}},
		{"edges grad 1", edgeValues, []float64{1.0, 1.0, 1.0},
			[]float64{-1.0, 1.0, math.NaN()}, tensor.Shape{3}},
	}

	for _, tt := range float64Tests {
		t.Run(tt.name, func(t *testing.T) {
			input, _ := tensor.NewRaw(tt.shape, tensor.Float64, backend.Device())
			copy(input.AsFloat64(), tt.a)

			resultRaw := backend.Abs(input)

			op := ops.NewAbsOp(input, resultRaw)

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
				if math.IsNaN(tt.want[i]) {
					if !math.IsNaN(v) {
						t.Errorf("grad_a(%f) = %f, want NaN", tt.a[i], v)
					}
				} else if math.Abs(v-tt.want[i]) > epsilon {
					t.Errorf("grad_a(%f) = %f, want %f", tt.a[i], v, tt.want[i])
				}
			}
		})
	}
}

func TestAbsOp_BackwardFloat32(t *testing.T) {
	backend := cpu.New()

	basicValues := []float32{-2.0, -1.0, 0.0, 1.0, 2.0}
	positiveValues := []float32{1.0, 2.0, 3.0}
	negativeValues := []float32{-1.0, -2.0, -3.0}
	edgeValues := []float32{float32(math.Inf(-1)), float32(math.Inf(1)), float32(math.NaN())}

	float32Tests := []struct {
		name       string
		a          []float32
		gradOutput []float32
		want       []float32
		shape      tensor.Shape
	}{
		{"basic grad 0", basicValues, []float32{0.0, 0.0, 0.0, 0.0, 0.0},
			[]float32{0.0, 0.0, 0.0, 0.0, 0.0}, tensor.Shape{5}},
		{"basic grad 1", basicValues, []float32{1.0, 1.0, 1.0, 1.0, 1.0},
			[]float32{-1.0, -1.0, 0.0, 1.0, 1.0}, tensor.Shape{5}},
		{"basic grad 2", basicValues, []float32{2.0, 2.0, 2.0, 2.0, 2.0},
			[]float32{-2.0, -2.0, 0.0, 2.0, 2.0}, tensor.Shape{5}},
		{"positive grad 1", positiveValues, []float32{1.0, 1.0, 1.0},
			[]float32{1.0, 1.0, 1.0}, tensor.Shape{3}},
		{"negative grad 1", negativeValues, []float32{1.0, 1.0, 1.0},
			[]float32{-1.0, -1.0, -1.0}, tensor.Shape{3}},
		{"edges grad 0", edgeValues, []float32{0.0, 0.0, 0.0},
			[]float32{0.0, 0.0, float32(math.NaN())}, tensor.Shape{3}},
		{"edges grad 1", edgeValues, []float32{1.0, 1.0, 1.0},
			[]float32{-1.0, 1.0, float32(math.NaN())}, tensor.Shape{3}},
	}

	for _, tt := range float32Tests {
		t.Run(tt.name, func(t *testing.T) {
			input, _ := tensor.NewRaw(tt.shape, tensor.Float32, backend.Device())
			copy(input.AsFloat32(), tt.a)

			resultRaw := backend.Abs(input)

			op := ops.NewAbsOp(input, resultRaw)

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
				if math.IsNaN(float64(tt.want[i])) {
					if !math.IsNaN(float64(v)) {
						t.Errorf("grad_a(%f) = %f, want NaN", tt.a[i], v)
					}
				} else if math.Abs(float64(v-tt.want[i])) > epsilon {
					t.Errorf("grad_a(%f) = %f, want %f", tt.a[i], v, tt.want[i])
				}
			}
		})
	}
}

func TestAbsOp_BackwardInt32(t *testing.T) {
	backend := cpu.New()

	basicValues := []int32{-2, -1, 0, 1, 2}
	positiveValues := []int32{1, 2, 3}
	negativeValues := []int32{-1, -2, -3}

	int32Tests := []struct {
		name       string
		a          []int32
		gradOutput []int32
		want       []int32
		shape      tensor.Shape
	}{
		{"basic grad 0", basicValues, []int32{0, 0, 0, 0, 0},
			[]int32{0, 0, 0, 0, 0}, tensor.Shape{5}},
		{"basic grad 1", basicValues, []int32{1, 1, 1, 1, 1},
			[]int32{-1, -1, 0, 1, 1}, tensor.Shape{5}},
		{"basic grad 2", basicValues, []int32{2, 2, 2, 2, 2},
			[]int32{-2, -2, 0, 2, 2}, tensor.Shape{5}},
		{"positive grad 1", positiveValues, []int32{1, 1, 1},
			[]int32{1, 1, 1}, tensor.Shape{3}},
		{"negative grad 1", negativeValues, []int32{1, 1, 1},
			[]int32{-1, -1, -1}, tensor.Shape{3}},
		{"zero grad 1", []int32{0, 0, 0}, []int32{1, 1, 1},
			[]int32{0, 0, 0}, tensor.Shape{3}},
	}

	for _, tt := range int32Tests {
		t.Run(tt.name, func(t *testing.T) {
			input, _ := tensor.NewRaw(tt.shape, tensor.Int32, backend.Device())
			copy(input.AsInt32(), tt.a)

			resultRaw := backend.Abs(input)

			op := ops.NewAbsOp(input, resultRaw)

			outputGrad, _ := tensor.NewRaw(tt.shape, tensor.Int32, backend.Device())
			copy(outputGrad.AsInt32(), tt.gradOutput)

			inputGrads := op.Backward(outputGrad, backend)

			gradA := inputGrads[0]
			if gradA == nil {
				t.Fatalf("Expected gradient for input tensor, got nil")
			}

			if !gradA.Shape().Equal(tt.shape) {
				t.Errorf("Expected shape %v, got %v", tt.shape, gradA.Shape())
			}

			outputData := gradA.AsInt32()
			for i, v := range outputData {
				if v != tt.want[i] {
					t.Errorf("grad_a(%d) = %d, want %d", tt.a[i], v, tt.want[i])
				}
			}
		})
	}
}

func TestAbsOp_BackwardInt64(t *testing.T) {
	backend := cpu.New()

	basicValues := []int64{-2, -1, 0, 1, 2}
	positiveValues := []int64{1, 2, 3}
	negativeValues := []int64{-1, -2, -3}

	int64Tests := []struct {
		name       string
		a          []int64
		gradOutput []int64
		want       []int64
		shape      tensor.Shape
	}{
		{"basic grad 0", basicValues, []int64{0, 0, 0, 0, 0},
			[]int64{0, 0, 0, 0, 0}, tensor.Shape{5}},
		{"basic grad 1", basicValues, []int64{1, 1, 1, 1, 1},
			[]int64{-1, -1, 0, 1, 1}, tensor.Shape{5}},
		{"basic grad 2", basicValues, []int64{2, 2, 2, 2, 2},
			[]int64{-2, -2, 0, 2, 2}, tensor.Shape{5}},
		{"positive grad 1", positiveValues, []int64{1, 1, 1},
			[]int64{1, 1, 1}, tensor.Shape{3}},
		{"negative grad 1", negativeValues, []int64{1, 1, 1},
			[]int64{-1, -1, -1}, tensor.Shape{3}},
		{"zero grad 1", []int64{0, 0, 0}, []int64{1, 1, 1},
			[]int64{0, 0, 0}, tensor.Shape{3}},
	}

	for _, tt := range int64Tests {
		t.Run(tt.name, func(t *testing.T) {
			input, _ := tensor.NewRaw(tt.shape, tensor.Int64, backend.Device())
			copy(input.AsInt64(), tt.a)

			resultRaw := backend.Abs(input)

			op := ops.NewAbsOp(input, resultRaw)

			outputGrad, _ := tensor.NewRaw(tt.shape, tensor.Int64, backend.Device())
			copy(outputGrad.AsInt64(), tt.gradOutput)

			inputGrads := op.Backward(outputGrad, backend)

			gradA := inputGrads[0]
			if gradA == nil {
				t.Fatalf("Expected gradient for input tensor, got nil")
			}

			if !gradA.Shape().Equal(tt.shape) {
				t.Errorf("Expected shape %v, got %v", tt.shape, gradA.Shape())
			}

			outputData := gradA.AsInt64()
			for i, v := range outputData {
				if v != tt.want[i] {
					t.Errorf("grad_a(%d) = %d, want %d", tt.a[i], v, tt.want[i])
				}
			}
		})
	}
}
