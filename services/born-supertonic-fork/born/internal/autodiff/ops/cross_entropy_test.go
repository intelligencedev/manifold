package ops_test

import (
	"math"
	"testing"

	"github.com/born-ml/born/internal/autodiff/ops"
	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/tensor"
)

// TestCrossEntropyOp_Forward tests forward pass of CrossEntropyOp.
func TestCrossEntropyOp_Forward(t *testing.T) {
	backend := cpu.New()

	// Simple test: 2 samples, 3 classes
	logits, _ := tensor.FromSlice([]float32{
		1.0, 2.0, 3.0, // Sample 1
		3.0, 2.0, 1.0, // Sample 2
	}, tensor.Shape{2, 3}, backend)

	targets, _ := tensor.FromSlice([]int32{2, 0}, tensor.Shape{2}, backend)

	output := ops.CrossEntropyForward(logits.Raw(), targets.Raw(), backend.Device())
	outputData := output.AsFloat32()

	// Verify output is scalar
	if len(output.Shape()) != 1 || output.Shape()[0] != 1 {
		t.Errorf("Output should be scalar, got shape %v", output.Shape())
	}

	// Verify loss is positive
	loss := outputData[0]
	if loss <= 0 {
		t.Errorf("Cross-entropy loss should be positive, got %f", loss)
	}

	// Verify reasonable range (for softmax of [1,2,3], loss should be small)
	if loss > 5.0 {
		t.Errorf("Cross-entropy loss seems too high: %f", loss)
	}
}

// TestCrossEntropyOp_Backward tests backward pass.
func TestCrossEntropyOp_Backward(t *testing.T) {
	backend := cpu.New()

	// Simple test: 2 samples, 3 classes
	logits, _ := tensor.FromSlice([]float32{
		1.0, 2.0, 3.0, // Sample 1, target class 2
		3.0, 2.0, 1.0, // Sample 2, target class 0
	}, tensor.Shape{2, 3}, backend)

	targets, _ := tensor.FromSlice([]int32{2, 0}, tensor.Shape{2}, backend)

	output := ops.CrossEntropyForward(logits.Raw(), targets.Raw(), backend.Device())

	op := ops.NewCrossEntropyOp(logits.Raw(), targets.Raw(), output)

	// Output gradient: 1.0 (standard for scalar loss)
	outputGrad, _ := tensor.FromSlice([]float32{1.0}, tensor.Shape{1}, backend)

	// Backward pass
	inputGrads := op.Backward(outputGrad.Raw(), backend)
	gradData := inputGrads[0].AsFloat32()

	// Verify gradient shape matches logits
	if len(inputGrads[0].Shape()) != 2 || !inputGrads[0].Shape().Equal(logits.Shape()) {
		t.Errorf("Gradient shape %v doesn't match logits shape %v", inputGrads[0].Shape(), logits.Shape())
	}

	// Verify gradient property: sum over classes should be 0 for each sample
	// This is because softmax outputs sum to 1, so gradients must sum to 0
	numClasses := 3
	for b := 0; b < 2; b++ {
		gradSum := float32(0.0)
		for i := 0; i < numClasses; i++ {
			gradSum += gradData[b*numClasses+i]
		}
		if math.Abs(float64(gradSum)) > 1e-6 {
			t.Errorf("Gradient sum for sample %d should be ~0, got %f", b, gradSum)
		}
	}

	// Verify gradient at target class is negative (softmax[target] - 1 < 0)
	target0 := 2 // First sample, target class 2
	target1 := 0 // Second sample, target class 0

	if gradData[0*numClasses+target0] >= 0 {
		t.Errorf("Gradient at target class should be negative, got %f", gradData[0*numClasses+target0])
	}
	if gradData[1*numClasses+target1] >= 0 {
		t.Errorf("Gradient at target class should be negative, got %f", gradData[1*numClasses+target1])
	}
}

// TestCrossEntropyOp_NumericalStability tests with extreme values.
func TestCrossEntropyOp_NumericalStability(t *testing.T) {
	backend := cpu.New()

	// Test with large logits (potential overflow)
	logits, _ := tensor.FromSlice([]float32{
		100.0, 101.0, 102.0,
	}, tensor.Shape{1, 3}, backend)

	targets, _ := tensor.FromSlice([]int32{2}, tensor.Shape{1}, backend)

	output := ops.CrossEntropyForward(logits.Raw(), targets.Raw(), backend.Device())
	outputData := output.AsFloat32()

	// Verify no NaN or Inf
	loss := outputData[0]
	if math.IsNaN(float64(loss)) || math.IsInf(float64(loss), 0) {
		t.Errorf("Cross-entropy produced invalid value: %f", loss)
	}

	// Verify loss is reasonable (should be small since target is max class)
	if loss > 5.0 {
		t.Errorf("Loss with correct prediction should be small, got %f", loss)
	}
}

// TestCrossEntropyOp_PerfectPrediction tests with perfect prediction.
func TestCrossEntropyOp_PerfectPrediction(t *testing.T) {
	backend := cpu.New()

	// Logits heavily favor class 1
	logits, _ := tensor.FromSlice([]float32{
		-100.0, 100.0, -100.0,
	}, tensor.Shape{1, 3}, backend)

	targets, _ := tensor.FromSlice([]int32{1}, tensor.Shape{1}, backend)

	output := ops.CrossEntropyForward(logits.Raw(), targets.Raw(), backend.Device())
	outputData := output.AsFloat32()

	loss := outputData[0]

	// Loss should be very close to 0 for perfect prediction
	if loss > 0.1 {
		t.Errorf("Loss for perfect prediction should be near 0, got %f", loss)
	}
}

// TestCrossEntropyOp_WorstPrediction tests with worst case prediction.
func TestCrossEntropyOp_WorstPrediction(t *testing.T) {
	backend := cpu.New()

	// Logits heavily favor class 0, but target is class 2
	logits, _ := tensor.FromSlice([]float32{
		100.0, -100.0, -100.0,
	}, tensor.Shape{1, 3}, backend)

	targets, _ := tensor.FromSlice([]int32{2}, tensor.Shape{1}, backend)

	output := ops.CrossEntropyForward(logits.Raw(), targets.Raw(), backend.Device())
	outputData := output.AsFloat32()

	loss := outputData[0]

	// Loss should be very high for completely wrong prediction
	if loss < 10.0 {
		t.Errorf("Loss for wrong prediction should be high, got %f", loss)
	}
}

// TestCrossEntropyOp_BatchAveraging tests that loss is averaged over batch.
func TestCrossEntropyOp_BatchAveraging(t *testing.T) {
	backend := cpu.New()

	// Identical samples (should give same result as single sample)
	logitsSingle, _ := tensor.FromSlice([]float32{
		1.0, 2.0, 3.0,
	}, tensor.Shape{1, 3}, backend)

	logitsBatch, _ := tensor.FromSlice([]float32{
		1.0, 2.0, 3.0,
		1.0, 2.0, 3.0,
	}, tensor.Shape{2, 3}, backend)

	targetsSingle, _ := tensor.FromSlice([]int32{2}, tensor.Shape{1}, backend)
	targetsBatch, _ := tensor.FromSlice([]int32{2, 2}, tensor.Shape{2}, backend)

	lossSingle := ops.CrossEntropyForward(logitsSingle.Raw(), targetsSingle.Raw(), backend.Device())
	lossBatch := ops.CrossEntropyForward(logitsBatch.Raw(), targetsBatch.Raw(), backend.Device())

	lossSingleVal := lossSingle.AsFloat32()[0]
	lossBatchVal := lossBatch.AsFloat32()[0]

	// Batch average should equal single sample loss
	if math.Abs(float64(lossSingleVal-lossBatchVal)) > 1e-6 {
		t.Errorf("Batch averaged loss (%f) should equal single sample loss (%f)",
			lossBatchVal, lossSingleVal)
	}
}

// TestCrossEntropyOp_Float64 tests with float64 dtype.
func TestCrossEntropyOp_Float64(t *testing.T) {
	backend := cpu.New()

	logits, _ := tensor.FromSlice([]float64{
		1.0, 2.0, 3.0,
		3.0, 2.0, 1.0,
	}, tensor.Shape{2, 3}, backend)

	targets, _ := tensor.FromSlice([]int32{2, 0}, tensor.Shape{2}, backend)

	output := ops.CrossEntropyForward(logits.Raw(), targets.Raw(), backend.Device())

	// Verify no panic and reasonable output
	outputData := output.AsFloat64()
	loss := outputData[0]

	if loss <= 0 || loss > 10.0 {
		t.Errorf("Float64 cross-entropy loss out of reasonable range: %f", loss)
	}

	// Test backward pass
	op := ops.NewCrossEntropyOp(logits.Raw(), targets.Raw(), output)
	outputGrad, _ := tensor.FromSlice([]float64{1.0}, tensor.Shape{1}, backend)

	inputGrads := op.Backward(outputGrad.Raw(), backend)

	// Verify gradient shape
	if !inputGrads[0].Shape().Equal(logits.Shape()) {
		t.Errorf("Float64 gradient shape mismatch")
	}

	// Verify gradient sum property
	gradData := inputGrads[0].AsFloat64()
	numClasses := 3
	for b := 0; b < 2; b++ {
		gradSum := 0.0
		for i := 0; i < numClasses; i++ {
			gradSum += gradData[b*numClasses+i]
		}
		if math.Abs(gradSum) > 1e-9 {
			t.Errorf("Float64 gradient sum for sample %d should be ~0, got %f", b, gradSum)
		}
	}
}

// TestCrossEntropyOp_InputsOutputMethods tests Inputs() and Output() methods.
func TestCrossEntropyOp_InputsOutputMethods(t *testing.T) {
	backend := cpu.New()

	logits, _ := tensor.FromSlice([]float32{1.0, 2.0, 3.0}, tensor.Shape{1, 3}, backend)
	targets, _ := tensor.FromSlice([]int32{2}, tensor.Shape{1}, backend)
	output := ops.CrossEntropyForward(logits.Raw(), targets.Raw(), backend.Device())

	op := ops.NewCrossEntropyOp(logits.Raw(), targets.Raw(), output)

	// Test Inputs() - should return only logits (targets are not differentiated)
	if len(op.Inputs()) != 1 {
		t.Errorf("CrossEntropyOp.Inputs() length: got %d, want 1", len(op.Inputs()))
	}
	if op.Inputs()[0] != logits.Raw() {
		t.Error("CrossEntropyOp.Inputs()[0] doesn't match logits")
	}

	// Test Output()
	if op.Output() != output {
		t.Error("CrossEntropyOp.Output() doesn't match result")
	}
}
