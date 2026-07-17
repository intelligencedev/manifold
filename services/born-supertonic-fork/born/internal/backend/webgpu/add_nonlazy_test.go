//go:build windows || linux

package webgpu

import (
	"github.com/born-ml/born/internal/tensor"
	"testing"
)

func TestAddNonLazy(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}

	backend, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer backend.Release()

	// Explicitly disable lazy mode to test non-lazy path
	backend.SetLazyMode(false)

	a := createTensor(t, tensor.Shape{4}, []float32{1, 2, 3, 4})
	b := createTensor(t, tensor.Shape{4}, []float32{5, 6, 7, 8})

	result := backend.Add(a, b)
	expected := []float32{6, 8, 10, 12}
	actual := extractData(t, result)

	if !compareSlices(t, expected, actual, 1e-6) {
		t.Errorf("Add (non-lazy) failed: expected %v, got %v", expected, actual)
	} else {
		t.Logf("Add (non-lazy) PASSED: %v", actual)
	}
}
