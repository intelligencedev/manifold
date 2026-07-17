package nn_test

import (
	"math"
	"testing"

	"github.com/born-ml/born/internal/autodiff"
	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/nn"
	"github.com/born-ml/born/internal/tensor"
)

// Helper to check if values are approximately equal.
//
//nolint:unparam // epsilon is always 1e-5 in tests, but keeping it as parameter for flexibility
func floatEqual(a, b, epsilon float32) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < epsilon
}

// TestParameter tests Parameter creation and methods.
func TestParameter(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// Create a parameter
	data, _ := tensor.FromSlice([]float32{1, 2, 3}, tensor.Shape{3}, backend)
	param := nn.NewParameter("test_param", data)

	// Test Name
	if param.Name() != "test_param" {
		t.Errorf("Name() = %s, want test_param", param.Name())
	}

	// Test Tensor
	if param.Tensor() != data {
		t.Error("Tensor() should return the original tensor")
	}

	// Test Grad (initially nil)
	if param.Grad() != nil {
		t.Error("Grad() should initially be nil")
	}

	// Test SetGrad
	grad, _ := tensor.FromSlice([]float32{0.1, 0.2, 0.3}, tensor.Shape{3}, backend)
	param.SetGrad(grad)
	if param.Grad() != grad {
		t.Error("SetGrad() should set the gradient")
	}

	// Test ZeroGrad
	param.ZeroGrad()
	if param.Grad() != nil {
		t.Error("ZeroGrad() should clear the gradient")
	}
}

// TestLinear_Creation tests Linear layer initialization.
func TestLinear_Creation(t *testing.T) {
	backend := autodiff.New(cpu.New())

	layer := nn.NewLinear(10, 5, backend)

	// Check dimensions
	if layer.InFeatures() != 10 {
		t.Errorf("InFeatures() = %d, want 10", layer.InFeatures())
	}
	if layer.OutFeatures() != 5 {
		t.Errorf("OutFeatures() = %d, want 5", layer.OutFeatures())
	}

	// Check weight shape: [out_features, in_features]
	weight := layer.Weight().Tensor()
	expectedShape := tensor.Shape{5, 10}
	if !weight.Shape().Equal(expectedShape) {
		t.Errorf("Weight shape = %v, want %v", weight.Shape(), expectedShape)
	}

	// Check bias shape: [out_features]
	bias := layer.Bias().Tensor()
	expectedBiasShape := tensor.Shape{5}
	if !bias.Shape().Equal(expectedBiasShape) {
		t.Errorf("Bias shape = %v, want %v", bias.Shape(), expectedBiasShape)
	}

	// Check bias is zeros
	biasData := bias.Raw().AsFloat32()
	for i, v := range biasData {
		if v != 0 {
			t.Errorf("Bias[%d] = %f, want 0", i, v)
		}
	}

	// Check parameters
	params := layer.Parameters()
	if len(params) != 2 {
		t.Errorf("Parameters() length = %d, want 2", len(params))
	}
}

// TestLinear_Forward tests Linear layer forward pass.
func TestLinear_Forward(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// Create a simple 2x2 linear layer for easy verification
	layer := nn.NewLinear(2, 2, backend)

	// Set known weights and bias for testing
	// Weight: [[1, 2], [3, 4]] (out=2, in=2)
	weightData := []float32{1, 2, 3, 4}
	copy(layer.Weight().Tensor().Raw().AsFloat32(), weightData)

	// Bias: [0.5, 1.0]
	biasData := []float32{0.5, 1.0}
	copy(layer.Bias().Tensor().Raw().AsFloat32(), biasData)

	// Input: [[1, 1]] (batch=1, in=2)
	input, _ := tensor.FromSlice([]float32{1, 1}, tensor.Shape{1, 2}, backend)

	// Forward pass
	output := layer.Forward(input)

	// Expected:
	// y = x @ W.T + b
	// W.T = [[1, 3], [2, 4]] (transpose of [2,2])
	// x @ W.T = [1, 1] @ [[1, 3], [2, 4]] = [1*1+1*2, 1*3+1*4] = [3, 7]
	// y = [3, 7] + [0.5, 1.0] = [3.5, 8.0]

	expected := []float32{3.5, 8.0}
	actual := output.Raw().AsFloat32()

	for i, exp := range expected {
		if !floatEqual(actual[i], exp, 1e-5) {
			t.Errorf("Output[%d] = %f, want %f", i, actual[i], exp)
		}
	}

	// Check output shape: [1, 2]
	expectedShape := tensor.Shape{1, 2}
	if !output.Shape().Equal(expectedShape) {
		t.Errorf("Output shape = %v, want %v", output.Shape(), expectedShape)
	}
}

// TestLinear_ForwardBatch tests Linear with batch input.
func TestLinear_ForwardBatch(t *testing.T) {
	backend := autodiff.New(cpu.New())

	layer := nn.NewLinear(3, 2, backend)

	// Input: batch_size=4, in_features=3
	input := tensor.Randn[float32](tensor.Shape{4, 3}, backend)

	output := layer.Forward(input)

	// Check output shape: [4, 2]
	expectedShape := tensor.Shape{4, 2}
	if !output.Shape().Equal(expectedShape) {
		t.Errorf("Output shape = %v, want %v", output.Shape(), expectedShape)
	}
}

// TestLinear_WithBias tests Linear layer WithBias option.
func TestLinear_WithBias(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// Test 1: Default (with bias)
	layerWithBias := nn.NewLinear(10, 5, backend)
	if !layerWithBias.HasBias() {
		t.Error("Default Linear should have bias")
	}
	if layerWithBias.Bias() == nil {
		t.Error("Bias() should not be nil for default Linear")
	}
	if len(layerWithBias.Parameters()) != 2 {
		t.Errorf("Default Linear should have 2 parameters, got %d", len(layerWithBias.Parameters()))
	}

	// Test 2: Explicit WithBias(true)
	layerExplicitBias := nn.NewLinear(10, 5, backend, nn.WithBias(true))
	if !layerExplicitBias.HasBias() {
		t.Error("Linear with WithBias(true) should have bias")
	}
	if len(layerExplicitBias.Parameters()) != 2 {
		t.Errorf("Linear with bias should have 2 parameters, got %d", len(layerExplicitBias.Parameters()))
	}

	// Test 3: WithBias(false)
	layerNoBias := nn.NewLinear(10, 5, backend, nn.WithBias(false))
	if layerNoBias.HasBias() {
		t.Error("Linear with WithBias(false) should not have bias")
	}
	if layerNoBias.Bias() != nil {
		t.Error("Bias() should be nil for Linear without bias")
	}
	if len(layerNoBias.Parameters()) != 1 {
		t.Errorf("Linear without bias should have 1 parameter, got %d", len(layerNoBias.Parameters()))
	}

	// Verify weight is still properly initialized
	weight := layerNoBias.Weight().Tensor()
	expectedShape := tensor.Shape{5, 10}
	if !weight.Shape().Equal(expectedShape) {
		t.Errorf("Weight shape = %v, want %v", weight.Shape(), expectedShape)
	}
}

// TestLinear_NoBias_Forward tests forward pass of Linear without bias.
func TestLinear_NoBias_Forward(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// Create linear layer without bias
	layer := nn.NewLinear(2, 2, backend, nn.WithBias(false))

	// Set known weights: [[1, 2], [3, 4]] (out=2, in=2)
	weightData := []float32{1, 2, 3, 4}
	copy(layer.Weight().Tensor().Raw().AsFloat32(), weightData)

	// Input: [[1, 1]] (batch=1, in=2)
	input, _ := tensor.FromSlice([]float32{1, 1}, tensor.Shape{1, 2}, backend)

	// Forward pass
	output := layer.Forward(input)

	// Expected:
	// y = x @ W.T (no bias)
	// W.T = [[1, 3], [2, 4]]
	// x @ W.T = [1, 1] @ [[1, 3], [2, 4]] = [3, 7]
	expected := []float32{3.0, 7.0}
	actual := output.Raw().AsFloat32()

	for i, exp := range expected {
		if !floatEqual(actual[i], exp, 1e-5) {
			t.Errorf("Output[%d] = %f, want %f", i, actual[i], exp)
		}
	}
}

// TestLinear_NoBias_StateDict tests StateDict for Linear without bias.
func TestLinear_NoBias_StateDict(t *testing.T) {
	backend := autodiff.New(cpu.New())

	layer := nn.NewLinear(4, 3, backend, nn.WithBias(false))

	stateDict := layer.StateDict()

	// Should have weight but no bias
	if _, ok := stateDict["weight"]; !ok {
		t.Error("StateDict should contain 'weight'")
	}
	if _, ok := stateDict["bias"]; ok {
		t.Error("StateDict should not contain 'bias' for layer without bias")
	}
}

// TestReLU_Forward tests ReLU activation.
func TestReLU_Forward(t *testing.T) {
	backend := autodiff.New(cpu.New())

	relu := nn.NewReLU[*autodiff.AutodiffBackend[*cpu.CPUBackend]]()

	// Test data with negative, zero, and positive values
	input, _ := tensor.FromSlice([]float32{-2, -1, 0, 1, 2}, tensor.Shape{5}, backend)

	output := relu.Forward(input)

	expected := []float32{0, 0, 0, 1, 2}
	actual := output.Raw().AsFloat32()

	for i, exp := range expected {
		if actual[i] != exp {
			t.Errorf("ReLU output[%d] = %f, want %f", i, actual[i], exp)
		}
	}

	// Check no trainable parameters
	if len(relu.Parameters()) != 0 {
		t.Error("ReLU should have no parameters")
	}
}

// TestSigmoid_Forward tests Sigmoid activation.
func TestSigmoid_Forward(t *testing.T) {
	backend := autodiff.New(cpu.New())

	sigmoid := nn.NewSigmoid[*autodiff.AutodiffBackend[*cpu.CPUBackend]]()

	// Test with known values
	input, _ := tensor.FromSlice([]float32{0, 1, -1}, tensor.Shape{3}, backend)

	output := sigmoid.Forward(input)

	actual := output.Raw().AsFloat32()

	// σ(0) = 0.5
	// σ(1) ≈ 0.731
	// σ(-1) ≈ 0.269
	expected := []float32{
		0.5,
		float32(1.0 / (1.0 + math.Exp(-1.0))),
		float32(1.0 / (1.0 + math.Exp(1.0))),
	}

	for i, exp := range expected {
		if !floatEqual(actual[i], exp, 1e-5) {
			t.Errorf("Sigmoid output[%d] = %f, want %f", i, actual[i], exp)
		}
	}

	// Check no trainable parameters
	if len(sigmoid.Parameters()) != 0 {
		t.Error("Sigmoid should have no parameters")
	}
}

// TestTanh_Forward tests Tanh activation.
func TestTanh_Forward(t *testing.T) {
	backend := autodiff.New(cpu.New())

	tanh := nn.NewTanh[*autodiff.AutodiffBackend[*cpu.CPUBackend]]()

	// Test with known values
	input, _ := tensor.FromSlice([]float32{0, 1, -1}, tensor.Shape{3}, backend)

	output := tanh.Forward(input)

	actual := output.Raw().AsFloat32()

	// tanh(0) = 0
	// tanh(1) ≈ 0.761
	// tanh(-1) ≈ -0.761
	expected := []float32{
		0,
		float32(math.Tanh(1.0)),
		float32(math.Tanh(-1.0)),
	}

	for i, exp := range expected {
		if !floatEqual(actual[i], exp, 1e-5) {
			t.Errorf("Tanh output[%d] = %f, want %f", i, actual[i], exp)
		}
	}

	// Check no trainable parameters
	if len(tanh.Parameters()) != 0 {
		t.Error("Tanh should have no parameters")
	}
}

// TestSequential tests Sequential container.
func TestSequential(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// Create a simple network: Linear(3, 2) -> ReLU
	linear := nn.NewLinear(3, 2, backend)
	relu := nn.NewReLU[*autodiff.AutodiffBackend[*cpu.CPUBackend]]()

	model := nn.NewSequential[*autodiff.AutodiffBackend[*cpu.CPUBackend]](linear, relu)

	// Test Len
	if model.Len() != 2 {
		t.Errorf("Sequential.Len() = %d, want 2", model.Len())
	}

	// Test Module
	if model.Module(0) != linear {
		t.Error("Module(0) should be the linear layer")
	}
	if model.Module(1) != relu {
		t.Error("Module(1) should be ReLU")
	}

	// Test Forward
	input := tensor.Randn[float32](tensor.Shape{4, 3}, backend)
	output := model.Forward(input)

	// Output shape should be [4, 2] after Linear(3, 2)
	expectedShape := tensor.Shape{4, 2}
	if !output.Shape().Equal(expectedShape) {
		t.Errorf("Sequential output shape = %v, want %v", output.Shape(), expectedShape)
	}

	// Test Parameters (should have linear's weight and bias)
	params := model.Parameters()
	if len(params) != 2 {
		t.Errorf("Sequential.Parameters() length = %d, want 2", len(params))
	}
}

// TestSequential_Add tests Sequential.Add method.
func TestSequential_Add(t *testing.T) {
	backend := autodiff.New(cpu.New())

	model := nn.NewSequential[*autodiff.AutodiffBackend[*cpu.CPUBackend]]()

	if model.Len() != 0 {
		t.Error("Empty Sequential should have length 0")
	}

	// Add modules
	model.Add(nn.NewLinear(10, 5, backend))
	model.Add(nn.NewReLU[*autodiff.AutodiffBackend[*cpu.CPUBackend]]())
	model.Add(nn.NewLinear(5, 2, backend))

	if model.Len() != 3 {
		t.Errorf("After adding 3 modules, Len() = %d, want 3", model.Len())
	}
}

// TestMSELoss tests MSE loss computation.
func TestMSELoss(t *testing.T) {
	backend := autodiff.New(cpu.New())

	mse := nn.NewMSELoss(backend)

	// Predictions: [1, 2, 3]
	predictions, _ := tensor.FromSlice([]float32{1, 2, 3}, tensor.Shape{3}, backend)

	// Targets: [1, 1, 1]
	targets, _ := tensor.FromSlice([]float32{1, 1, 1}, tensor.Shape{3}, backend)

	// Compute loss
	loss := mse.Forward(predictions, targets)

	// Expected: mean((1-1)² + (2-1)² + (3-1)²) = mean(0 + 1 + 4) = 5/3 ≈ 1.667
	expected := float32(5.0 / 3.0)
	actual := loss.Raw().AsFloat32()[0]

	if !floatEqual(actual, expected, 1e-5) {
		t.Errorf("MSE loss = %f, want %f", actual, expected)
	}

	// Check no trainable parameters
	if len(mse.Parameters()) != 0 {
		t.Error("MSE loss should have no parameters")
	}
}

// TestInitialization tests Xavier initialization bounds.
func TestInitialization(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// Xavier initialization for fanIn=100, fanOut=50
	w := nn.Xavier(100, 50, tensor.Shape{50, 100}, backend)

	// Expected bound: sqrt(6 / (100 + 50)) ≈ 0.2
	expectedBound := math.Sqrt(6.0 / 150.0) // ≈ 0.2

	data := w.Raw().AsFloat32()

	// Check all values are within [-bound, bound]
	// Tolerance accounts for float64→float32 rounding in Xavier()
	const eps = 1e-6
	for i, val := range data {
		if math.Abs(float64(val)) > expectedBound+eps {
			t.Errorf("Xavier init value[%d] = %f exceeds bound %f", i, val, expectedBound)
		}
	}
}
