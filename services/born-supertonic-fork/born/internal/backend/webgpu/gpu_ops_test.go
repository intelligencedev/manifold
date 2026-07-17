//go:build windows || linux

package webgpu

import (
	"math"
	"testing"

	"github.com/born-ml/born/internal/tensor"
)

// TestAddGPU tests GPU-native addition.
func TestAddGPU(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	backend, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer backend.Release()

	// Create input tensors
	aData := []float32{1, 2, 3, 4}
	bData := []float32{5, 6, 7, 8}
	shape := tensor.Shape{2, 2}

	aRaw, _ := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	copy(aRaw.AsFloat32(), aData)
	bRaw, _ := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	copy(bRaw.AsFloat32(), bData)

	// Upload to GPU
	aGPU := backend.UploadTensor(aRaw)
	defer aGPU.Release()
	bGPU := backend.UploadTensor(bRaw)
	defer bGPU.Release()

	// Run GPU addition
	cGPU := backend.AddGPU(aGPU, bGPU)
	defer cGPU.Release()

	// Transfer result back to CPU
	result := cGPU.ToCPU()

	// Verify result
	expected := []float32{6, 8, 10, 12}
	resultData := result.AsFloat32()

	for i, exp := range expected {
		if resultData[i] != exp {
			t.Errorf("AddGPU[%d]: expected %v, got %v", i, exp, resultData[i])
		}
	}
}

// TestSubGPU tests GPU-native subtraction.
func TestSubGPU(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	backend, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer backend.Release()

	// Create input tensors
	aData := []float32{5, 6, 7, 8}
	bData := []float32{1, 2, 3, 4}
	shape := tensor.Shape{2, 2}

	aRaw, _ := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	copy(aRaw.AsFloat32(), aData)
	bRaw, _ := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	copy(bRaw.AsFloat32(), bData)

	// Upload to GPU
	aGPU := backend.UploadTensor(aRaw)
	defer aGPU.Release()
	bGPU := backend.UploadTensor(bRaw)
	defer bGPU.Release()

	// Run GPU subtraction
	cGPU := backend.SubGPU(aGPU, bGPU)
	defer cGPU.Release()

	// Transfer result back to CPU
	result := cGPU.ToCPU()

	// Verify result
	expected := []float32{4, 4, 4, 4}
	resultData := result.AsFloat32()

	for i, exp := range expected {
		if resultData[i] != exp {
			t.Errorf("SubGPU[%d]: expected %v, got %v", i, exp, resultData[i])
		}
	}
}

// TestMulGPU tests GPU-native multiplication.
func TestMulGPU(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	backend, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer backend.Release()

	// Create input tensors
	aData := []float32{1, 2, 3, 4}
	bData := []float32{2, 3, 4, 5}
	shape := tensor.Shape{2, 2}

	aRaw, _ := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	copy(aRaw.AsFloat32(), aData)
	bRaw, _ := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	copy(bRaw.AsFloat32(), bData)

	// Upload to GPU
	aGPU := backend.UploadTensor(aRaw)
	defer aGPU.Release()
	bGPU := backend.UploadTensor(bRaw)
	defer bGPU.Release()

	// Run GPU multiplication
	cGPU := backend.MulGPU(aGPU, bGPU)
	defer cGPU.Release()

	// Transfer result back to CPU
	result := cGPU.ToCPU()

	// Verify result
	expected := []float32{2, 6, 12, 20}
	resultData := result.AsFloat32()

	for i, exp := range expected {
		if resultData[i] != exp {
			t.Errorf("MulGPU[%d]: expected %v, got %v", i, exp, resultData[i])
		}
	}
}

// TestDivGPU tests GPU-native division.
func TestDivGPU(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	backend, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer backend.Release()

	// Create input tensors
	aData := []float32{10, 20, 30, 40}
	bData := []float32{2, 4, 5, 8}
	shape := tensor.Shape{2, 2}

	aRaw, _ := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	copy(aRaw.AsFloat32(), aData)
	bRaw, _ := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	copy(bRaw.AsFloat32(), bData)

	// Upload to GPU
	aGPU := backend.UploadTensor(aRaw)
	defer aGPU.Release()
	bGPU := backend.UploadTensor(bRaw)
	defer bGPU.Release()

	// Run GPU division
	cGPU := backend.DivGPU(aGPU, bGPU)
	defer cGPU.Release()

	// Transfer result back to CPU
	result := cGPU.ToCPU()

	// Verify result
	expected := []float32{5, 5, 6, 5}
	resultData := result.AsFloat32()

	for i, exp := range expected {
		if resultData[i] != exp {
			t.Errorf("DivGPU[%d]: expected %v, got %v", i, exp, resultData[i])
		}
	}
}

func TestErfGPU(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	backend, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer backend.Release()

	// Create input tensors
	aData := []float32{-2.0, -1.0, 0.0, 1.0, 2.0}
	shape := tensor.Shape{5}

	aRaw, _ := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	copy(aRaw.AsFloat32(), aData)

	// Upload to GPU
	aGPU := backend.UploadTensor(aRaw)
	defer aGPU.Release()

	// Run GPU error function
	cGPU := backend.ErfGPU(aGPU)
	defer cGPU.Release()

	// Transfer result back to CPU
	result := cGPU.ToCPU()

	// Verify result
	expected := []float32{-0.9953222650189527, -0.8427007929497148, 0.0, 0.8427007929497148, 0.9953222650189527}
	resultData := result.AsFloat32()

	const eps = 1e-5
	for i, exp := range expected {
		if math.Abs(float64(resultData[i]-exp)) > eps {
			t.Errorf("ErfGPU[%d]: expected %v, got %v", i, exp, resultData[i])
		}
	}
}

func TestSignGPU(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	backend, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer backend.Release()

	// Create input tensors
	aData := []float32{-1.0, 0.0, 1.0}
	shape := tensor.Shape{3}

	aRaw, _ := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	copy(aRaw.AsFloat32(), aData)

	// Upload to GPU
	aGPU := backend.UploadTensor(aRaw)
	defer aGPU.Release()

	// Run GPU sign function
	cGPU := backend.SignGPU(aGPU)
	defer cGPU.Release()

	// Transfer result back to CPU
	result := cGPU.ToCPU()

	// Verify result
	expected := []float32{-1.0, 0.0, 1.0}
	resultData := result.AsFloat32()

	const eps = 1e-5
	for i, exp := range expected {
		if math.Abs(float64(resultData[i]-exp)) > eps {
			t.Errorf("SignGPU[%d]: expected %v, got %v", i, exp, resultData[i])
		}
	}
}

func TestAbsGPU(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	backend, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer backend.Release()

	// Create input tensors
	aData := []float32{-5.0, -1.0, 0.0, 1.0, 5.0}
	shape := tensor.Shape{5}

	aRaw, _ := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	copy(aRaw.AsFloat32(), aData)

	// Upload to GPU
	aGPU := backend.UploadTensor(aRaw)
	defer aGPU.Release()

	// Run GPU absolute value function
	cGPU := backend.AbsGPU(aGPU)
	defer cGPU.Release()

	// Transfer result back to CPU
	result := cGPU.ToCPU()

	// Verify result
	expected := []float32{5.0, 1.0, 0.0, 1.0, 5.0}
	resultData := result.AsFloat32()

	const eps = 1e-5
	for i, exp := range expected {
		if math.Abs(float64(resultData[i]-exp)) > eps {
			t.Errorf("AbsGPU[%d]: expected %v, got %v", i, exp, resultData[i])
		}
	}
}

// TestMatMulGPU tests GPU-native matrix multiplication.
func TestMatMulGPU(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	backend, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer backend.Release()

	// Create 2x3 @ 3x2 = 2x2 matrix multiplication
	aData := []float32{
		1, 2, 3,
		4, 5, 6,
	}
	bData := []float32{
		7, 8,
		9, 10,
		11, 12,
	}

	aRaw, _ := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Float32, tensor.CPU)
	copy(aRaw.AsFloat32(), aData)
	bRaw, _ := tensor.NewRaw(tensor.Shape{3, 2}, tensor.Float32, tensor.CPU)
	copy(bRaw.AsFloat32(), bData)

	// Upload to GPU
	aGPU := backend.UploadTensor(aRaw)
	defer aGPU.Release()
	bGPU := backend.UploadTensor(bRaw)
	defer bGPU.Release()

	// Run GPU matmul
	cGPU := backend.MatMulGPU(aGPU, bGPU)
	defer cGPU.Release()

	// Transfer result back to CPU
	result := cGPU.ToCPU()

	// Verify result shape
	expectedShape := tensor.Shape{2, 2}
	if !result.Shape().Equal(expectedShape) {
		t.Fatalf("MatMulGPU: expected shape %v, got %v", expectedShape, result.Shape())
	}

	// Verify result values
	// [1*7+2*9+3*11, 1*8+2*10+3*12] = [58, 64]
	// [4*7+5*9+6*11, 4*8+5*10+6*12] = [139, 154]
	expected := []float32{58, 64, 139, 154}
	resultData := result.AsFloat32()

	for i, exp := range expected {
		if resultData[i] != exp {
			t.Errorf("MatMulGPU[%d]: expected %v, got %v", i, exp, resultData[i])
		}
	}
}

// TestTransposeGPU tests GPU-native transpose.
func TestTransposeGPU(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	backend, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer backend.Release()

	// Create 2x3 matrix
	data := []float32{
		1, 2, 3,
		4, 5, 6,
	}

	raw, _ := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Float32, tensor.CPU)
	copy(raw.AsFloat32(), data)

	// Upload to GPU
	gpu := backend.UploadTensor(raw)
	defer gpu.Release()

	// Run GPU transpose
	tGPU := backend.TransposeGPU(gpu)
	defer tGPU.Release()

	// Transfer result back to CPU
	result := tGPU.ToCPU()

	// Verify result shape
	expectedShape := tensor.Shape{3, 2}
	if !result.Shape().Equal(expectedShape) {
		t.Fatalf("TransposeGPU: expected shape %v, got %v", expectedShape, result.Shape())
	}

	// Verify result values
	expected := []float32{
		1, 4,
		2, 5,
		3, 6,
	}
	resultData := result.AsFloat32()

	for i, exp := range expected {
		if resultData[i] != exp {
			t.Errorf("TransposeGPU[%d]: expected %v, got %v", i, exp, resultData[i])
		}
	}
}

// TestReLUGPU tests GPU-native ReLU activation.
func TestReLUGPU(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	backend, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer backend.Release()

	// Create input tensor with negative and positive values
	data := []float32{-2, -1, 0, 1, 2, 3}
	shape := tensor.Shape{2, 3}

	raw, _ := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	copy(raw.AsFloat32(), data)

	// Upload to GPU
	gpu := backend.UploadTensor(raw)
	defer gpu.Release()

	// Run GPU ReLU
	reluGPU := backend.ReLUGPU(gpu)
	defer reluGPU.Release()

	// Transfer result back to CPU
	result := reluGPU.ToCPU()

	// Verify result
	expected := []float32{0, 0, 0, 1, 2, 3}
	resultData := result.AsFloat32()

	for i, exp := range expected {
		if resultData[i] != exp {
			t.Errorf("ReLUGPU[%d]: expected %v, got %v", i, exp, resultData[i])
		}
	}
}

// TestSigmoidGPU tests GPU-native sigmoid activation.
func TestSigmoidGPU(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	backend, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer backend.Release()

	// Create input tensor
	data := []float32{-2, -1, 0, 1, 2}
	shape := tensor.Shape{5}

	raw, _ := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	copy(raw.AsFloat32(), data)

	// Upload to GPU
	gpu := backend.UploadTensor(raw)
	defer gpu.Release()

	// Run GPU sigmoid
	sigmoidGPU := backend.SigmoidGPU(gpu)
	defer sigmoidGPU.Release()

	// Transfer result back to CPU
	result := sigmoidGPU.ToCPU()
	resultData := result.AsFloat32()

	// Verify result (sigmoid(x) = 1 / (1 + exp(-x)))
	for i, x := range data {
		expected := float32(1.0 / (1.0 + math.Exp(float64(-x))))
		if math.Abs(float64(resultData[i]-expected)) > 1e-6 {
			t.Errorf("SigmoidGPU[%d]: expected %v, got %v", i, expected, resultData[i])
		}
	}
}

// TestSoftmaxGPU tests GPU-native softmax activation.
func TestSoftmaxGPU(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	backend, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer backend.Release()

	// Create input tensor (2x3 - 2 batches, 3 features each)
	data := []float32{
		1, 2, 3, // batch 0
		4, 5, 6, // batch 1
	}
	shape := tensor.Shape{2, 3}

	raw, _ := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	copy(raw.AsFloat32(), data)

	// Upload to GPU
	gpu := backend.UploadTensor(raw)
	defer gpu.Release()

	// Run GPU softmax on last dimension
	softmaxGPU := backend.SoftmaxGPU(gpu, -1)
	defer softmaxGPU.Release()

	// Transfer result back to CPU
	result := softmaxGPU.ToCPU()
	resultData := result.AsFloat32()

	// Verify result - each batch should sum to 1
	batch0Sum := resultData[0] + resultData[1] + resultData[2]
	batch1Sum := resultData[3] + resultData[4] + resultData[5]

	if math.Abs(float64(batch0Sum-1.0)) > 1e-5 {
		t.Errorf("SoftmaxGPU batch 0: sum should be 1.0, got %v", batch0Sum)
	}
	if math.Abs(float64(batch1Sum-1.0)) > 1e-5 {
		t.Errorf("SoftmaxGPU batch 1: sum should be 1.0, got %v", batch1Sum)
	}

	// Verify that softmax values are in (0, 1)
	for i, val := range resultData {
		if val <= 0 || val >= 1 {
			t.Errorf("SoftmaxGPU[%d]: value %v should be in (0, 1)", i, val)
		}
	}
}

// TestGPUOpsChain tests chaining multiple GPU operations without CPU transfer.
// This is the key advantage - no intermediate CPU↔GPU transfers!
func TestGPUOpsChain(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	backend, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer backend.Release()

	// Create input tensors
	aData := []float32{1, 2, 3, 4}
	bData := []float32{2, 2, 2, 2}
	cData := []float32{3, 3, 3, 3}
	shape := tensor.Shape{2, 2}

	aRaw, _ := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	copy(aRaw.AsFloat32(), aData)
	bRaw, _ := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	copy(bRaw.AsFloat32(), bData)
	cRaw, _ := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	copy(cRaw.AsFloat32(), cData)

	// Upload to GPU
	aGPU := backend.UploadTensor(aRaw)
	defer aGPU.Release()
	bGPU := backend.UploadTensor(bRaw)
	defer bGPU.Release()
	cGPU := backend.UploadTensor(cRaw)
	defer cGPU.Release()

	// Chain operations on GPU: result = (a + b) * c
	// NO CPU transfer between operations!
	addResult := backend.AddGPU(aGPU, bGPU)
	defer addResult.Release()

	mulResult := backend.MulGPU(addResult, cGPU)
	defer mulResult.Release()

	// Only transfer final result to CPU
	result := mulResult.ToCPU()

	// Verify result: (a+b)*c = (1+2)*3, (2+2)*3, (3+2)*3, (4+2)*3 = 9, 12, 15, 18
	expected := []float32{9, 12, 15, 18}
	resultData := result.AsFloat32()

	for i, exp := range expected {
		if resultData[i] != exp {
			t.Errorf("GPUOpsChain[%d]: expected %v, got %v", i, exp, resultData[i])
		}
	}
}

// TestGPUOpsChainComplex tests complex operation chain.
func TestGPUOpsChainComplex(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	backend, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer backend.Release()

	// Create input
	data := []float32{-1, 0, 1, 2}
	shape := tensor.Shape{2, 2}

	raw, _ := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	copy(raw.AsFloat32(), data)

	// Upload to GPU
	gpu := backend.UploadTensor(raw)
	defer gpu.Release()

	// Chain: x -> ReLU -> Sigmoid
	// All operations stay on GPU!
	reluResult := backend.ReLUGPU(gpu)
	defer reluResult.Release()

	sigmoidResult := backend.SigmoidGPU(reluResult)
	defer sigmoidResult.Release()

	// Transfer final result to CPU
	result := sigmoidResult.ToCPU()
	resultData := result.AsFloat32()

	// Verify: ReLU(-1) = 0, ReLU(0) = 0, ReLU(1) = 1, ReLU(2) = 2
	//         Sigmoid(0) ≈ 0.5, Sigmoid(0) ≈ 0.5, Sigmoid(1) ≈ 0.731, Sigmoid(2) ≈ 0.881
	expectedReLU := []float32{0, 0, 1, 2}
	for i, val := range expectedReLU {
		expected := float32(1.0 / (1.0 + math.Exp(float64(-val))))
		if math.Abs(float64(resultData[i]-expected)) > 1e-3 {
			t.Errorf("GPUOpsChainComplex[%d]: expected %v, got %v", i, expected, resultData[i])
		}
	}
}

// TestGPUOpsInt32 tests GPU operations with int32 dtype.
func TestGPUOpsInt32(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	backend, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer backend.Release()

	// Create int32 input tensors
	aData := []int32{1, 2, 3, 4}
	bData := []int32{5, 6, 7, 8}
	shape := tensor.Shape{2, 2}

	aRaw, _ := tensor.NewRaw(shape, tensor.Int32, tensor.CPU)
	copy(aRaw.AsInt32(), aData)
	bRaw, _ := tensor.NewRaw(shape, tensor.Int32, tensor.CPU)
	copy(bRaw.AsInt32(), bData)

	// Upload to GPU
	aGPU := backend.UploadTensor(aRaw)
	defer aGPU.Release()
	bGPU := backend.UploadTensor(bRaw)
	defer bGPU.Release()

	// Run GPU addition with int32
	cGPU := backend.AddGPU(aGPU, bGPU)
	defer cGPU.Release()

	// Transfer result back to CPU
	result := cGPU.ToCPU()

	// Verify result
	expected := []int32{6, 8, 10, 12}
	resultData := result.AsInt32()

	for i, exp := range expected {
		if resultData[i] != exp {
			t.Errorf("AddGPU int32[%d]: expected %v, got %v", i, exp, resultData[i])
		}
	}
}
