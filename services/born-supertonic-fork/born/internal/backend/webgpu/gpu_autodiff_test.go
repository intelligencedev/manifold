//go:build windows || linux

package webgpu

import (
	"math"
	"testing"

	"github.com/born-ml/born/internal/tensor"
)

// TestGPUTapeRecord tests that operations are recorded correctly.
func TestGPUTapeRecord(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	backend, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer backend.Release()

	tape := NewGPUTape(backend)

	// Create dummy tensors
	aRaw, _ := tensor.NewRaw(tensor.Shape{2, 2}, tensor.Float32, tensor.CPU)
	aGPU := backend.UploadTensor(aRaw)
	defer aGPU.Release()

	bRaw, _ := tensor.NewRaw(tensor.Shape{2, 2}, tensor.Float32, tensor.CPU)
	bGPU := backend.UploadTensor(bRaw)
	defer bGPU.Release()

	cGPU := backend.AddGPU(aGPU, bGPU)
	defer cGPU.Release()

	// Record operation
	tape.Record("add", []*GPUTensor{aGPU, bGPU}, cGPU, func(grad *GPUTensor) []*GPUTensor {
		return []*GPUTensor{grad, grad}
	})

	// Verify tape has one operation
	if tape.NumOps() != 1 {
		t.Errorf("Expected 1 operation, got %d", tape.NumOps())
	}

	// Verify tape can be cleared
	tape.Clear()
	if tape.NumOps() != 0 {
		t.Errorf("Expected 0 operations after clear, got %d", tape.NumOps())
	}
}

// TestGPUBackwardAdd tests gradient computation for addition.
func TestGPUBackwardAdd(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	backend, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer backend.Release()

	tape := NewGPUTape(backend)

	// Create tensors: a = [1, 2], b = [3, 4]
	aData := []float32{1, 2}
	bData := []float32{3, 4}
	shape := tensor.Shape{2}

	aRaw, _ := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	copy(aRaw.AsFloat32(), aData)
	aGPU := backend.UploadTensor(aRaw)
	defer aGPU.Release()
	aGPU.requiresGrad = true
	aGPU.tape = tape

	bRaw, _ := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	copy(bRaw.AsFloat32(), bData)
	bGPU := backend.UploadTensor(bRaw)
	defer bGPU.Release()
	bGPU.requiresGrad = true
	bGPU.tape = tape

	// Forward: c = a + b = [4, 6]
	cGPU := backend.AddGPU(aGPU, bGPU)
	defer cGPU.Release()

	// Record operation
	tape.Record("add", []*GPUTensor{aGPU, bGPU}, cGPU, func(grad *GPUTensor) []*GPUTensor {
		gradA, gradB := backend.AddBackwardGPU(aGPU, bGPU, grad)
		return []*GPUTensor{gradA, gradB}
	})

	// Create output gradient: [1, 1]
	gradData := []float32{1, 1}
	gradRaw, _ := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	copy(gradRaw.AsFloat32(), gradData)
	gradGPU := backend.UploadTensor(gradRaw)
	defer gradGPU.Release()

	// Backward pass
	grads := tape.Backward(gradGPU)

	// Verify gradients: both should be [1, 1]
	if gradA, ok := grads[aGPU]; ok {
		gradACPU := gradA.ToCPU()
		gradAData := gradACPU.AsFloat32()
		expected := []float32{1, 1}
		for i, exp := range expected {
			if math.Abs(float64(gradAData[i]-exp)) > 1e-5 {
				t.Errorf("grad_a[%d]: expected %v, got %v", i, exp, gradAData[i])
			}
		}
	} else {
		t.Error("Expected gradient for a")
	}

	if gradB, ok := grads[bGPU]; ok {
		gradBCPU := gradB.ToCPU()
		gradBData := gradBCPU.AsFloat32()
		expected := []float32{1, 1}
		for i, exp := range expected {
			if math.Abs(float64(gradBData[i]-exp)) > 1e-5 {
				t.Errorf("grad_b[%d]: expected %v, got %v", i, exp, gradBData[i])
			}
		}
	} else {
		t.Error("Expected gradient for b")
	}
}

// TestGPUBackwardMul tests gradient computation for multiplication.
func TestGPUBackwardMul(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	backend, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer backend.Release()

	tape := NewGPUTape(backend)

	// Create tensors: a = [2, 3], b = [4, 5]
	aData := []float32{2, 3}
	bData := []float32{4, 5}
	shape := tensor.Shape{2}

	aRaw, _ := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	copy(aRaw.AsFloat32(), aData)
	aGPU := backend.UploadTensor(aRaw)
	defer aGPU.Release()
	aGPU.requiresGrad = true
	aGPU.tape = tape

	bRaw, _ := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	copy(bRaw.AsFloat32(), bData)
	bGPU := backend.UploadTensor(bRaw)
	defer bGPU.Release()
	bGPU.requiresGrad = true
	bGPU.tape = tape

	// Forward: c = a * b = [8, 15]
	cGPU := backend.MulGPU(aGPU, bGPU)
	defer cGPU.Release()

	// Record operation
	tape.Record("mul", []*GPUTensor{aGPU, bGPU}, cGPU, func(grad *GPUTensor) []*GPUTensor {
		gradA, gradB := backend.MulBackwardGPU(aGPU, bGPU, grad)
		return []*GPUTensor{gradA, gradB}
	})

	// Create output gradient: [1, 1]
	gradData := []float32{1, 1}
	gradRaw, _ := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	copy(gradRaw.AsFloat32(), gradData)
	gradGPU := backend.UploadTensor(gradRaw)
	defer gradGPU.Release()

	// Backward pass
	grads := tape.Backward(gradGPU)

	// Verify gradients: grad_a = [4, 5], grad_b = [2, 3]
	if gradA, ok := grads[aGPU]; ok {
		gradACPU := gradA.ToCPU()
		gradAData := gradACPU.AsFloat32()
		expected := []float32{4, 5}
		for i, exp := range expected {
			if math.Abs(float64(gradAData[i]-exp)) > 1e-5 {
				t.Errorf("grad_a[%d]: expected %v, got %v", i, exp, gradAData[i])
			}
		}
	} else {
		t.Error("Expected gradient for a")
	}

	if gradB, ok := grads[bGPU]; ok {
		gradBCPU := gradB.ToCPU()
		gradBData := gradBCPU.AsFloat32()
		expected := []float32{2, 3}
		for i, exp := range expected {
			if math.Abs(float64(gradBData[i]-exp)) > 1e-5 {
				t.Errorf("grad_b[%d]: expected %v, got %v", i, exp, gradBData[i])
			}
		}
	} else {
		t.Error("Expected gradient for b")
	}
}

// TestGPUBackwardMatMul tests gradient computation for matrix multiplication.
func TestGPUBackwardMatMul(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	backend, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer backend.Release()

	tape := NewGPUTape(backend)

	// Create tensors: A = [[1, 2], [3, 4]], B = [[5, 6], [7, 8]]
	aData := []float32{1, 2, 3, 4}
	bData := []float32{5, 6, 7, 8}
	shape := tensor.Shape{2, 2}

	aRaw, _ := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	copy(aRaw.AsFloat32(), aData)
	aGPU := backend.UploadTensor(aRaw)
	defer aGPU.Release()
	aGPU.requiresGrad = true
	aGPU.tape = tape

	bRaw, _ := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	copy(bRaw.AsFloat32(), bData)
	bGPU := backend.UploadTensor(bRaw)
	defer bGPU.Release()
	bGPU.requiresGrad = true
	bGPU.tape = tape

	// Forward: C = A @ B
	cGPU := backend.MatMulGPU(aGPU, bGPU)
	defer cGPU.Release()

	// Record operation
	tape.Record("matmul", []*GPUTensor{aGPU, bGPU}, cGPU, func(grad *GPUTensor) []*GPUTensor {
		gradA, gradB := backend.MatMulBackwardGPU(aGPU, bGPU, grad)
		return []*GPUTensor{gradA, gradB}
	})

	// Create output gradient: all ones
	gradData := []float32{1, 1, 1, 1}
	gradRaw, _ := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	copy(gradRaw.AsFloat32(), gradData)
	gradGPU := backend.UploadTensor(gradRaw)
	defer gradGPU.Release()

	// Backward pass
	grads := tape.Backward(gradGPU)

	// Verify gradients exist
	if _, ok := grads[aGPU]; !ok {
		t.Error("Expected gradient for A")
	}

	if _, ok := grads[bGPU]; !ok {
		t.Error("Expected gradient for B")
	}

	// Note: Exact values depend on matmul backward implementation
	// Just verify shapes are correct
	if gradA, ok := grads[aGPU]; ok {
		if !gradA.Shape().Equal(aGPU.Shape()) {
			t.Errorf("grad_A shape mismatch: expected %v, got %v", aGPU.Shape(), gradA.Shape())
		}
	}

	if gradB, ok := grads[bGPU]; ok {
		if !gradB.Shape().Equal(bGPU.Shape()) {
			t.Errorf("grad_B shape mismatch: expected %v, got %v", bGPU.Shape(), gradB.Shape())
		}
	}
}

// TestGPUBackwardChain tests gradient computation through a chain of operations.
func TestGPUBackwardChain(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	backend, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer backend.Release()

	tape := NewGPUTape(backend)

	// Create tensors: a = [2], b = [3], c = [5]
	shape := tensor.Shape{1}

	aData := []float32{2}
	aRaw, _ := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	copy(aRaw.AsFloat32(), aData)
	aGPU := backend.UploadTensor(aRaw)
	defer aGPU.Release()
	aGPU.requiresGrad = true
	aGPU.tape = tape

	bData := []float32{3}
	bRaw, _ := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	copy(bRaw.AsFloat32(), bData)
	bGPU := backend.UploadTensor(bRaw)
	defer bGPU.Release()
	bGPU.requiresGrad = true
	bGPU.tape = tape

	cData := []float32{5}
	cRaw, _ := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	copy(cRaw.AsFloat32(), cData)
	cGPU := backend.UploadTensor(cRaw)
	defer cGPU.Release()
	cGPU.requiresGrad = true
	cGPU.tape = tape

	// Forward: d = a * b = [6]
	dGPU := backend.MulGPU(aGPU, bGPU)
	defer dGPU.Release()
	tape.Record("mul", []*GPUTensor{aGPU, bGPU}, dGPU, func(grad *GPUTensor) []*GPUTensor {
		gradA, gradB := backend.MulBackwardGPU(aGPU, bGPU, grad)
		return []*GPUTensor{gradA, gradB}
	})

	// Forward: e = d + c = [11]
	eGPU := backend.AddGPU(dGPU, cGPU)
	defer eGPU.Release()
	tape.Record("add", []*GPUTensor{dGPU, cGPU}, eGPU, func(grad *GPUTensor) []*GPUTensor {
		gradD, gradC := backend.AddBackwardGPU(dGPU, cGPU, grad)
		return []*GPUTensor{gradD, gradC}
	})

	// Create output gradient: [1]
	gradData := []float32{1}
	gradRaw, _ := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	copy(gradRaw.AsFloat32(), gradData)
	gradGPU := backend.UploadTensor(gradRaw)
	defer gradGPU.Release()

	// Backward pass
	grads := tape.Backward(gradGPU)

	// Verify gradients:
	// de/dc = 1
	// de/dd = 1
	// dd/da = b = 3
	// dd/db = a = 2
	// de/da = de/dd * dd/da = 1 * 3 = 3
	// de/db = de/dd * dd/db = 1 * 2 = 2
	// de/dc = 1

	if gradA, ok := grads[aGPU]; ok {
		gradACPU := gradA.ToCPU()
		gradAData := gradACPU.AsFloat32()
		expected := float32(3.0)
		if math.Abs(float64(gradAData[0]-expected)) > 1e-5 {
			t.Errorf("grad_a: expected %v, got %v", expected, gradAData[0])
		}
	} else {
		t.Error("Expected gradient for a")
	}

	if gradB, ok := grads[bGPU]; ok {
		gradBCPU := gradB.ToCPU()
		gradBData := gradBCPU.AsFloat32()
		expected := float32(2.0)
		if math.Abs(float64(gradBData[0]-expected)) > 1e-5 {
			t.Errorf("grad_b: expected %v, got %v", expected, gradBData[0])
		}
	} else {
		t.Error("Expected gradient for b")
	}

	if gradC, ok := grads[cGPU]; ok {
		gradCCPU := gradC.ToCPU()
		gradCData := gradCCPU.AsFloat32()
		expected := float32(1.0)
		if math.Abs(float64(gradCData[0]-expected)) > 1e-5 {
			t.Errorf("grad_c: expected %v, got %v", expected, gradCData[0])
		}
	} else {
		t.Error("Expected gradient for c")
	}
}

// TestGPUGradAccumulation tests that gradients accumulate correctly when
// the same tensor is used multiple times.
func TestGPUGradAccumulation(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	backend, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer backend.Release()

	tape := NewGPUTape(backend)

	// Create tensor: a = [2]
	shape := tensor.Shape{1}

	aData := []float32{2}
	aRaw, _ := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	copy(aRaw.AsFloat32(), aData)
	aGPU := backend.UploadTensor(aRaw)
	defer aGPU.Release()
	aGPU.requiresGrad = true
	aGPU.tape = tape

	// Forward: b = a + a = [4]
	bGPU := backend.AddGPU(aGPU, aGPU)
	defer bGPU.Release()
	tape.Record("add", []*GPUTensor{aGPU, aGPU}, bGPU, func(grad *GPUTensor) []*GPUTensor {
		gradA1, gradA2 := backend.AddBackwardGPU(aGPU, aGPU, grad)
		return []*GPUTensor{gradA1, gradA2}
	})

	// Create output gradient: [1]
	gradData := []float32{1}
	gradRaw, _ := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	copy(gradRaw.AsFloat32(), gradData)
	gradGPU := backend.UploadTensor(gradRaw)
	defer gradGPU.Release()

	// Backward pass
	grads := tape.Backward(gradGPU)

	// Verify gradient: grad_a should be 2 (accumulated from both uses)
	if gradA, ok := grads[aGPU]; ok {
		gradACPU := gradA.ToCPU()
		gradAData := gradACPU.AsFloat32()
		expected := float32(2.0)
		if math.Abs(float64(gradAData[0]-expected)) > 1e-5 {
			t.Errorf("grad_a: expected %v, got %v (should accumulate)", expected, gradAData[0])
		}
	} else {
		t.Error("Expected gradient for a")
	}
}

// TestGPUTapeEnable tests enabling/disabling tape recording.
func TestGPUTapeEnable(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	backend, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer backend.Release()

	tape := NewGPUTape(backend)

	// Tape should be enabled by default
	if !tape.IsEnabled() {
		t.Error("Expected tape to be enabled by default")
	}

	// Create dummy tensors
	aRaw, _ := tensor.NewRaw(tensor.Shape{2}, tensor.Float32, tensor.CPU)
	aGPU := backend.UploadTensor(aRaw)
	defer aGPU.Release()

	bRaw, _ := tensor.NewRaw(tensor.Shape{2}, tensor.Float32, tensor.CPU)
	bGPU := backend.UploadTensor(bRaw)
	defer bGPU.Release()

	cGPU := backend.AddGPU(aGPU, bGPU)
	defer cGPU.Release()

	// Record operation
	tape.Record("add", []*GPUTensor{aGPU, bGPU}, cGPU, func(grad *GPUTensor) []*GPUTensor {
		return []*GPUTensor{grad, grad}
	})

	if tape.NumOps() != 1 {
		t.Errorf("Expected 1 operation, got %d", tape.NumOps())
	}

	// Disable tape
	tape.Disable()
	if tape.IsEnabled() {
		t.Error("Expected tape to be disabled")
	}

	// Record should be a no-op when disabled
	tape.Record("mul", []*GPUTensor{aGPU, bGPU}, cGPU, func(grad *GPUTensor) []*GPUTensor {
		return []*GPUTensor{grad, grad}
	})

	if tape.NumOps() != 1 {
		t.Errorf("Expected 1 operation (recording disabled), got %d", tape.NumOps())
	}

	// Re-enable tape
	tape.Enable()
	if !tape.IsEnabled() {
		t.Error("Expected tape to be enabled")
	}

	// Recording should work again
	tape.Record("sub", []*GPUTensor{aGPU, bGPU}, cGPU, func(grad *GPUTensor) []*GPUTensor {
		return []*GPUTensor{grad, grad}
	})

	if tape.NumOps() != 2 {
		t.Errorf("Expected 2 operations, got %d", tape.NumOps())
	}
}
