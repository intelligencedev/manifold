// Copyright 2025 Born ML Framework. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package nn_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/serialization"
	"github.com/born-ml/born/internal/tensor"
	"github.com/born-ml/born/internal/tolerance"
	"github.com/born-ml/born/nn"
)

// TestModuleInterface verifies that concrete types implement Module interface.
func TestModuleInterface(t *testing.T) {
	backend := cpu.New()

	tests := []struct {
		name   string
		module nn.Module[*cpu.CPUBackend]
	}{
		{
			name:   "Linear",
			module: nn.NewLinear(10, 5, backend),
		},
		{
			name: "Sequential",
			module: nn.NewSequential[*cpu.CPUBackend](
				nn.NewLinear(10, 5, backend),
				nn.NewReLU[*cpu.CPUBackend](),
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify Forward works
			input := tensor.Randn[float32](tensor.Shape{2, 10}, backend)
			_ = tt.module.Forward(input)

			// Verify Parameters works
			params := tt.module.Parameters()
			if params == nil {
				t.Error("Parameters() returned nil, expected non-nil slice")
			}

			// Verify StateDict works
			stateDict := tt.module.StateDict()
			if stateDict == nil {
				t.Error("StateDict() returned nil, expected non-nil map")
			}
		})
	}
}

// TestParameterInterface verifies that concrete Parameter implements interface.
func TestParameterInterface(t *testing.T) {
	backend := cpu.New()
	tensorData := tensor.Randn[float32](tensor.Shape{3, 3}, backend)

	param := nn.NewParameter("test.weight", tensorData)

	// Verify interface methods
	if name := param.Name(); name != "test.weight" {
		t.Errorf("Name() = %q, want %q", name, "test.weight")
	}

	if got := param.Tensor(); got != tensorData {
		t.Error("Tensor() returned different tensor than provided")
	}

	if grad := param.Grad(); grad != nil {
		t.Error("Grad() should be nil before backward pass")
	}

	// Test SetGrad
	gradTensor := tensor.Zeros[float32](tensor.Shape{3, 3}, backend)
	param.SetGrad(gradTensor)
	if got := param.Grad(); got != gradTensor {
		t.Error("Grad() returned different tensor after SetGrad")
	}

	// Test ZeroGrad
	param.ZeroGrad()
	if grad := param.Grad(); grad != nil {
		t.Error("Grad() should be nil after ZeroGrad()")
	}
}

// TestModuleComposition verifies modules can be composed.
func TestModuleComposition(t *testing.T) {
	backend := cpu.New()

	// Create a sequential model
	model := nn.NewSequential[*cpu.CPUBackend](
		nn.NewLinear(784, 128, backend),
		nn.NewReLU[*cpu.CPUBackend](),
		nn.NewLinear(128, 10, backend),
	)

	// Verify it implements Module
	var _ nn.Module[*cpu.CPUBackend] = model

	// Test forward pass
	input := tensor.Randn[float32](tensor.Shape{2, 784}, backend)
	output := model.Forward(input)

	expectedShape := tensor.Shape{2, 10}
	if !output.Shape().Equal(expectedShape) {
		t.Errorf("Output shape = %v, want %v", output.Shape(), expectedShape)
	}

	// Verify parameters from nested modules
	params := model.Parameters()
	// 2 Linear layers: weights + biases = 4 parameters
	if len(params) != 4 {
		t.Errorf("Parameters() returned %d params, want 4", len(params))
	}
}

// TestNewParameter verifies parameter creation.
func TestNewParameter(t *testing.T) {
	backend := cpu.New()

	tests := []struct {
		name        string
		paramName   string
		tensorShape tensor.Shape
	}{
		{
			name:        "weight parameter",
			paramName:   "layer1.weight",
			tensorShape: tensor.Shape{128, 784},
		},
		{
			name:        "bias parameter",
			paramName:   "layer1.bias",
			tensorShape: tensor.Shape{128},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tensorData := tensor.Randn[float32](tt.tensorShape, backend)
			param := nn.NewParameter(tt.paramName, tensorData)

			if got := param.Name(); got != tt.paramName {
				t.Errorf("Name() = %q, want %q", got, tt.paramName)
			}

			if got := param.Tensor(); got != tensorData {
				t.Error("Tensor() returned different tensor")
			}
		})
	}
}

func TestLoadFromBytesRoundTrip(t *testing.T) {
	backend := cpu.New()

	model := nn.NewLinear(784, 128, backend)

	input := tensor.Randn[float32](tensor.Shape{1, 784}, backend)
	pred1 := model.Forward(input)

	tmpFile := t.TempDir() + "/model.born"
	if err := nn.Save(model, tmpFile, "Linear", nil); err != nil {
		t.Fatalf("Failed to save model: %v", err)
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to read saved file: %v", err)
	}

	model2 := nn.NewLinear(784, 128, backend)
	if _, err := nn.LoadFromBytes(data, backend, model2); err != nil {
		t.Fatalf("LoadFromBytes failed: %v", err)
	}

	pred2 := model2.Forward(input)

	pred1Data := pred1.Data()
	pred2Data := pred2.Data()
	if err := tolerance.AssertAllApproxEqual(pred1Data, pred2Data, tolerance.NewDefaultTolerance[float32]()); err != nil {
		t.Fatal(err)
	}
}

func TestLoadFromBytesInvalidData(t *testing.T) {
	backend := cpu.New()
	model := nn.NewLinear(10, 5, backend)

	_, err := nn.LoadFromBytes([]byte("garbage"), backend, model)
	if err == nil {
		t.Fatal("Expected error for invalid data, got nil")
	}
}

func TestLoadFromBytesShapeMismatch(t *testing.T) {
	backend := cpu.New()

	model := nn.NewLinear(10, 5, backend)
	stateDict := model.StateDict()

	var buf bytes.Buffer
	if err := serialization.WriteTo(&buf, stateDict, "Linear", nil); err != nil {
		t.Fatalf("WriteTo failed: %v", err)
	}

	model2 := nn.NewLinear(20, 5, backend)
	_, err := nn.LoadFromBytes(buf.Bytes(), backend, model2)
	if err == nil {
		t.Fatal("Expected error for shape mismatch, got nil")
	}
}

func TestSaveToBytesRoundTrip(t *testing.T) {
	backend := cpu.New()

	model := nn.NewLinear(784, 128, backend)

	input := tensor.Randn[float32](tensor.Shape{1, 784}, backend)
	pred1 := model.Forward(input)

	data, err := nn.SaveToBytes(model, "Linear", nil)
	if err != nil {
		t.Fatalf("SaveToBytes failed: %v", err)
	}

	model2 := nn.NewLinear(784, 128, backend)
	if _, err := nn.LoadFromBytes(data, backend, model2); err != nil {
		t.Fatalf("LoadFromBytes failed: %v", err)
	}

	pred2 := model2.Forward(input)

	pred1Data := pred1.Data()
	pred2Data := pred2.Data()
	if err := tolerance.AssertAllApproxEqual(pred1Data, pred2Data, tolerance.NewDefaultTolerance[float32]()); err != nil {
		t.Fatal(err)
	}
}

func TestSaveToBytesHeader(t *testing.T) {
	backend := cpu.New()

	model := nn.NewLinear(10, 5, backend)

	data, err := nn.SaveToBytes(model, "Linear", map[string]string{"dataset": "MNIST"})
	if err != nil {
		t.Fatalf("SaveToBytes failed: %v", err)
	}

	model2 := nn.NewLinear(10, 5, backend)
	header, err := nn.LoadFromBytes(data, backend, model2)
	if err != nil {
		t.Fatalf("LoadFromBytes failed: %v", err)
	}

	if header.ModelType != "Linear" {
		t.Errorf("Expected model type %q, got %q", "Linear", header.ModelType)
	}
	if got := header.Metadata["dataset"]; got != "MNIST" {
		t.Errorf("Expected metadata dataset %q, got %q", "MNIST", got)
	}
}

func TestSaveToLoadFromRoundTrip(t *testing.T) {
	backend := cpu.New()

	model := nn.NewLinear(784, 128, backend)

	input := tensor.Randn[float32](tensor.Shape{1, 784}, backend)
	pred1 := model.Forward(input)

	var buf bytes.Buffer
	if err := nn.SaveTo(&buf, model, "Linear", nil); err != nil {
		t.Fatalf("SaveTo failed: %v", err)
	}

	model2 := nn.NewLinear(784, 128, backend)
	if _, err := nn.LoadFrom(bytes.NewReader(buf.Bytes()), backend, model2); err != nil {
		t.Fatalf("LoadFrom failed: %v", err)
	}

	pred2 := model2.Forward(input)
	if err := tolerance.AssertAllApproxEqual(pred1.Data(), pred2.Data(), tolerance.NewDefaultTolerance[float32]()); err != nil {
		t.Fatal(err)
	}
}
