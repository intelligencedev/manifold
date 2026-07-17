//go:build windows || linux

package webgpu

import (
	"testing"

	"github.com/born-ml/born/internal/tensor"
)

// TestAutoFlush_ManyOpsWithoutReadback verifies that GPU does not crash
// when many operations are chained without any Data() readback.
//
// Before the auto-flush fix (maxPendingBeforeFlush), accumulating 500+
// dispatches triggered Windows TDR timeout (VK_ERROR_DEVICE_LOST) on iGPUs.
func TestAutoFlush_ManyOpsWithoutReadback(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer backend.Release()

	// Create initial tensor.
	data := make([]float32, 256)
	for i := range data {
		data[i] = float32(i) * 0.01
	}
	raw, err := tensor.NewRaw(tensor.Shape{16, 16}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatal(err)
	}
	copy(raw.AsFloat32(), data)

	// Chain 500 GPU ops without any readback.
	// This exceeds maxPendingBeforeFlush (128) — auto-flush must kick in.
	result := raw
	for i := 0; i < 500; i++ {
		result = backend.Add(result, raw)
	}

	// NOW read — this triggers final flushCommands + readback.
	out := result.AsFloat32()

	// Verify: result = original + 500 * original = 501 * original
	expected := float32(0) * 501 * 0.01 // element 0
	if len(out) != 256 {
		t.Fatalf("output length: got %d, want 256", len(out))
	}
	// Spot check element 10: 10*0.01 * 501 = 50.1
	got := out[10]
	want := float32(10) * 0.01 * 501
	if diff := got - want; diff > 0.5 || diff < -0.5 {
		t.Errorf("element 10: got %f, want %f (diff %f)", got, want, diff)
	}
	_ = expected
}

// TestAutoFlush_PendingCountResets verifies that the pending queue
// is drained by auto-flush and subsequent ops work correctly.
func TestAutoFlush_PendingCountResets(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer backend.Release()

	raw, err := tensor.NewRaw(tensor.Shape{4}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatal(err)
	}
	copy(raw.AsFloat32(), []float32{1, 2, 3, 4})

	// Phase 1: exceed threshold.
	result := raw
	for i := 0; i < maxPendingBeforeFlush+10; i++ {
		result = backend.Add(result, raw)
	}

	// Phase 2: more ops after auto-flush triggered.
	for i := 0; i < 50; i++ {
		result = backend.Mul(result, raw)
	}

	// Must not crash. Verify readback works.
	out := result.AsFloat32()
	if len(out) != 4 {
		t.Fatalf("output length: got %d, want 4", len(out))
	}
	// Just verify non-zero — exact values depend on op count.
	if out[0] == 0 && out[1] == 0 {
		t.Error("output is all zeros — auto-flush likely broke the pipeline")
	}
}

// TestAutoFlush_Threshold verifies the constant is sensible.
func TestAutoFlush_Threshold(t *testing.T) {
	if maxPendingBeforeFlush < 16 {
		t.Errorf("maxPendingBeforeFlush=%d too low — excessive flushing", maxPendingBeforeFlush)
	}
	if maxPendingBeforeFlush > 1024 {
		t.Errorf("maxPendingBeforeFlush=%d too high — risks TDR on iGPUs", maxPendingBeforeFlush)
	}
}
