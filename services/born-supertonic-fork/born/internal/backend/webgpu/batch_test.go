//go:build windows || linux

package webgpu

import (
	"testing"

	"github.com/born-ml/born/internal/tensor"
)

func TestNewBatch(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	backend, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer backend.Release()

	batch := backend.NewBatch()
	if batch == nil {
		t.Fatal("NewBatch returned nil")
	}

	if batch.backend != backend {
		t.Error("Batch backend mismatch")
	}

	if batch.encoder == nil {
		t.Error("Batch encoder is nil")
	}

	if batch.ops == nil {
		t.Error("Batch ops slice is nil")
	}

	if batch.Count() != 0 {
		t.Errorf("New batch should have 0 ops, got %d", batch.Count())
	}
}

func TestBatchAdd(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	backend, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer backend.Release()

	batch := backend.NewBatch()

	// Add a mock operation
	batch.Add("test_op", nil, func() {})

	if batch.Count() != 1 {
		t.Errorf("Expected 1 op in batch, got %d", batch.Count())
	}

	// Test method chaining
	batch.Add("test_op2", nil, func() {}).
		Add("test_op3", nil, func() {})

	if batch.Count() != 3 {
		t.Errorf("Expected 3 ops in batch, got %d", batch.Count())
	}
}

func TestBatchSubmit(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	backend, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer backend.Release()

	batch := backend.NewBatch()

	// Track which operations executed
	executed := make([]bool, 3)

	batch.Add("op1", nil, func() { executed[0] = true })
	batch.Add("op2", nil, func() { executed[1] = true })
	batch.Add("op3", nil, func() { executed[2] = true })

	// Submit the batch
	batch.Submit()

	// Verify all operations executed
	for i, exec := range executed {
		if !exec {
			t.Errorf("Operation %d did not execute", i)
		}
	}
}

func TestBatchSubmitEmpty(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	backend, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer backend.Release()

	batch := backend.NewBatch()

	// Submitting empty batch should not panic
	batch.Submit()

	if batch.Count() != 0 {
		t.Errorf("Empty batch should have 0 ops, got %d", batch.Count())
	}
}

func TestBatchMultipleOps(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	backend, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer backend.Release()

	// Create test tensors
	shapeA := tensor.Shape{4}
	dataA := []float32{1.0, 2.0, 3.0, 4.0}
	rawA, err := tensor.NewRaw(shapeA, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatalf("Failed to create raw tensor A: %v", err)
	}
	copy(rawA.AsFloat32(), dataA)

	shapeB := tensor.Shape{4}
	dataB := []float32{5.0, 6.0, 7.0, 8.0}
	rawB, err := tensor.NewRaw(shapeB, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatalf("Failed to create raw tensor B: %v", err)
	}
	copy(rawB.AsFloat32(), dataB)

	// Upload to GPU
	gpuA := backend.UploadTensor(rawA)
	defer gpuA.Release()

	gpuB := backend.UploadTensor(rawB)
	defer gpuB.Release()

	// Create a batch with multiple operations
	// Note: For this test, we're just testing the batching mechanism
	// The actual GPU operations are tested in gpu_ops_test.go
	batch := backend.NewBatch()

	opCount := 0
	batch.Add("add", nil, func() { opCount++ })
	batch.Add("mul", nil, func() { opCount++ })
	batch.Add("matmul", nil, func() { opCount++ })

	if batch.Count() != 3 {
		t.Errorf("Expected 3 ops, got %d", batch.Count())
	}

	// Submit all at once
	batch.Submit()

	if opCount != 3 {
		t.Errorf("Expected 3 ops to execute, got %d", opCount)
	}
}

func TestEvalWithBatch(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	backend, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer backend.Release()

	// Create a simple computation graph: c = a + b
	shapeA := tensor.Shape{4}
	dataA := []float32{1.0, 2.0, 3.0, 4.0}
	rawA, err := tensor.NewRaw(shapeA, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatalf("Failed to create raw tensor A: %v", err)
	}
	copy(rawA.AsFloat32(), dataA)

	gpuA := backend.UploadTensor(rawA)
	defer gpuA.Release()

	// Test 1: Eval on already computed tensor (should be no-op)
	result1 := gpuA.Eval()
	if result1 != gpuA {
		t.Error("Eval should return same tensor")
	}

	if !gpuA.computed {
		t.Error("Uploaded tensor should be marked as computed")
	}

	// Test 2: Eval with no dependencies
	gpuB := &GPUTensor{
		shape:      shapeA,
		dtype:      tensor.Float32,
		backend:    backend,
		computed:   false,
		bufferSize: 16,
	}

	result2 := gpuB.Eval()
	if !result2.computed {
		t.Error("Eval should mark tensor as computed")
	}

	// Test 3: Eval with dependencies (full lazy eval test)
	// Create dependency chain: d = c + b, where c = a + 1
	// Both operations should be batched together
	computeC := false
	computeD := false

	tensorC := &GPUTensor{
		shape:        shapeA,
		dtype:        tensor.Float32,
		backend:      backend,
		computed:     false,
		bufferSize:   16,
		dependencies: []*GPUTensor{gpuA},
		computeFunc: func() {
			computeC = true
		},
	}

	tensorD := &GPUTensor{
		shape:        shapeA,
		dtype:        tensor.Float32,
		backend:      backend,
		computed:     false,
		bufferSize:   16,
		dependencies: []*GPUTensor{tensorC, gpuA},
		computeFunc: func() {
			computeD = true
		},
	}

	// Eval should batch both computeC and computeD
	result3 := tensorD.Eval()

	if !computeC {
		t.Error("Dependency C should have been computed")
	}

	if !computeD {
		t.Error("Tensor D should have been computed")
	}

	if !result3.computed {
		t.Error("Result should be marked as computed")
	}

	if !tensorC.computed {
		t.Error("Dependency C should be marked as computed")
	}
}

func TestCollectDependencies(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	backend, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer backend.Release()

	// Create a computation graph:
	//     D
	//    / \
	//   B   C
	//    \ /
	//     A

	tensorA := &GPUTensor{
		shape:    tensor.Shape{2},
		dtype:    tensor.Float32,
		backend:  backend,
		computed: true, // Already computed
	}

	tensorB := &GPUTensor{
		shape:        tensor.Shape{2},
		dtype:        tensor.Float32,
		backend:      backend,
		computed:     false,
		dependencies: []*GPUTensor{tensorA},
	}

	tensorC := &GPUTensor{
		shape:        tensor.Shape{2},
		dtype:        tensor.Float32,
		backend:      backend,
		computed:     false,
		dependencies: []*GPUTensor{tensorA},
	}

	tensorD := &GPUTensor{
		shape:        tensor.Shape{2},
		dtype:        tensor.Float32,
		backend:      backend,
		computed:     false,
		dependencies: []*GPUTensor{tensorB, tensorC},
	}

	// Collect dependencies
	deps := tensorD.collectDependencies()

	// Should only include uncomputed tensors (B, C, D)
	// A is already computed so should be skipped
	if len(deps) != 3 {
		t.Errorf("Expected 3 dependencies (B, C, D), got %d", len(deps))
	}

	// Verify topological order: B and C before D
	dIndex := -1
	for i, dep := range deps {
		if dep == tensorD {
			dIndex = i
			break
		}
	}

	if dIndex == -1 {
		t.Fatal("Tensor D not found in dependencies")
	}

	// Check that B and C appear before D
	foundB := false
	foundC := false
	for i := 0; i < dIndex; i++ {
		if deps[i] == tensorB {
			foundB = true
		}
		if deps[i] == tensorC {
			foundC = true
		}
	}

	if !foundB || !foundC {
		t.Error("Dependencies B and C should appear before D in topological order")
	}
}

func TestCollectDependenciesNoOp(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	backend, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer backend.Release()

	// Test 1: Already computed tensor
	tensor1 := &GPUTensor{
		shape:    tensor.Shape{2},
		dtype:    tensor.Float32,
		backend:  backend,
		computed: true,
	}

	deps1 := tensor1.collectDependencies()
	if len(deps1) != 0 {
		t.Errorf("Already computed tensor should have 0 dependencies, got %d", len(deps1))
	}

	// Test 2: Tensor with no dependencies
	tensor2 := &GPUTensor{
		shape:    tensor.Shape{2},
		dtype:    tensor.Float32,
		backend:  backend,
		computed: false,
	}

	deps2 := tensor2.collectDependencies()
	if len(deps2) != 1 {
		t.Errorf("Tensor with no deps should return itself, got %d deps", len(deps2))
	}

	if len(deps2) > 0 && deps2[0] != tensor2 {
		t.Error("Dependency should be the tensor itself")
	}
}
