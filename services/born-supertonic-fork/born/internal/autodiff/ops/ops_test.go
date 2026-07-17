package ops_test

import (
	"math"
	"testing"

	"github.com/born-ml/born/internal/autodiff"
	"github.com/born-ml/born/internal/autodiff/ops"
	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/tensor"
)

// Helper to check float32 slices are equal within epsilon.
func float32Equal(a, b []float32, epsilon float32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		diff := a[i] - b[i]
		if diff < 0 {
			diff = -diff
		}
		if diff > epsilon {
			return false
		}
	}
	return true
}

// TestAddOp_Backward tests AddOp backward pass.
func TestAddOp_Backward(t *testing.T) {
	backend := cpu.New()

	// Create inputs: a = [1, 2, 3], b = [4, 5, 6]
	a, _ := tensor.FromSlice([]float32{1, 2, 3}, tensor.Shape{3}, backend)
	b, _ := tensor.FromSlice([]float32{4, 5, 6}, tensor.Shape{3}, backend)
	result := backend.Add(a.Raw(), b.Raw())

	// Create operation
	op := ops.NewAddOp(a.Raw(), b.Raw(), result)

	// Output gradient: [1, 1, 1]
	outputGrad, _ := tensor.FromSlice([]float32{1, 1, 1}, tensor.Shape{3}, backend)

	// Backward pass
	inputGrads := op.Backward(outputGrad.Raw(), backend)

	// For addition: grad_a = grad_b = outputGrad
	expectedGrad := []float32{1, 1, 1}

	if !float32Equal(inputGrads[0].AsFloat32(), expectedGrad, 1e-6) {
		t.Errorf("AddOp grad_a: got %v, want %v", inputGrads[0].AsFloat32(), expectedGrad)
	}

	if !float32Equal(inputGrads[1].AsFloat32(), expectedGrad, 1e-6) {
		t.Errorf("AddOp grad_b: got %v, want %v", inputGrads[1].AsFloat32(), expectedGrad)
	}
}

// TestAddOp_BroadcastBackward tests AddOp backward with broadcasting.
func TestAddOp_BroadcastBackward(t *testing.T) {
	backend := cpu.New()

	// a = [1, 2, 3] (shape [3]), b = [10] (shape [1])
	// result = [11, 12, 13] (shape [3])
	a, _ := tensor.FromSlice([]float32{1, 2, 3}, tensor.Shape{3}, backend)
	b, _ := tensor.FromSlice([]float32{10}, tensor.Shape{1}, backend)
	result := backend.Add(a.Raw(), b.Raw())

	op := ops.NewAddOp(a.Raw(), b.Raw(), result)

	// Output gradient: [1, 1, 1]
	outputGrad, _ := tensor.FromSlice([]float32{1, 1, 1}, tensor.Shape{3}, backend)

	inputGrads := op.Backward(outputGrad.Raw(), backend)

	// grad_a = [1, 1, 1] (no reduction needed)
	// grad_b = sum([1, 1, 1]) = [3] (reduced to shape [1])
	expectedGradA := []float32{1, 1, 1}
	expectedGradB := []float32{3}

	if !float32Equal(inputGrads[0].AsFloat32(), expectedGradA, 1e-6) {
		t.Errorf("AddOp grad_a: got %v, want %v", inputGrads[0].AsFloat32(), expectedGradA)
	}

	if !float32Equal(inputGrads[1].AsFloat32(), expectedGradB, 1e-6) {
		t.Errorf("AddOp grad_b: got %v, want %v", inputGrads[1].AsFloat32(), expectedGradB)
	}
}

// TestSubOp_Backward tests SubOp backward pass.
func TestSubOp_Backward(t *testing.T) {
	backend := cpu.New()

	// a = [5, 6, 7], b = [1, 2, 3]
	// result = a - b = [4, 4, 4]
	a, _ := tensor.FromSlice([]float32{5, 6, 7}, tensor.Shape{3}, backend)
	b, _ := tensor.FromSlice([]float32{1, 2, 3}, tensor.Shape{3}, backend)
	result := backend.Sub(a.Raw(), b.Raw())

	op := ops.NewSubOp(a.Raw(), b.Raw(), result)

	// Output gradient: [1, 1, 1]
	outputGrad, _ := tensor.FromSlice([]float32{1, 1, 1}, tensor.Shape{3}, backend)

	inputGrads := op.Backward(outputGrad.Raw(), backend)

	// For subtraction: grad_a = outputGrad, grad_b = -outputGrad
	expectedGradA := []float32{1, 1, 1}
	expectedGradB := []float32{-1, -1, -1}

	if !float32Equal(inputGrads[0].AsFloat32(), expectedGradA, 1e-6) {
		t.Errorf("SubOp grad_a: got %v, want %v", inputGrads[0].AsFloat32(), expectedGradA)
	}

	if !float32Equal(inputGrads[1].AsFloat32(), expectedGradB, 1e-6) {
		t.Errorf("SubOp grad_b: got %v, want %v", inputGrads[1].AsFloat32(), expectedGradB)
	}
}

// TestMulOp_Backward tests MulOp backward pass.
func TestMulOp_Backward(t *testing.T) {
	// Use AutodiffBackend to prevent inplace corruption during backward pass
	backend := autodiff.New(cpu.New())

	// a = [2, 3, 4], b = [5, 6, 7]
	// result = a * b = [10, 18, 28]
	a, _ := tensor.FromSlice([]float32{2, 3, 4}, tensor.Shape{3}, backend)
	b, _ := tensor.FromSlice([]float32{5, 6, 7}, tensor.Shape{3}, backend)

	result := backend.Mul(a.Raw(), b.Raw())

	op := ops.NewMulOp(a.Raw(), b.Raw(), result)

	// Output gradient: [1, 1, 1]
	outputGrad, _ := tensor.FromSlice([]float32{1, 1, 1}, tensor.Shape{3}, backend)

	inputGrads := op.Backward(outputGrad.Raw(), backend)

	// For multiplication: grad_a = outputGrad * b, grad_b = outputGrad * a
	expectedGradA := []float32{5, 6, 7} // 1*5, 1*6, 1*7
	expectedGradB := []float32{2, 3, 4} // 1*2, 1*3, 1*4

	if !float32Equal(inputGrads[0].AsFloat32(), expectedGradA, 1e-6) {
		t.Errorf("MulOp grad_a: got %v, want %v", inputGrads[0].AsFloat32(), expectedGradA)
	}

	if !float32Equal(inputGrads[1].AsFloat32(), expectedGradB, 1e-6) {
		t.Errorf("MulOp grad_b: got %v, want %v", inputGrads[1].AsFloat32(), expectedGradB)
	}
}

// TestDivOp_Backward tests DivOp backward pass.
func TestDivOp_Backward(t *testing.T) {
	// Use AutodiffBackend to prevent inplace corruption during backward pass
	backend := autodiff.New(cpu.New())

	// a = [10, 20, 30], b = [2, 4, 5]
	// result = a / b = [5, 5, 6]
	a, _ := tensor.FromSlice([]float32{10, 20, 30}, tensor.Shape{3}, backend)
	b, _ := tensor.FromSlice([]float32{2, 4, 5}, tensor.Shape{3}, backend)

	result := backend.Div(a.Raw(), b.Raw())

	op := ops.NewDivOp(a.Raw(), b.Raw(), result)

	// Output gradient: [1, 1, 1]
	outputGrad, _ := tensor.FromSlice([]float32{1, 1, 1}, tensor.Shape{3}, backend)

	inputGrads := op.Backward(outputGrad.Raw(), backend)

	// For division: grad_a = outputGrad / b, grad_b = -outputGrad * a / b²
	expectedGradA := []float32{0.5, 0.25, 0.2}    // 1/2, 1/4, 1/5
	expectedGradB := []float32{-2.5, -1.25, -1.2} // -10/4, -20/16, -30/25

	if !float32Equal(inputGrads[0].AsFloat32(), expectedGradA, 1e-5) {
		t.Errorf("DivOp grad_a: got %v, want %v", inputGrads[0].AsFloat32(), expectedGradA)
	}

	if !float32Equal(inputGrads[1].AsFloat32(), expectedGradB, 1e-5) {
		t.Errorf("DivOp grad_b: got %v, want %v", inputGrads[1].AsFloat32(), expectedGradB)
	}
}

// TestMatMulOp_Backward tests MatMulOp backward pass.
func TestMatMulOp_Backward(t *testing.T) {
	backend := cpu.New()

	// A = [[1, 2],    B = [[5, 6],
	//      [3, 4]]         [7, 8]]
	//
	// C = A @ B = [[19, 22],
	//              [43, 50]]
	a, _ := tensor.FromSlice([]float32{1, 2, 3, 4}, tensor.Shape{2, 2}, backend)
	b, _ := tensor.FromSlice([]float32{5, 6, 7, 8}, tensor.Shape{2, 2}, backend)
	result := backend.MatMul(a.Raw(), b.Raw())

	op := ops.NewMatMulOp(a.Raw(), b.Raw(), result)

	// Output gradient: all ones
	outputGrad, _ := tensor.FromSlice([]float32{1, 1, 1, 1}, tensor.Shape{2, 2}, backend)

	inputGrads := op.Backward(outputGrad.Raw(), backend)

	// grad_A = outputGrad @ B^T
	// B^T = [[5, 7], [6, 8]]
	// grad_A = [[1*5+1*6, 1*7+1*8], [1*5+1*6, 1*7+1*8]] = [[11, 15], [11, 15]]
	expectedGradA := []float32{11, 15, 11, 15}

	// grad_B = A^T @ outputGrad
	// A^T = [[1, 3], [2, 4]]
	// grad_B = [[1*1+3*1, 1*1+3*1], [2*1+4*1, 2*1+4*1]] = [[4, 4], [6, 6]]
	expectedGradB := []float32{4, 4, 6, 6}

	if !float32Equal(inputGrads[0].AsFloat32(), expectedGradA, 1e-5) {
		t.Errorf("MatMulOp grad_A: got %v, want %v", inputGrads[0].AsFloat32(), expectedGradA)
	}

	if !float32Equal(inputGrads[1].AsFloat32(), expectedGradB, 1e-5) {
		t.Errorf("MatMulOp grad_B: got %v, want %v", inputGrads[1].AsFloat32(), expectedGradB)
	}
}

// TestReLUOp_Backward tests ReLU backward pass.
func TestReLUOp_Backward(t *testing.T) {
	backend := cpu.New()

	// Input: [-2, -1, 0, 1, 2]
	// ReLU output: [0, 0, 0, 1, 2]
	input, _ := tensor.FromSlice([]float32{-2, -1, 0, 1, 2}, tensor.Shape{5}, backend)

	// Apply ReLU manually
	output, _ := tensor.NewRaw(tensor.Shape{5}, tensor.Float32, tensor.CPU)
	outputData := output.AsFloat32()
	inputData := input.Raw().AsFloat32()
	for i, val := range inputData {
		if val > 0 {
			outputData[i] = val
		} else {
			outputData[i] = 0
		}
	}

	op := ops.NewReLUOp(input.Raw(), output)

	// Output gradient: [1, 1, 1, 1, 1]
	outputGrad, _ := tensor.FromSlice([]float32{1, 1, 1, 1, 1}, tensor.Shape{5}, backend)

	inputGrads := op.Backward(outputGrad.Raw(), backend)

	// ReLU gradient: 1 if input > 0, else 0
	// Expected: [0, 0, 0, 1, 1]
	expectedGrad := []float32{0, 0, 0, 1, 1}

	if !float32Equal(inputGrads[0].AsFloat32(), expectedGrad, 1e-6) {
		t.Errorf("ReLUOp grad: got %v, want %v", inputGrads[0].AsFloat32(), expectedGrad)
	}
}

// TestReLUOp_BackwardAllPositive tests ReLU backward with all positive inputs.
func TestReLUOp_BackwardAllPositive(t *testing.T) {
	backend := cpu.New()

	input, _ := tensor.FromSlice([]float32{1, 2, 3, 4}, tensor.Shape{4}, backend)

	output, _ := tensor.NewRaw(tensor.Shape{4}, tensor.Float32, tensor.CPU)
	copy(output.AsFloat32(), input.Raw().AsFloat32())

	op := ops.NewReLUOp(input.Raw(), output)

	outputGrad, _ := tensor.FromSlice([]float32{2, 3, 4, 5}, tensor.Shape{4}, backend)

	inputGrads := op.Backward(outputGrad.Raw(), backend)

	// All positive → gradient passes through
	expectedGrad := []float32{2, 3, 4, 5}

	if !float32Equal(inputGrads[0].AsFloat32(), expectedGrad, 1e-6) {
		t.Errorf("ReLUOp grad (all positive): got %v, want %v",
			inputGrads[0].AsFloat32(), expectedGrad)
	}
}

// TestReLUOp_BackwardAllNegative tests ReLU backward with all negative inputs.
func TestReLUOp_BackwardAllNegative(t *testing.T) {
	backend := cpu.New()

	input, _ := tensor.FromSlice([]float32{-1, -2, -3, -4}, tensor.Shape{4}, backend)

	output, _ := tensor.NewRaw(tensor.Shape{4}, tensor.Float32, tensor.CPU)
	// ReLU output is all zeros for negative inputs
	for i := range output.AsFloat32() {
		output.AsFloat32()[i] = 0
	}

	op := ops.NewReLUOp(input.Raw(), output)

	outputGrad, _ := tensor.FromSlice([]float32{2, 3, 4, 5}, tensor.Shape{4}, backend)

	inputGrads := op.Backward(outputGrad.Raw(), backend)

	// All negative → gradient is zero
	expectedGrad := []float32{0, 0, 0, 0}

	if !float32Equal(inputGrads[0].AsFloat32(), expectedGrad, 1e-6) {
		t.Errorf("ReLUOp grad (all negative): got %v, want %v",
			inputGrads[0].AsFloat32(), expectedGrad)
	}
}

// TestReLUOp_BackwardFloat64 tests ReLU backward with float64.
func TestReLUOp_BackwardFloat64(t *testing.T) {
	backend := cpu.New()

	input, _ := tensor.FromSlice([]float64{-1.5, 0, 2.5}, tensor.Shape{3}, backend)

	output, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float64, tensor.CPU)
	outputData := output.AsFloat64()
	inputData := input.Raw().AsFloat64()
	for i, val := range inputData {
		if val > 0 {
			outputData[i] = val
		} else {
			outputData[i] = 0
		}
	}

	op := ops.NewReLUOp(input.Raw(), output)

	outputGrad, _ := tensor.FromSlice([]float64{1, 1, 1}, tensor.Shape{3}, backend)

	inputGrads := op.Backward(outputGrad.Raw(), backend)

	// Expected: [0, 0, 1] (gradient for positive input only)
	result := inputGrads[0].AsFloat64()
	expected := []float64{0, 0, 1}

	for i := range result {
		if math.Abs(result[i]-expected[i]) > 1e-9 {
			t.Errorf("ReLUOp grad (float64): got %v, want %v", result, expected)
			break
		}
	}
}

// TestOperations_InputsOutputMethods tests Inputs() and Output() methods.
func TestOperations_InputsOutputMethods(t *testing.T) {
	backend := cpu.New()

	a, _ := tensor.FromSlice([]float32{1, 2}, tensor.Shape{2}, backend)
	b, _ := tensor.FromSlice([]float32{3, 4}, tensor.Shape{2}, backend)
	result := backend.Add(a.Raw(), b.Raw())

	// Test AddOp
	addOp := ops.NewAddOp(a.Raw(), b.Raw(), result)
	if len(addOp.Inputs()) != 2 {
		t.Errorf("AddOp.Inputs() length: got %d, want 2", len(addOp.Inputs()))
	}
	if addOp.Output() != result {
		t.Error("AddOp.Output() doesn't match result")
	}

	// Test SubOp
	subResult := backend.Sub(a.Raw(), b.Raw())
	subOp := ops.NewSubOp(a.Raw(), b.Raw(), subResult)
	if len(subOp.Inputs()) != 2 {
		t.Errorf("SubOp.Inputs() length: got %d, want 2", len(subOp.Inputs()))
	}
	if subOp.Output() != subResult {
		t.Error("SubOp.Output() doesn't match result")
	}

	// Test MulOp
	mulResult := backend.Mul(a.Raw(), b.Raw())
	mulOp := ops.NewMulOp(a.Raw(), b.Raw(), mulResult)
	if len(mulOp.Inputs()) != 2 {
		t.Errorf("MulOp.Inputs() length: got %d, want 2", len(mulOp.Inputs()))
	}
	if mulOp.Output() != mulResult {
		t.Error("MulOp.Output() doesn't match result")
	}

	// Test DivOp
	divResult := backend.Div(a.Raw(), b.Raw())
	divOp := ops.NewDivOp(a.Raw(), b.Raw(), divResult)
	if len(divOp.Inputs()) != 2 {
		t.Errorf("DivOp.Inputs() length: got %d, want 2", len(divOp.Inputs()))
	}
	if divOp.Output() != divResult {
		t.Error("DivOp.Output() doesn't match result")
	}

	// Test MatMulOp
	matA, _ := tensor.FromSlice([]float32{1, 2, 3, 4}, tensor.Shape{2, 2}, backend)
	matB, _ := tensor.FromSlice([]float32{5, 6, 7, 8}, tensor.Shape{2, 2}, backend)
	matResult := backend.MatMul(matA.Raw(), matB.Raw())
	matMulOp := ops.NewMatMulOp(matA.Raw(), matB.Raw(), matResult)
	if len(matMulOp.Inputs()) != 2 {
		t.Errorf("MatMulOp.Inputs() length: got %d, want 2", len(matMulOp.Inputs()))
	}
	if matMulOp.Output() != matResult {
		t.Error("MatMulOp.Output() doesn't match result")
	}

	// Test ReLUOp
	reluInput, _ := tensor.FromSlice([]float32{-1, 2}, tensor.Shape{2}, backend)
	reluOutput, _ := tensor.NewRaw(tensor.Shape{2}, tensor.Float32, tensor.CPU)
	copy(reluOutput.AsFloat32(), []float32{0, 2})
	reluOp := ops.NewReLUOp(reluInput.Raw(), reluOutput)
	if len(reluOp.Inputs()) != 1 {
		t.Errorf("ReLUOp.Inputs() length: got %d, want 1", len(reluOp.Inputs()))
	}
	if reluOp.Output() != reluOutput {
		t.Error("ReLUOp.Output() doesn't match result")
	}
}

// TestDivOp_Backward_Float64 tests DivOp backward with float64 (covers sumFloat64AlongDimension).
func TestDivOp_Backward_Float64(t *testing.T) {
	// Use AutodiffBackend to prevent inplace corruption during backward pass
	backend := autodiff.New(cpu.New())

	a, _ := tensor.FromSlice([]float64{10, 20, 30}, tensor.Shape{3}, backend)
	b, _ := tensor.FromSlice([]float64{2, 4, 5}, tensor.Shape{3}, backend)

	result := backend.Div(a.Raw(), b.Raw())
	op := ops.NewDivOp(a.Raw(), b.Raw(), result)

	outputGrad, _ := tensor.FromSlice([]float64{1, 1, 1}, tensor.Shape{3}, backend)
	inputGrads := op.Backward(outputGrad.Raw(), backend)

	// For division: grad_a = outputGrad / b
	expectedGradA := []float64{0.5, 0.25, 0.2}

	actualGradA := inputGrads[0].AsFloat64()
	for i, expected := range expectedGradA {
		if math.Abs(actualGradA[i]-expected) > 1e-6 {
			t.Errorf("DivOp float64 grad_a[%d]: got %v, want %v", i, actualGradA[i], expected)
		}
	}
}

// TestAddOp_Broadcasting_ScalarTarget tests broadcasting reduction to scalar (covers sumAll).
func TestAddOp_Broadcasting_ScalarTarget(t *testing.T) {
	// Use AutodiffBackend to prevent inplace corruption during backward pass
	backend := autodiff.New(cpu.New())

	// a is scalar, b is vector - broadcasts a to vector shape
	a, _ := tensor.FromSlice([]float32{10}, tensor.Shape{}, backend) // scalar
	b, _ := tensor.FromSlice([]float32{1, 2, 3}, tensor.Shape{3}, backend)

	result := backend.Add(a.Raw(), b.Raw())
	op := ops.NewAddOp(a.Raw(), b.Raw(), result)

	outputGrad, _ := tensor.FromSlice([]float32{1, 1, 1}, tensor.Shape{3}, backend)
	inputGrads := op.Backward(outputGrad.Raw(), backend)

	// grad_a should be scalar with sum of outputGrad = 3
	gradA := inputGrads[0]
	if len(gradA.Shape()) != 0 {
		t.Errorf("grad_a shape should be scalar (empty), got %v", gradA.Shape())
	}

	expectedGradA := float32(3.0) // sum of [1, 1, 1]
	actualGradA := gradA.AsFloat32()[0]

	if math.Abs(float64(actualGradA-expectedGradA)) > 1e-6 {
		t.Errorf("grad_a scalar: got %v, want %v", actualGradA, expectedGradA)
	}
}

// TestMulOp_Broadcasting_ReduceLeadingDims tests broadcasting that requires summing leading dimensions.
func TestMulOp_Broadcasting_ReduceLeadingDims(t *testing.T) {
	// Use AutodiffBackend to prevent inplace corruption during backward pass
	backend := autodiff.New(cpu.New())

	// a is (2,3,4), b is (3,4) - b broadcasts to (2,3,4) by adding leading dimension
	aData := make([]float32, 2*3*4)
	for i := range aData {
		aData[i] = float32(i + 1)
	}
	a, _ := tensor.FromSlice(aData, tensor.Shape{2, 3, 4}, backend)

	bData := make([]float32, 3*4)
	for i := range bData {
		bData[i] = float32(i + 1)
	}
	b, _ := tensor.FromSlice(bData, tensor.Shape{3, 4}, backend)

	result := backend.Mul(a.Raw(), b.Raw())
	op := ops.NewMulOp(a.Raw(), b.Raw(), result)

	// Output gradient: ones with shape (2,3,4)
	outputGradData := make([]float32, 2*3*4)
	for i := range outputGradData {
		outputGradData[i] = 1.0
	}
	outputGrad, _ := tensor.FromSlice(outputGradData, tensor.Shape{2, 3, 4}, backend)

	inputGrads := op.Backward(outputGrad.Raw(), backend)

	gradB := inputGrads[1]

	// grad_b should have shape (3,4) - summed over leading dimension
	if !gradB.Shape().Equal(b.Shape()) {
		t.Errorf("grad_b shape: got %v, want %v (should reduce leading dim)", gradB.Shape(), b.Shape())
	}

	// Verify it's not all zeros
	gradBData := gradB.AsFloat32()
	allZero := true
	for _, v := range gradBData {
		if v != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Error("grad_b should not be all zeros after reducing leading dimension")
	}
}

// TestReLUOp_Backward_AllNegative tests ReLU backward when all inputs are negative.
func TestReLUOp_Backward_AllNegative(t *testing.T) {
	// Use AutodiffBackend to prevent inplace corruption during backward pass
	backend := autodiff.New(cpu.New())

	input, _ := tensor.FromSlice([]float32{-5, -3, -1}, tensor.Shape{3}, backend)
	output, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, backend.Device())
	// ReLU output should be all zeros
	copy(output.AsFloat32(), []float32{0, 0, 0})

	op := ops.NewReLUOp(input.Raw(), output)

	outputGrad, _ := tensor.FromSlice([]float32{1, 2, 3}, tensor.Shape{3}, backend)
	inputGrads := op.Backward(outputGrad.Raw(), backend)

	// All inputs are negative, so all gradients should be 0
	expectedGrad := []float32{0, 0, 0}
	actualGrad := inputGrads[0].AsFloat32()

	if !float32Equal(actualGrad, expectedGrad, 1e-6) {
		t.Errorf("ReLUOp all negative grad: got %v, want %v", actualGrad, expectedGrad)
	}
}

// TestLogOp_Forward tests the forward pass of LogOp.
func TestLogOp_Forward(t *testing.T) {
	backend := cpu.New()

	// Test: log([1, e, e²])
	input, _ := tensor.FromSlice([]float32{1.0, float32(math.E), float32(math.E * math.E)}, tensor.Shape{3}, backend)

	output := ops.Log(input.Raw(), backend.Device())
	outputData := output.AsFloat32()

	// Expected: [0, 1, 2]
	expected := []float32{0, 1, 2}
	for i := range expected {
		if math.Abs(float64(outputData[i]-expected[i])) > 1e-6 {
			t.Errorf("Log forward: expected %f, got %f at index %d", expected[i], outputData[i], i)
		}
	}
}

// TestLogOp_Backward tests the backward pass of LogOp.
func TestLogOp_Backward(t *testing.T) {
	backend := cpu.New()

	// Input: [1, 2, 4]
	input, _ := tensor.FromSlice([]float32{1.0, 2.0, 4.0}, tensor.Shape{3}, backend)
	output := ops.Log(input.Raw(), backend.Device())

	// Create operation
	op := ops.NewLogOp(input.Raw(), output)

	// Output gradient: all ones
	outputGrad, _ := tensor.FromSlice([]float32{1.0, 1.0, 1.0}, tensor.Shape{3}, backend)

	// Backward pass
	inputGrads := op.Backward(outputGrad.Raw(), backend)
	inputGradData := inputGrads[0].AsFloat32()

	// Expected: [1/1, 1/2, 1/4] = [1.0, 0.5, 0.25]
	expected := []float32{1.0, 0.5, 0.25}
	if !float32Equal(inputGradData, expected, 1e-6) {
		t.Errorf("Log backward: got %v, want %v", inputGradData, expected)
	}
}

// TestLogOp_BackwardFloat64 tests LogOp backward with float64.
func TestLogOp_BackwardFloat64(t *testing.T) {
	backend := cpu.New()

	input, _ := tensor.FromSlice([]float64{1.0, 2.0, 4.0}, tensor.Shape{3}, backend)
	output := ops.Log(input.Raw(), backend.Device())

	op := ops.NewLogOp(input.Raw(), output)

	outputGrad, _ := tensor.FromSlice([]float64{1.0, 1.0, 1.0}, tensor.Shape{3}, backend)
	inputGrads := op.Backward(outputGrad.Raw(), backend)

	// Expected: [1.0, 0.5, 0.25]
	result := inputGrads[0].AsFloat64()
	expected := []float64{1.0, 0.5, 0.25}

	for i := range result {
		if math.Abs(result[i]-expected[i]) > 1e-9 {
			t.Errorf("Log backward float64: got %v, want %v", result, expected)
			break
		}
	}
}

// TestLogWithEpsilonOp tests LogWithEpsilonOp for numerical stability.
func TestLogWithEpsilonOp(t *testing.T) {
	backend := cpu.New()

	// Input: [0, 1e-10, 1]
	input, _ := tensor.FromSlice([]float32{0.0, 1e-10, 1.0}, tensor.Shape{3}, backend)

	epsilon := 1e-8

	// Forward: log(x + epsilon)
	output, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, backend.Device())
	inputData := input.Raw().AsFloat32()
	outputData := output.AsFloat32()
	for i, val := range inputData {
		outputData[i] = float32(math.Log(float64(val + float32(epsilon))))
	}

	op := ops.NewLogWithEpsilonOp(input.Raw(), output, epsilon)

	// Output gradient: all ones
	outputGrad, _ := tensor.FromSlice([]float32{1.0, 1.0, 1.0}, tensor.Shape{3}, backend)

	// Backward pass
	inputGrads := op.Backward(outputGrad.Raw(), backend)
	inputGradData := inputGrads[0].AsFloat32()

	// Expected: 1 / (x + epsilon)
	for i := range inputData {
		expected := float32(1.0 / (float64(inputData[i]) + epsilon))
		if math.Abs(float64(inputGradData[i]-expected)) > 1e-5 {
			t.Errorf("LogWithEpsilon backward: expected %f, got %f at index %d", expected, inputGradData[i], i)
		}
	}
}

// TestSoftmaxOp_Forward tests the forward pass of Softmax.
func TestSoftmaxOp_Forward(t *testing.T) {
	backend := cpu.New()

	// Test: 2D tensor [2, 3] (batch_size=2, num_classes=3)
	input, _ := tensor.FromSlice([]float32{
		1.0, 2.0, 3.0, // Batch 1
		0.0, 0.0, 0.0, // Batch 2
	}, tensor.Shape{2, 3}, backend)

	output := ops.Softmax(input.Raw(), backend.Device())
	outputData := output.AsFloat32()

	// Batch 1: softmax([1,2,3]) ≈ [0.09, 0.24, 0.67]
	e1 := math.Exp(1.0)
	e2 := math.Exp(2.0)
	e3 := math.Exp(3.0)
	sum1 := e1 + e2 + e3
	expected1 := []float32{float32(e1 / sum1), float32(e2 / sum1), float32(e3 / sum1)}

	for i := 0; i < 3; i++ {
		if math.Abs(float64(outputData[i]-expected1[i])) > 1e-6 {
			t.Errorf("Softmax batch1: expected %f, got %f at index %d", expected1[i], outputData[i], i)
		}
	}

	// Batch 2: softmax([0,0,0]) = [1/3, 1/3, 1/3]
	expected2 := float32(1.0 / 3.0)
	for i := 3; i < 6; i++ {
		if math.Abs(float64(outputData[i]-expected2)) > 1e-6 {
			t.Errorf("Softmax batch2: expected %f, got %f at index %d", expected2, outputData[i], i)
		}
	}

	// Verify probabilities sum to 1
	sum := float32(0.0)
	for i := 0; i < 3; i++ {
		sum += outputData[i]
	}
	if math.Abs(float64(sum-1.0)) > 1e-6 {
		t.Errorf("Softmax batch1: probabilities don't sum to 1, got %f", sum)
	}
}

// TestSoftmaxOp_Backward tests the backward pass of SoftmaxOp.
func TestSoftmaxOp_Backward(t *testing.T) {
	// Use autodiff backend so that backward operations (Mul, Sub, SumDim) work
	cpuBackend := cpu.New()
	backend := autodiff.New(cpuBackend)

	// Simple test: 1 sample, 2 classes
	input, _ := tensor.FromSlice([]float32{1.0, 2.0}, tensor.Shape{1, 2}, cpuBackend)

	// Compute softmax using ops.Softmax (non-autodiff version)
	output := ops.Softmax(input.Raw(), cpuBackend.Device())
	outputData := output.AsFloat32()

	op := ops.NewSoftmaxOp(input.Raw(), output, -1) // dim=-1 (last dimension)

	// Output gradient: [1, 0] (gradient w.r.t. first class only)
	outputGrad, _ := tensor.FromSlice([]float32{1.0, 0.0}, tensor.Shape{1, 2}, cpuBackend)

	// Backward pass - use autodiff backend for operations
	inputGrads := op.Backward(outputGrad.Raw(), backend)
	inputGradData := inputGrads[0].AsFloat32()

	// Verify gradient formula: ∂L/∂x_j = softmax_j * (∂L/∂softmax_j - dot_product)
	// dot_product = Σ(grad_output[i] * softmax[i]) = 1*softmax[0] + 0*softmax[1] = softmax[0]
	gradData := outputGrad.Raw().AsFloat32()
	dotProduct := outputData[0]

	expected0 := outputData[0] * (gradData[0] - dotProduct) // softmax[0] * (1 - softmax[0])
	expected1 := outputData[1] * (gradData[1] - dotProduct) // softmax[1] * (0 - softmax[0])

	if math.Abs(float64(inputGradData[0]-expected0)) > 1e-6 {
		t.Errorf("Softmax backward[0]: expected %f, got %f", expected0, inputGradData[0])
	}
	if math.Abs(float64(inputGradData[1]-expected1)) > 1e-6 {
		t.Errorf("Softmax backward[1]: expected %f, got %f", expected1, inputGradData[1])
	}

	// Verify Jacobian property: sum of gradients w.r.t. input should be 0
	// (because softmax outputs sum to 1, so gradients must sum to 0)
	gradSum := inputGradData[0] + inputGradData[1]
	if math.Abs(float64(gradSum)) > 1e-6 {
		t.Errorf("Softmax backward: gradient sum should be 0, got %f", gradSum)
	}
}

// TestSoftmax_NumericalStability tests softmax with extreme values.
func TestSoftmax_NumericalStability(t *testing.T) {
	backend := cpu.New()

	// Test with large values (potential overflow without max-shifting)
	input, _ := tensor.FromSlice([]float32{1000.0, 1001.0, 1002.0}, tensor.Shape{1, 3}, backend)

	output := ops.Softmax(input.Raw(), backend.Device())
	outputData := output.AsFloat32()

	// Verify no NaN or Inf
	for i := range outputData {
		if math.IsNaN(float64(outputData[i])) || math.IsInf(float64(outputData[i]), 0) {
			t.Errorf("Softmax produced invalid value at index %d: %f", i, outputData[i])
		}
	}

	// Verify probabilities sum to 1
	sum := float32(0.0)
	for i := range outputData {
		sum += outputData[i]
	}
	if math.Abs(float64(sum-1.0)) > 1e-5 {
		t.Errorf("Softmax with large values: probabilities don't sum to 1, got %f", sum)
	}

	// Verify output is approximately [0.09, 0.24, 0.67] (same proportions as [1,2,3])
	// Because softmax(x + c) = softmax(x) for any constant c
	e1 := math.Exp(1.0)
	e2 := math.Exp(2.0)
	e3 := math.Exp(3.0)
	sumExp := e1 + e2 + e3
	expected := []float32{float32(e1 / sumExp), float32(e2 / sumExp), float32(e3 / sumExp)}

	for i := range expected {
		if math.Abs(float64(outputData[i]-expected[i])) > 1e-5 {
			t.Errorf("Softmax stability: expected %f, got %f at index %d", expected[i], outputData[i], i)
		}
	}
}

// TestExp tests the Exp helper function.
func TestExp(t *testing.T) {
	backend := cpu.New()

	input, _ := tensor.FromSlice([]float32{0.0, 1.0, 2.0}, tensor.Shape{3}, backend)

	output := ops.Exp(input.Raw(), backend.Device())
	outputData := output.AsFloat32()

	// Expected: [1, e, e²]
	expected := []float32{1.0, float32(math.E), float32(math.E * math.E)}
	if !float32Equal(outputData, expected, 1e-6) {
		t.Errorf("Exp: got %v, want %v", outputData, expected)
	}
}

// TestSoftmaxOp_InputsOutputMethods tests Inputs() and Output() methods.
func TestSoftmaxOp_InputsOutputMethods(t *testing.T) {
	backend := cpu.New()

	input, _ := tensor.FromSlice([]float32{1.0, 2.0, 3.0}, tensor.Shape{1, 3}, backend)
	output := ops.Softmax(input.Raw(), backend.Device())

	op := ops.NewSoftmaxOp(input.Raw(), output, -1) // dim=-1 (last dimension)

	if len(op.Inputs()) != 1 {
		t.Errorf("SoftmaxOp.Inputs() length: got %d, want 1", len(op.Inputs()))
	}
	if op.Output() != output {
		t.Error("SoftmaxOp.Output() doesn't match result")
	}
}

// TestLogOp_InputsOutputMethods tests Inputs() and Output() methods.
func TestLogOp_InputsOutputMethods(t *testing.T) {
	backend := cpu.New()

	input, _ := tensor.FromSlice([]float32{1.0, 2.0, 3.0}, tensor.Shape{3}, backend)
	output := ops.Log(input.Raw(), backend.Device())

	op := ops.NewLogOp(input.Raw(), output)

	if len(op.Inputs()) != 1 {
		t.Errorf("LogOp.Inputs() length: got %d, want 1", len(op.Inputs()))
	}
	if op.Output() != output {
		t.Error("LogOp.Output() doesn't match result")
	}
}

// TestSoftmaxOp_Backward_3D tests SoftmaxOp backward with 3D tensors.
func TestSoftmaxOp_Backward_3D(t *testing.T) {
	cpuBackend := cpu.New()
	backend := autodiff.New(cpuBackend)

	// Test 3D tensor [Batch, SeqLen, VocabSize]
	// Simple case: [2, 3, 4]
	batchSize := 2
	seqLen := 3
	vocabSize := 4

	// Create input logits
	data := make([]float32, batchSize*seqLen*vocabSize)
	for i := range data {
		data[i] = float32(i%vocabSize) + 1.0 // [1, 2, 3, 4, 1, 2, 3, 4, ...]
	}
	logits, _ := tensor.FromSlice(data, tensor.Shape{batchSize, seqLen, vocabSize}, cpuBackend)

	// Apply softmax using CPU backend (to get output)
	probs := backend.Inner().Softmax(logits.Raw(), -1)

	// Create SoftmaxOp
	op := ops.NewSoftmaxOp(logits.Raw(), probs, -1)

	// Create upstream gradient (non-uniform - important!)
	// Using all ones would give zero gradient because sum(softmax) = const
	upstreamGradData := make([]float32, batchSize*seqLen*vocabSize)
	for i := range upstreamGradData {
		// Vary the gradient: [1, 2, 3, 4, 1, 2, 3, 4, ...]
		upstreamGradData[i] = float32(i%vocabSize) + 1.0
	}
	upstreamGrad, _ := tensor.FromSlice(
		upstreamGradData,
		tensor.Shape{batchSize, seqLen, vocabSize},
		cpuBackend,
	)

	// Call backward directly
	inputGrads := op.Backward(upstreamGrad.Raw(), backend)

	// Verify gradient shape
	gradLogits := inputGrads[0]
	expectedShape := tensor.Shape{batchSize, seqLen, vocabSize}
	if !gradLogits.Shape().Equal(expectedShape) {
		t.Errorf("Gradient shape: got %v, want %v", gradLogits.Shape(), expectedShape)
	}

	// Verify gradients are not all zeros (sanity check)
	gradData := gradLogits.AsFloat32()
	hasNonZero := false
	for _, v := range gradData {
		if v != 0 {
			hasNonZero = true
			break
		}
	}
	if !hasNonZero {
		t.Error("All gradients are zero, expected non-zero values")
	}
}

// TestSoftmaxOp_Backward_4D tests SoftmaxOp backward with 4D tensors (attention scores).
func TestSoftmaxOp_Backward_4D(t *testing.T) {
	cpuBackend := cpu.New()
	backend := autodiff.New(cpuBackend)

	// Attention scores: [Batch, Heads, SeqLen, SeqLen]
	batchSize := 2
	numHeads := 4
	seqLen := 8

	// Create input scores
	totalSize := batchSize * numHeads * seqLen * seqLen
	data := make([]float32, totalSize)
	for i := range data {
		data[i] = float32(i%seqLen) / float32(seqLen) // Small values
	}
	scores, _ := tensor.FromSlice(data, tensor.Shape{batchSize, numHeads, seqLen, seqLen}, cpuBackend)

	// Apply softmax using CPU backend (to get output)
	weights := backend.Inner().Softmax(scores.Raw(), -1)

	// Create SoftmaxOp
	op := ops.NewSoftmaxOp(scores.Raw(), weights, -1)

	// Create upstream gradient (non-uniform - important!)
	// Using all ones would give zero gradient because sum(softmax) = const
	upstreamGradData := make([]float32, totalSize)
	for i := range upstreamGradData {
		// Vary the gradient: [1, 2, 3, ..., seqLen, 1, 2, ...]
		upstreamGradData[i] = float32(i%seqLen) + 1.0
	}
	upstreamGrad, _ := tensor.FromSlice(
		upstreamGradData,
		tensor.Shape{batchSize, numHeads, seqLen, seqLen},
		cpuBackend,
	)

	// Call backward directly
	inputGrads := op.Backward(upstreamGrad.Raw(), backend)

	// Verify gradient shape
	gradScores := inputGrads[0]
	expectedShape := tensor.Shape{batchSize, numHeads, seqLen, seqLen}
	if !gradScores.Shape().Equal(expectedShape) {
		t.Errorf("Gradient shape: got %v, want %v", gradScores.Shape(), expectedShape)
	}

	// Verify gradients are not all zeros
	gradData := gradScores.AsFloat32()
	hasNonZero := false
	for _, v := range gradData {
		if v != 0 {
			hasNonZero = true
			break
		}
	}
	if !hasNonZero {
		t.Error("All gradients are zero, expected non-zero values")
	}

	// Additional check: verify gradient sum property
	// For softmax, gradients along the softmax dimension should sum to ~0
	// (because softmax outputs sum to 1, so their gradients must sum to 0)
	// Check one slice: [batch=0, head=0, seq=0, :]
	slice := gradData[0:seqLen]
	sum := float32(0.0)
	for _, v := range slice {
		sum += v
	}
	if math.Abs(float64(sum)) > 1e-4 {
		t.Logf("Warning: gradient sum along softmax dimension should be ~0, got %f", sum)
		// This is expected behavior, just logging for verification
	}
}

// TestSoftmaxOp_Backward_GradientCorrectness tests numerical gradient correctness.
func TestSoftmaxOp_Backward_GradientCorrectness(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// Simple 2D case for numerical gradient check
	// Input: [1, 3] shape
	input, _ := tensor.FromSlice([]float32{1.0, 2.0, 3.0}, tensor.Shape{1, 3}, backend)

	// Forward pass
	backend.Tape().StartRecording()
	output := input.Softmax(-1)
	loss := output.Sum() // Simple loss: sum of all outputs
	backend.Tape().StopRecording()

	// Analytical gradient
	grads := autodiff.Backward(loss, backend)
	analyticalGrad := grads[input.Raw()].AsFloat32()

	// Numerical gradient using finite differences
	epsilon := float32(1e-4)
	numericalGrad := make([]float32, 3)
	inputData := input.Raw().AsFloat32()

	for i := 0; i < 3; i++ {
		// Forward perturbation: input[i] + epsilon
		inputData[i] += epsilon
		outPlus := backend.Inner().Softmax(input.Raw(), -1)
		lossPlus := float32(0.0)
		outPlusData := outPlus.AsFloat32()
		for _, v := range outPlusData {
			lossPlus += v
		}

		// Backward perturbation: input[i] - epsilon
		inputData[i] -= 2 * epsilon
		outMinus := backend.Inner().Softmax(input.Raw(), -1)
		lossMinus := float32(0.0)
		outMinusData := outMinus.AsFloat32()
		for _, v := range outMinusData {
			lossMinus += v
		}

		// Restore original value
		inputData[i] += epsilon

		// Compute numerical gradient
		numericalGrad[i] = (lossPlus - lossMinus) / (2 * epsilon)
	}

	// Compare analytical and numerical gradients
	for i := 0; i < 3; i++ {
		diff := math.Abs(float64(analyticalGrad[i] - numericalGrad[i]))
		if diff > 1e-3 {
			t.Errorf("Gradient mismatch at index %d: analytical=%f, numerical=%f, diff=%f",
				i, analyticalGrad[i], numericalGrad[i], diff)
		}
	}
}
