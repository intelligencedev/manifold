//go:build windows || linux

package webgpu

import (
	"testing"
	"time"

	"github.com/born-ml/born/internal/tensor"
)

// TestLazyModeAdd tests that lazy mode doesn't call readBuffer during Add.
func TestLazyModeAdd(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	backend, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer backend.Release()

	// Ensure lazy mode is enabled
	if !backend.LazyMode {
		t.Fatal("LazyMode should be enabled by default")
	}

	// Create input tensors
	a, err := tensor.NewRaw(tensor.Shape{1000, 1000}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatalf("Failed to create tensor a: %v", err)
	}
	b, err := tensor.NewRaw(tensor.Shape{1000, 1000}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatalf("Failed to create tensor b: %v", err)
	}

	// Fill with test data
	aData := a.AsFloat32()
	bData := b.AsFloat32()
	for i := range aData {
		aData[i] = float32(i % 100)
		bData[i] = float32(i % 50)
	}

	// Perform Add in lazy mode - should be fast (no readBuffer)
	start := time.Now()
	result := backend.Add(a, b)
	addTime := time.Since(start)

	// The result should be lazy (unrealized)
	if !result.IsLazy() {
		t.Error("Result should be lazy (unrealized)")
	}

	// Add operation should be very fast (<100ms for 1M elements)
	if addTime > 100*time.Millisecond {
		t.Errorf("Lazy Add took too long: %v (expected <100ms)", addTime)
	}

	t.Logf("Lazy Add time: %v", addTime)

	// Now access Data() - this triggers realization
	start = time.Now()
	_ = result.Data()
	realizeTime := time.Since(start)

	// Result should now be realized
	if result.IsLazy() {
		t.Error("Result should be realized after Data() call")
	}

	t.Logf("Realize time: %v", realizeTime)
}

// TestLazyModeChain tests chaining multiple lazy operations.
func TestLazyModeChain(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	backend, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer backend.Release()

	// Create input tensors
	a, err := tensor.NewRaw(tensor.Shape{100, 100}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatalf("Failed to create tensor a: %v", err)
	}
	b, err := tensor.NewRaw(tensor.Shape{100, 100}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatalf("Failed to create tensor b: %v", err)
	}

	// Fill with ones
	aData := a.AsFloat32()
	bData := b.AsFloat32()
	for i := range aData {
		aData[i] = 1.0
		bData[i] = 2.0
	}

	// Chain of operations - all should be fast (no readBuffer)
	start := time.Now()
	c := backend.Add(a, b) // 1 + 2 = 3
	d := backend.Mul(c, c) // 3 * 3 = 9 (but c is lazy!)
	e := backend.Add(d, a) // 9 + 1 = 10
	chainTime := time.Since(start)

	// All results should be lazy
	if !c.IsLazy() {
		t.Error("c should be lazy")
	}
	// Note: d and e may or may not be lazy depending on implementation
	// because they use c which triggers realization

	t.Logf("Chain time (3 ops): %v", chainTime)

	// Access final result
	_ = e.Data()

	// Verify correctness - e should be ~10.0
	eData := e.AsFloat32()
	expected := float32(10.0)
	tolerance := float32(0.001)
	for i := 0; i < 10; i++ {
		if eData[i] < expected-tolerance || eData[i] > expected+tolerance {
			t.Errorf("e[%d] = %v, expected ~%v", i, eData[i], expected)
		}
	}
}

// TestEagerModeAdd tests that eager mode calls readBuffer immediately.
func TestEagerModeAdd(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	backend, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer backend.Release()

	// Disable lazy mode
	backend.SetLazyMode(false)

	// Create input tensors
	a, err := tensor.NewRaw(tensor.Shape{100, 100}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatalf("Failed to create tensor a: %v", err)
	}
	b, err := tensor.NewRaw(tensor.Shape{100, 100}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatalf("Failed to create tensor b: %v", err)
	}

	// Fill with test data
	aData := a.AsFloat32()
	bData := b.AsFloat32()
	for i := range aData {
		aData[i] = float32(i)
		bData[i] = float32(i * 2)
	}

	// Perform Add in eager mode
	result := backend.Add(a, b)

	// In eager mode, result should NOT be lazy
	if result.IsLazy() {
		t.Error("Result should NOT be lazy in eager mode")
	}

	// Verify data is immediately available
	resultData := result.AsFloat32()
	for i := 0; i < 10; i++ {
		expected := float32(i + i*2)
		if resultData[i] != expected {
			t.Errorf("result[%d] = %v, expected %v", i, resultData[i], expected)
		}
	}
}

// BenchmarkLazyVsEager compares lazy vs eager mode performance.
func BenchmarkLazyVsEager(b *testing.B) {
	backend, err := New()
	if err != nil {
		b.Skipf("WebGPU not available: %v", err)
	}
	defer backend.Release()

	// Create input tensors
	size := 1000
	a, _ := tensor.NewRaw(tensor.Shape{size, size}, tensor.Float32, tensor.CPU)
	t2, _ := tensor.NewRaw(tensor.Shape{size, size}, tensor.Float32, tensor.CPU)

	// Fill with test data
	aData := a.AsFloat32()
	t2Data := t2.AsFloat32()
	for i := range aData {
		aData[i] = float32(i % 100)
		t2Data[i] = float32(i % 50)
	}

	b.Run("Lazy", func(b *testing.B) {
		backend.SetLazyMode(true)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			result := backend.Add(a, t2)
			_ = result.Data() // Force realization
		}
	})

	b.Run("Eager", func(b *testing.B) {
		backend.SetLazyMode(false)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			result := backend.Add(a, t2)
			_ = result.Data()
		}
	})
}
