package optim_test

import (
	"math"
	"testing"

	"github.com/born-ml/born/internal/autodiff"
	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/nn"
	"github.com/born-ml/born/internal/optim"
	"github.com/born-ml/born/internal/tensor"
)

// Helper to check float equality with tolerance.
func floatEqual(a, b, eps float32) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < eps
}

// TestSGD_SimpleUpdate tests SGD without momentum.
func TestSGD_SimpleUpdate(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// Create a simple parameter: x = [2.0]
	x, _ := tensor.FromSlice([]float32{2.0}, tensor.Shape{1}, backend)
	param := nn.NewParameter("x", x)

	// Create SGD optimizer (no momentum)
	optimizer := optim.NewSGD([]*nn.Parameter[*autodiff.AutodiffBackend[*cpu.CPUBackend]]{param},
		optim.SGDConfig{LR: 0.1, Momentum: 0.0},
		backend,
	)

	// Simulate gradient: grad_x = 1.0
	grad, _ := tensor.NewRaw(tensor.Shape{1}, tensor.Float32, backend.Device())
	grad.AsFloat32()[0] = 1.0

	// Create gradient map
	grads := map[*tensor.RawTensor]*tensor.RawTensor{
		param.Tensor().Raw(): grad,
	}

	// Perform one step
	optimizer.Step(grads)

	// Expected: x_new = x_old - lr * grad = 2.0 - 0.1 * 1.0 = 1.9
	expected := float32(1.9)
	actual := param.Tensor().Raw().AsFloat32()[0]

	if !floatEqual(actual, expected, 1e-6) {
		t.Errorf("SGD update: got %f, want %f", actual, expected)
	}
}

// TestSGD_WithMomentum tests SGD with momentum.
func TestSGD_WithMomentum(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// Create parameter: x = [1.0]
	x, _ := tensor.FromSlice([]float32{1.0}, tensor.Shape{1}, backend)
	param := nn.NewParameter("x", x)

	// Create SGD with momentum=0.9
	optimizer := optim.NewSGD([]*nn.Parameter[*autodiff.AutodiffBackend[*cpu.CPUBackend]]{param},
		optim.SGDConfig{LR: 0.1, Momentum: 0.9},
		backend,
	)

	// First step: grad = 1.0
	grad1, _ := tensor.NewRaw(tensor.Shape{1}, tensor.Float32, backend.Device())
	grad1.AsFloat32()[0] = 1.0

	grads1 := map[*tensor.RawTensor]*tensor.RawTensor{
		param.Tensor().Raw(): grad1,
	}

	optimizer.Step(grads1)

	// First step:
	// v_1 = 0.9 * 0 + 1.0 = 1.0
	// x_1 = 1.0 - 0.1 * 1.0 = 0.9
	expected1 := float32(0.9)
	actual1 := param.Tensor().Raw().AsFloat32()[0]

	if !floatEqual(actual1, expected1, 1e-6) {
		t.Errorf("SGD momentum step 1: got %f, want %f", actual1, expected1)
	}

	// Second step: grad = 1.0
	grad2, _ := tensor.NewRaw(tensor.Shape{1}, tensor.Float32, backend.Device())
	grad2.AsFloat32()[0] = 1.0

	grads2 := map[*tensor.RawTensor]*tensor.RawTensor{
		param.Tensor().Raw(): grad2,
	}

	optimizer.Step(grads2)

	// Second step:
	// v_2 = 0.9 * 1.0 + 1.0 = 1.9
	// x_2 = 0.9 - 0.1 * 1.9 = 0.71
	expected2 := float32(0.71)
	actual2 := param.Tensor().Raw().AsFloat32()[0]

	if !floatEqual(actual2, expected2, 1e-5) {
		t.Errorf("SGD momentum step 2: got %f, want %f", actual2, expected2)
	}
}

// TestSGD_ZeroGrad tests ZeroGrad method.
func TestSGD_ZeroGrad(t *testing.T) {
	backend := autodiff.New(cpu.New())

	x, _ := tensor.FromSlice([]float32{1.0}, tensor.Shape{1}, backend)
	param := nn.NewParameter("x", x)

	// Set gradient
	grad, _ := tensor.FromSlice([]float32{5.0}, tensor.Shape{1}, backend)
	param.SetGrad(grad)

	if param.Grad() == nil {
		t.Fatal("Grad should not be nil after SetGrad")
	}

	optimizer := optim.NewSGD([]*nn.Parameter[*autodiff.AutodiffBackend[*cpu.CPUBackend]]{param},
		optim.SGDConfig{LR: 0.1},
		backend,
	)

	// ZeroGrad should clear gradient
	optimizer.ZeroGrad()

	if param.Grad() != nil {
		t.Error("Grad should be nil after ZeroGrad")
	}
}

// TestSGD_GetSetLR tests learning rate getter/setter.
func TestSGD_GetSetLR(t *testing.T) {
	backend := autodiff.New(cpu.New())

	x, _ := tensor.FromSlice([]float32{1.0}, tensor.Shape{1}, backend)
	param := nn.NewParameter("x", x)

	optimizer := optim.NewSGD([]*nn.Parameter[*autodiff.AutodiffBackend[*cpu.CPUBackend]]{param},
		optim.SGDConfig{LR: 0.01},
		backend,
	)

	// Test GetLR
	if optimizer.GetLR() != 0.01 {
		t.Errorf("GetLR: got %f, want 0.01", optimizer.GetLR())
	}

	// Test SetLR
	optimizer.SetLR(0.001)
	if optimizer.GetLR() != 0.001 {
		t.Errorf("GetLR after SetLR: got %f, want 0.001", optimizer.GetLR())
	}
}

// TestAdam_SimpleUpdate tests Adam optimizer update.
func TestAdam_SimpleUpdate(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// Create parameter: x = [1.0]
	x, _ := tensor.FromSlice([]float32{1.0}, tensor.Shape{1}, backend)
	param := nn.NewParameter("x", x)

	// Create Adam optimizer with default hyperparameters
	optimizer := optim.NewAdam([]*nn.Parameter[*autodiff.AutodiffBackend[*cpu.CPUBackend]]{param},
		optim.AdamConfig{
			LR:    0.001,
			Betas: [2]float32{0.9, 0.999},
			Eps:   1e-8,
		},
		backend,
	)

	// Gradient: grad = 1.0
	grad, _ := tensor.NewRaw(tensor.Shape{1}, tensor.Float32, backend.Device())
	grad.AsFloat32()[0] = 1.0

	grads := map[*tensor.RawTensor]*tensor.RawTensor{
		param.Tensor().Raw(): grad,
	}

	// First step
	optimizer.Step(grads)

	// After first step (with bias correction):
	// m_1 = 0.9 * 0 + 0.1 * 1.0 = 0.1
	// v_1 = 0.999 * 0 + 0.001 * 1.0 = 0.001
	// m_hat = 0.1 / (1 - 0.9^1) = 0.1 / 0.1 = 1.0
	// v_hat = 0.001 / (1 - 0.999^1) = 0.001 / 0.001 = 1.0
	// x_new = 1.0 - 0.001 * 1.0 / (sqrt(1.0) + 1e-8) ≈ 0.999

	actual := param.Tensor().Raw().AsFloat32()[0]
	expected := float32(0.999)

	if !floatEqual(actual, expected, 1e-5) {
		t.Errorf("Adam first step: got %f, want %f", actual, expected)
	}
}

// TestAdam_BiasCorrection tests that Adam applies bias correction correctly.
func TestAdam_BiasCorrection(t *testing.T) {
	backend := autodiff.New(cpu.New())

	x, _ := tensor.FromSlice([]float32{1.0}, tensor.Shape{1}, backend)
	param := nn.NewParameter("x", x)

	optimizer := optim.NewAdam([]*nn.Parameter[*autodiff.AutodiffBackend[*cpu.CPUBackend]]{param},
		optim.AdamConfig{
			LR:    0.01,
			Betas: [2]float32{0.9, 0.999},
			Eps:   1e-8,
		},
		backend,
	)

	// Check initial timestep
	if optimizer.GetTimestep() != 0 {
		t.Errorf("Initial timestep: got %d, want 0", optimizer.GetTimestep())
	}

	grad, _ := tensor.NewRaw(tensor.Shape{1}, tensor.Float32, backend.Device())
	grad.AsFloat32()[0] = 1.0

	grads := map[*tensor.RawTensor]*tensor.RawTensor{
		param.Tensor().Raw(): grad,
	}

	// Perform 3 steps and verify timestep increments
	for i := 1; i <= 3; i++ {
		optimizer.Step(grads)

		if optimizer.GetTimestep() != i {
			t.Errorf("After step %d, timestep: got %d, want %d", i, optimizer.GetTimestep(), i)
		}
	}

	// Parameter should decrease over steps due to bias correction
	final := param.Tensor().Raw().AsFloat32()[0]
	if final >= 1.0 {
		t.Errorf("After 3 Adam steps with positive gradient, parameter should decrease: got %f", final)
	}
}

// TestAdam_ZeroGrad tests ZeroGrad for Adam.
func TestAdam_ZeroGrad(t *testing.T) {
	backend := autodiff.New(cpu.New())

	x, _ := tensor.FromSlice([]float32{1.0}, tensor.Shape{1}, backend)
	param := nn.NewParameter("x", x)

	grad, _ := tensor.FromSlice([]float32{5.0}, tensor.Shape{1}, backend)
	param.SetGrad(grad)

	optimizer := optim.NewAdam([]*nn.Parameter[*autodiff.AutodiffBackend[*cpu.CPUBackend]]{param},
		optim.AdamConfig{LR: 0.001},
		backend,
	)

	optimizer.ZeroGrad()

	if param.Grad() != nil {
		t.Error("Adam ZeroGrad should clear gradients")
	}
}

// TestConvergence_SimpleQuadratic tests optimizer convergence on f(x) = x².
//
// This is an integration test that verifies both SGD and Adam can minimize
// a simple quadratic function. The minimum is at x = 0.
func TestConvergence_SimpleQuadratic(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// Test SGD convergence
	t.Run("SGD", func(t *testing.T) {
		// Start at x = 3.0
		x, _ := tensor.FromSlice([]float32{3.0}, tensor.Shape{1}, backend)
		param := nn.NewParameter("x", x)

		optimizer := optim.NewSGD([]*nn.Parameter[*autodiff.AutodiffBackend[*cpu.CPUBackend]]{param},
			optim.SGDConfig{LR: 0.1, Momentum: 0.9},
			backend,
		)

		// Train for 100 steps
		// f(x) = x², df/dx = 2x
		for i := 0; i < 100; i++ {
			// Compute gradient manually: df/dx = 2x
			currentX := param.Tensor().Raw().AsFloat32()[0]
			gradValue := 2.0 * currentX

			grad, _ := tensor.NewRaw(tensor.Shape{1}, tensor.Float32, backend.Device())
			grad.AsFloat32()[0] = gradValue

			grads := map[*tensor.RawTensor]*tensor.RawTensor{
				param.Tensor().Raw(): grad,
			}

			optimizer.Step(grads)
		}

		// After 100 steps, x should be close to 0
		final := param.Tensor().Raw().AsFloat32()[0]
		if math.Abs(float64(final)) > 0.1 {
			t.Errorf("SGD convergence: x = %f, expected close to 0", final)
		}
	})

	// Test Adam convergence
	t.Run("Adam", func(t *testing.T) {
		// Start at x = 3.0
		x, _ := tensor.FromSlice([]float32{3.0}, tensor.Shape{1}, backend)
		param := nn.NewParameter("x", x)

		optimizer := optim.NewAdam([]*nn.Parameter[*autodiff.AutodiffBackend[*cpu.CPUBackend]]{param},
			optim.AdamConfig{
				LR:    0.1,
				Betas: [2]float32{0.9, 0.999},
				Eps:   1e-8,
			},
			backend,
		)

		// Train for 100 steps
		for i := 0; i < 100; i++ {
			currentX := param.Tensor().Raw().AsFloat32()[0]
			gradValue := 2.0 * currentX

			grad, _ := tensor.NewRaw(tensor.Shape{1}, tensor.Float32, backend.Device())
			grad.AsFloat32()[0] = gradValue

			grads := map[*tensor.RawTensor]*tensor.RawTensor{
				param.Tensor().Raw(): grad,
			}

			optimizer.Step(grads)
		}

		// After 100 steps, x should be close to 0
		final := param.Tensor().Raw().AsFloat32()[0]
		if math.Abs(float64(final)) > 0.1 {
			t.Errorf("Adam convergence: x = %f, expected close to 0", final)
		}
	})
}

// TestMultipleParameters tests optimizers with multiple parameters.
func TestMultipleParameters(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// Create two parameters
	x1, _ := tensor.FromSlice([]float32{1.0, 2.0}, tensor.Shape{2}, backend)
	param1 := nn.NewParameter("x1", x1)

	x2, _ := tensor.FromSlice([]float32{3.0}, tensor.Shape{1}, backend)
	param2 := nn.NewParameter("x2", x2)

	optimizer := optim.NewSGD(
		[]*nn.Parameter[*autodiff.AutodiffBackend[*cpu.CPUBackend]]{param1, param2},
		optim.SGDConfig{LR: 0.1},
		backend,
	)

	// Create gradients
	grad1, _ := tensor.NewRaw(tensor.Shape{2}, tensor.Float32, backend.Device())
	grad1.AsFloat32()[0] = 1.0
	grad1.AsFloat32()[1] = 2.0

	grad2, _ := tensor.NewRaw(tensor.Shape{1}, tensor.Float32, backend.Device())
	grad2.AsFloat32()[0] = 0.5

	grads := map[*tensor.RawTensor]*tensor.RawTensor{
		param1.Tensor().Raw(): grad1,
		param2.Tensor().Raw(): grad2,
	}

	// Perform step
	optimizer.Step(grads)

	// Check param1: [1.0, 2.0] - 0.1 * [1.0, 2.0] = [0.9, 1.8]
	p1Data := param1.Tensor().Raw().AsFloat32()
	if !floatEqual(p1Data[0], 0.9, 1e-6) || !floatEqual(p1Data[1], 1.8, 1e-6) {
		t.Errorf("param1: got [%f, %f], want [0.9, 1.8]", p1Data[0], p1Data[1])
	}

	// Check param2: 3.0 - 0.1 * 0.5 = 2.95
	p2Data := param2.Tensor().Raw().AsFloat32()
	if !floatEqual(p2Data[0], 2.95, 1e-6) {
		t.Errorf("param2: got %f, want 2.95", p2Data[0])
	}
}
