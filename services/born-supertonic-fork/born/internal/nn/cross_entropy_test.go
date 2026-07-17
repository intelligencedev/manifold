package nn_test

import (
	"math"
	"testing"

	"github.com/born-ml/born/internal/autodiff"
	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/nn"
	"github.com/born-ml/born/internal/tensor"
)

// Helper to check float equality with tolerance.
func floatEqualCE(a, b, eps float32) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < eps
}

// TestCrossEntropyLoss_Forward tests the forward pass of CrossEntropyLoss.
func TestCrossEntropyLoss_Forward(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// Create simple 2-class classification problem
	// Logits: [[2.0, 1.0]] (class 0 is more confident)
	// Target: 0
	logitsRaw, _ := tensor.NewRaw(tensor.Shape{1, 2}, tensor.Float32, backend.Device())
	logitsRaw.AsFloat32()[0] = 2.0 // class 0
	logitsRaw.AsFloat32()[1] = 1.0 // class 1
	logits := tensor.New[float32](logitsRaw, backend)

	targetsRaw, _ := tensor.NewRaw(tensor.Shape{1}, tensor.Int32, backend.Device())
	targetsRaw.AsInt32()[0] = 0 // target is class 0
	targets := tensor.New[int32](targetsRaw, backend)

	// Create loss function
	criterion := nn.NewCrossEntropyLoss(backend)
	loss := criterion.Forward(logits, targets)

	// Expected loss calculation (manual):
	// log_softmax([2.0, 1.0])
	// max = 2.0
	// exp(2-2) = 1.0, exp(1-2) = 0.368
	// sum_exp = 1.0 + 0.368 = 1.368
	// log_sum_exp = 2.0 + log(1.368) = 2.0 + 0.313 = 2.313
	// log_softmax[0] = 2.0 - 2.313 = -0.313
	// log_softmax[1] = 1.0 - 2.313 = -1.313
	// loss = -log_softmax[target] = -(-0.313) = 0.313

	expectedLoss := float32(0.313)
	actualLoss := loss.Raw().AsFloat32()[0]

	if !floatEqualCE(actualLoss, expectedLoss, 1e-2) {
		t.Errorf("CrossEntropyLoss forward: got %f, want %f", actualLoss, expectedLoss)
	}
}

// TestCrossEntropyLoss_Batch tests loss computation on a batch.
func TestCrossEntropyLoss_Batch(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// Batch of 3 samples, 3 classes
	// Sample 0: logits=[1, 2, 3], target=2 (correct: highest score)
	// Sample 1: logits=[3, 1, 2], target=0 (correct: highest score)
	// Sample 2: logits=[2, 3, 1], target=1 (correct: highest score)
	logitsRaw, _ := tensor.NewRaw(tensor.Shape{3, 3}, tensor.Float32, backend.Device())
	logitsData := logitsRaw.AsFloat32()
	// Sample 0
	logitsData[0] = 1.0
	logitsData[1] = 2.0
	logitsData[2] = 3.0
	// Sample 1
	logitsData[3] = 3.0
	logitsData[4] = 1.0
	logitsData[5] = 2.0
	// Sample 2
	logitsData[6] = 2.0
	logitsData[7] = 3.0
	logitsData[8] = 1.0

	logits := tensor.New[float32](logitsRaw, backend)

	targetsRaw, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Int32, backend.Device())
	targetsData := targetsRaw.AsInt32()
	targetsData[0] = 2 // target for sample 0
	targetsData[1] = 0 // target for sample 1
	targetsData[2] = 1 // target for sample 2
	targets := tensor.New[int32](targetsRaw, backend)

	criterion := nn.NewCrossEntropyLoss(backend)
	loss := criterion.Forward(logits, targets)

	actualLoss := loss.Raw().AsFloat32()[0]

	// Since all predictions are correct (highest logit matches target),
	// the loss should be relatively small (around 0.4 for each sample)
	// Average loss should be around 0.4
	if actualLoss < 0.0 || actualLoss > 1.0 {
		t.Errorf("CrossEntropyLoss batch: loss %f out of expected range [0.0, 1.0]", actualLoss)
	}
}

// TestCrossEntropyBackward tests the backward pass.
func TestCrossEntropyBackward(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// Simple case: 2 classes, 1 sample
	// Logits: [1.0, 2.0], Target: 1
	logitsRaw, _ := tensor.NewRaw(tensor.Shape{1, 2}, tensor.Float32, backend.Device())
	logitsRaw.AsFloat32()[0] = 1.0
	logitsRaw.AsFloat32()[1] = 2.0
	logits := tensor.New[float32](logitsRaw, backend)

	targetsRaw, _ := tensor.NewRaw(tensor.Shape{1}, tensor.Int32, backend.Device())
	targetsRaw.AsInt32()[0] = 1 // target is class 1
	targets := tensor.New[int32](targetsRaw, backend)

	// Compute gradient
	grad := nn.CrossEntropyBackward(logits, targets, backend)
	gradData := grad.Raw().AsFloat32()

	// Expected gradient: softmax([1.0, 2.0]) - [0, 1]
	// softmax([1.0, 2.0]):
	// max = 2.0
	// exp(1-2) = 0.368, exp(2-2) = 1.0
	// sum = 1.368
	// softmax = [0.368/1.368, 1.0/1.368] = [0.269, 0.731]
	// gradient = [0.269, 0.731] - [0, 1] = [0.269, -0.269]
	// divided by batch_size (1) = [0.269, -0.269]

	expectedGrad0 := float32(0.269)
	expectedGrad1 := float32(-0.269)

	if !floatEqualCE(gradData[0], expectedGrad0, 1e-2) {
		t.Errorf("Gradient[0]: got %f, want %f", gradData[0], expectedGrad0)
	}

	if !floatEqualCE(gradData[1], expectedGrad1, 1e-2) {
		t.Errorf("Gradient[1]: got %f, want %f", gradData[1], expectedGrad1)
	}
}

// TestAccuracy tests the Accuracy function.
func TestAccuracy(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// Batch of 4 samples, 3 classes
	// Sample 0: [1, 2, 3] -> predicted=2, target=2 ✓
	// Sample 1: [3, 1, 2] -> predicted=0, target=0 ✓
	// Sample 2: [2, 3, 1] -> predicted=1, target=0 ✗
	// Sample 3: [1, 1, 3] -> predicted=2, target=2 ✓
	// Accuracy: 3/4 = 0.75

	logitsRaw, _ := tensor.NewRaw(tensor.Shape{4, 3}, tensor.Float32, backend.Device())
	logitsData := logitsRaw.AsFloat32()
	logitsData[0], logitsData[1], logitsData[2] = 1, 2, 3   // sample 0
	logitsData[3], logitsData[4], logitsData[5] = 3, 1, 2   // sample 1
	logitsData[6], logitsData[7], logitsData[8] = 2, 3, 1   // sample 2
	logitsData[9], logitsData[10], logitsData[11] = 1, 1, 3 // sample 3
	logits := tensor.New[float32](logitsRaw, backend)

	targetsRaw, _ := tensor.NewRaw(tensor.Shape{4}, tensor.Int32, backend.Device())
	targetsData := targetsRaw.AsInt32()
	targetsData[0] = 2 // correct
	targetsData[1] = 0 // correct
	targetsData[2] = 0 // incorrect (predicted 1)
	targetsData[3] = 2 // correct
	targets := tensor.New[int32](targetsRaw, backend)

	acc := nn.Accuracy(logits, targets)

	expectedAcc := float32(0.75)
	if !floatEqualCE(acc, expectedAcc, 1e-6) {
		t.Errorf("Accuracy: got %f, want %f", acc, expectedAcc)
	}
}

// TestLogSoftmax_NumericalStability tests that log-sum-exp trick prevents overflow.
func TestLogSoftmax_NumericalStability(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// Extreme positive logits (would overflow without max-shifting)
	// [1000, 999, 998]
	logitsRaw, _ := tensor.NewRaw(tensor.Shape{1, 3}, tensor.Float32, backend.Device())
	logitsRaw.AsFloat32()[0] = 1000
	logitsRaw.AsFloat32()[1] = 999
	logitsRaw.AsFloat32()[2] = 998
	logits := tensor.New[float32](logitsRaw, backend)

	targetsRaw, _ := tensor.NewRaw(tensor.Shape{1}, tensor.Int32, backend.Device())
	targetsRaw.AsInt32()[0] = 0
	targets := tensor.New[int32](targetsRaw, backend)

	criterion := nn.NewCrossEntropyLoss(backend)
	loss := criterion.Forward(logits, targets)

	lossValue := loss.Raw().AsFloat32()[0]

	// Loss should be finite (not Inf or NaN)
	if math.IsInf(float64(lossValue), 0) || math.IsNaN(float64(lossValue)) {
		t.Errorf("Loss is not finite with extreme logits: %f", lossValue)
	}

	// Loss should be small (close to 0) since target has highest logit
	if lossValue > 1.0 {
		t.Errorf("Loss too high with extreme logits: %f", lossValue)
	}
}

// TestCrossEntropyLoss_WrongTarget tests panic on invalid target index.
func TestCrossEntropyLoss_WrongTarget(t *testing.T) {
	backend := autodiff.New(cpu.New())

	logitsRaw, _ := tensor.NewRaw(tensor.Shape{1, 3}, tensor.Float32, backend.Device())
	logitsRaw.AsFloat32()[0] = 1.0
	logitsRaw.AsFloat32()[1] = 2.0
	logitsRaw.AsFloat32()[2] = 3.0
	logits := tensor.New[float32](logitsRaw, backend)

	// Invalid target (out of bounds)
	targetsRaw, _ := tensor.NewRaw(tensor.Shape{1}, tensor.Int32, backend.Device())
	targetsRaw.AsInt32()[0] = 5 // invalid: only 3 classes
	targets := tensor.New[int32](targetsRaw, backend)

	criterion := nn.NewCrossEntropyLoss(backend)

	// Should panic
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic for invalid target index")
		}
	}()

	criterion.Forward(logits, targets)
}

// TestCrossEntropyLoss_AutodiffTape tests that CrossEntropyLoss records on autodiff tape.
func TestCrossEntropyLoss_AutodiffTape(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// Start recording
	backend.Tape().StartRecording()
	defer backend.Tape().StopRecording()

	// Create logits [batch=2, classes=3]
	logits, _ := tensor.FromSlice([]float32{
		1.0, 2.0, 3.0, // sample 0
		3.0, 2.0, 1.0, // sample 1
	}, tensor.Shape{2, 3}, backend)

	targets, _ := tensor.FromSlice([]int32{2, 0}, tensor.Shape{2}, backend)

	// Forward pass through CrossEntropyLoss
	criterion := nn.NewCrossEntropyLoss(backend)
	loss := criterion.Forward(logits, targets)

	// Check that tape has operations recorded
	numOps := backend.Tape().NumOps()
	if numOps == 0 {
		t.Error("CrossEntropyLoss should record operations on autodiff tape")
	}

	// Verify loss value is reasonable
	lossValue := loss.Raw().AsFloat32()[0]
	if lossValue < 0 || lossValue > 10 {
		t.Errorf("Unexpected loss value: %f", lossValue)
	}

	// Backward pass should work
	grads := autodiff.Backward(loss, backend)

	// Gradients should exist for logits
	logitsGrad := grads[logits.Raw()]
	if logitsGrad == nil {
		t.Error("CrossEntropyLoss backward: logits gradient should not be nil")
	}

	// Gradient should have same shape as logits
	if !logitsGrad.Shape().Equal(logits.Shape()) {
		t.Errorf("Gradient shape mismatch: got %v, want %v",
			logitsGrad.Shape(), logits.Shape())
	}

	// Gradient values should be non-zero (softmax - one_hot)
	gradData := logitsGrad.AsFloat32()
	hasNonZero := false
	for _, g := range gradData {
		if g != 0 {
			hasNonZero = true
			break
		}
	}
	if !hasNonZero {
		t.Error("CrossEntropyLoss gradient should have non-zero values")
	}
}
