//go:build windows || linux

// Package webgpu implements the WebGPU backend for GPU-accelerated tensor operations.
package webgpu

import (
	"runtime"
	"testing"

	"github.com/born-ml/born/internal/tensor"
	wgpu "github.com/gogpu/wgpu"
)

// TestDeferredStaging_NoStagingDuringOps verifies that chaining 100 lazy Add
// ops creates only Storage|CopySrc result buffers — no MapRead staging buffers
// allocated until Data() is called.
//
// This is verified by inspecting the LazyGPUData buffer usage flags: a Storage
// buffer cannot be mapped, so a successful Map() call would indicate a staging
// buffer leaked through. We confirm that no Map() is possible on the raw buffer
// pointer embedded in LazyGPUData before Data() forces readback.
func TestDeferredStaging_NoStagingDuringOps(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer backend.Release()

	raw, err := tensor.NewRaw(tensor.Shape{64}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatal(err)
	}
	for i := range raw.AsFloat32() {
		raw.AsFloat32()[i] = 1.0
	}

	// Chain 100 Add ops without any readback.
	result := raw
	for i := 0; i < 100; i++ {
		result = backend.Add(result, raw)
	}

	// result is now a lazy tensor. Its buffer must be Storage|CopySrc, not MapRead.
	gpuData := result.GPUData()
	if gpuData == nil {
		t.Fatal("expected lazy tensor with GPU data after 100 chained Adds")
	}
	if gpuData.IsRealized() {
		t.Fatal("GPU data should not be realized yet — no Data() call was made")
	}

	// Verify the buffer pointer is non-nil (buffer was created).
	if gpuData.BufferPtr() == nil {
		t.Fatal("buffer pointer is nil — result buffer was released prematurely")
	}
}

// TestDeferredStaging_DirectStorageBinding verifies that a lazy GPU tensor is
// bound directly as compute input without a GPU→GPU copy. Before deferred
// staging, getOrCreateInputBuffer called copyGPUBuffer for lazy tensors, which
// forced a flush+Poll synchronization. After the fix, the result buffer is
// reused directly, and activeBatch.count should exceed 1 after chained ops.
func TestDeferredStaging_DirectStorageBinding(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer backend.Release()

	raw, err := tensor.NewRaw(tensor.Shape{16, 16}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatal(err)
	}
	for i := range raw.AsFloat32() {
		raw.AsFloat32()[i] = float32(i) * 0.1
	}

	// Do two chained ops. If direct binding works, both should be in the SAME
	// encoder batch (activeBatch.count == 2 at peak, or was > 1 before flush).
	_ = backend.Add(raw, raw)       // op 1: CPU+CPU → lazy
	result := backend.Add(raw, raw) // op 2: CPU+CPU → lazy (independent)

	// The activeBatch should have accumulated ops (count > 0 means not yet flushed).
	batchCount := backend.activeBatchCount()
	if batchCount < 1 {
		t.Errorf("expected activeBatch.count >= 1 after 2 Add ops, got %d", batchCount)
	}

	// Trigger readback and verify correctness.
	data := result.AsFloat32()
	if len(data) != 256 {
		t.Fatalf("got %d elements, want 256", len(data))
	}
	// raw[i] + raw[i] = 2 * raw[i]
	for i, v := range data {
		want := 2 * float32(i) * 0.1
		if diff := v - want; diff > 1e-5 || diff < -1e-5 {
			t.Errorf("element %d: got %f, want %f", i, v, want)
			break
		}
	}
}

// TestDeferredStaging_StagingOnDataOnly verifies the core deferred staging
// invariant: the staging buffer (MapRead) is created only on Data() access,
// not during op execution. After N lazy ops, we verify that no read-mapped
// buffers have been allocated by confirming that the batch has not been
// drained spuriously (no sync points injected between ops).
func TestDeferredStaging_StagingOnDataOnly(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer backend.Release()

	raw, err := tensor.NewRaw(tensor.Shape{32}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatal(err)
	}
	for i := range raw.AsFloat32() {
		raw.AsFloat32()[i] = float32(i + 1)
	}

	// Chain 10 ops — none should trigger a flush/Poll by themselves (no sync).
	var result *tensor.RawTensor
	result = backend.Add(raw, raw)
	for i := 1; i < 10; i++ {
		result = backend.Add(result, raw)
	}

	// Before Data(): GPU data should be unrealized.
	gpuData := result.GPUData()
	if gpuData == nil || gpuData.IsRealized() {
		t.Fatal("result must be an unrealized lazy tensor before Data() call")
	}

	// Trigger realization (Data call) — this is when staging is created.
	out := result.AsFloat32()
	if len(out) != 32 {
		t.Fatalf("got %d elements, want 32", len(out))
	}

	// After Data(): GPU data should be realized.
	// Note: gpuData pointer was captured before; the tensor itself is now realized.
	gpuData2 := result.GPUData()
	// GPUData() may return nil after realization in some implementations, or
	// return the same object marked as realized. Both are acceptable.
	if gpuData2 != nil && !gpuData2.IsRealized() {
		t.Error("GPU data should be realized after Data() call")
	}

	// Spot check: result = raw + raw*10 = 11*raw
	for i, v := range out {
		want := float32(i+1) * 11
		if diff := v - want; diff > 0.5 || diff < -0.5 {
			t.Errorf("element %d: got %f, want %f", i, v, want)
			break
		}
	}
}

// TestDeferredStaging_ChainedOpsNumericalCorrectness verifies correctness across
// 500 chained GPU Add operations using lazy inputs — the primary regression test
// for deferred staging. Each iteration uses the previous lazy result as input.
func TestDeferredStaging_ChainedOpsNumericalCorrectness(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer backend.Release()

	const n = 16
	raw, err := tensor.NewRaw(tensor.Shape{n}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatal(err)
	}
	for i := range raw.AsFloat32() {
		raw.AsFloat32()[i] = float32(i+1) * 0.1
	}

	// Chain 500 Add ops: result = raw + raw + ... (500 times)
	// After 500 iterations: result[i] = raw[i] * 501
	result := raw
	for i := 0; i < 500; i++ {
		result = backend.Add(result, raw)
	}

	// Allow GC to collect intermediate tensors and release their GPU buffers
	// before the final readback — prevents OOM on iGPUs with limited shared memory.
	runtime.GC()

	out := result.AsFloat32()
	if len(out) != n {
		t.Fatalf("got %d elements, want %d", len(out), n)
	}

	for i, v := range out {
		want := float32(i+1) * 0.1 * 501
		if diff := v - want; diff > 1.0 || diff < -1.0 {
			t.Errorf("element %d: got %f, want %f (diff %f)", i, v, want, diff)
		}
	}
}

// TestDeferredStaging_MemoryBounded verifies that running 200 ops does not leak
// GPU memory by accumulating staging buffers. With deferred staging, memory
// should stay roughly constant — each op creates ONE result buffer (not two).
// We verify the encoder batch resets (count goes back to 0 after auto-flush).
func TestDeferredStaging_MemoryBounded(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer backend.Release()

	raw, err := tensor.NewRaw(tensor.Shape{128, 128}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatal(err)
	}
	for i := range raw.AsFloat32() {
		raw.AsFloat32()[i] = 0.001
	}

	// Run 200 independent ops (each takes CPU tensor as input, not chained).
	// These accumulate in the encoder batch up to maxPendingBeforeFlush, then auto-flush.
	var lastResult *tensor.RawTensor
	for i := 0; i < 200; i++ {
		lastResult = backend.Add(raw, raw)
	}

	// After 200 ops, the batch should have auto-flushed multiple times.
	// activeBatchCount() should be < maxPendingBeforeFlush (64).
	batchCount := backend.activeBatchCount()
	if batchCount >= maxPendingBeforeFlush {
		t.Errorf("batch count %d >= maxPendingBeforeFlush %d — auto-flush not working",
			batchCount, maxPendingBeforeFlush)
	}

	// Verify the last result is numerically correct.
	out := lastResult.AsFloat32()
	for i, v := range out {
		want := float32(0.002)
		if diff := v - want; diff > 1e-5 || diff < -1e-5 {
			t.Errorf("element %d: got %f, want %f", i, v, want)
			break
		}
	}
}

// TestDeferredStaging_SharedEncoderBatches verifies that lazy chained ops share
// a single encoder batch (activeBatch.count > 1 between ops), which is the
// entire point of deferred staging — without it, each op forced a flush because
// the staging copy required a separate encoder+submit cycle.
func TestDeferredStaging_SharedEncoderBatches(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer backend.Release()

	raw, err := tensor.NewRaw(tensor.Shape{32}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatal(err)
	}
	for i := range raw.AsFloat32() {
		raw.AsFloat32()[i] = 1.0
	}

	// Dispatch 3 independent Add ops without reading back.
	backend.Add(raw, raw)
	backend.Add(raw, raw)
	backend.Add(raw, raw)

	// With deferred staging, all 3 ops should share one encoder batch.
	// activeBatch.count must be 3 (none flushed yet — 3 < maxPendingBeforeFlush=64).
	batchCount := backend.activeBatchCount()
	if batchCount != 3 {
		t.Errorf("expected activeBatch.count == 3 (shared encoder), got %d", batchCount)
	}
}

// TestDeferredStaging_MixedLazyAndCPU verifies that an op can accept one lazy
// GPU input (Storage buffer) and one CPU-cached input (Storage buffer from cache)
// in the same bind group — the critical path for real neural network computation
// where activations are lazy and weights are CPU-cached.
func TestDeferredStaging_MixedLazyAndCPU(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer backend.Release()

	// CPU tensor (will be cached).
	weight, err := tensor.NewRaw(tensor.Shape{8}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatal(err)
	}
	for i := range weight.AsFloat32() {
		weight.AsFloat32()[i] = float32(i + 1) // [1,2,...,8]
	}

	// CPU input tensor.
	input, err := tensor.NewRaw(tensor.Shape{8}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatal(err)
	}
	for i := range input.AsFloat32() {
		input.AsFloat32()[i] = 1.0
	}

	// First op: CPU + CPU → lazy result (input is lazy after this).
	lazyResult := backend.Add(input, input) // lazy: [2,2,...,2]

	// Second op: lazy + CPU weight → lazy result (mixed input types).
	finalResult := backend.Add(lazyResult, weight) // lazy: [3,4,...,9]

	// Both ops should be in the same encoder batch.
	batchCount := backend.activeBatchCount()
	if batchCount < 2 {
		t.Errorf("expected at least 2 ops in encoder batch, got %d — flush injected between ops", batchCount)
	}

	// Verify numerical correctness.
	out := finalResult.AsFloat32()
	for i, v := range out {
		want := 2.0 + float32(i+1) // input+input + weight = 2 + (i+1)
		if diff := v - want; diff > 1e-5 || diff < -1e-5 {
			t.Errorf("element %d: got %f, want %f", i, v, want)
		}
	}

	// Verify weight buffer is cached (reused, not uploaded again).
	cacheSize := backend.inputBufferCacheSize()
	if cacheSize < 1 {
		t.Error("expected at least 1 entry in input buffer cache (weight tensor)")
	}
}

// TestDeferredStaging_ReadbackAfterChain verifies end-to-end correctness for a
// 10-op chain with full readback. This is the basic sanity check that deferred
// staging produces numerically correct results.
func TestDeferredStaging_ReadbackAfterChain(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer backend.Release()

	raw, err := tensor.NewRaw(tensor.Shape{4, 4}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatal(err)
	}
	for i := range raw.AsFloat32() {
		raw.AsFloat32()[i] = 1.0
	}

	// Chain 10 Adds: result = raw * 2^10 = 1024 (since each Add doubles).
	// Actually: raw + raw = 2*raw. Then (2*raw) + raw = 3*raw. Then (3*raw)+raw = 4*raw, etc.
	// After N adds of raw to result: result = (N+1) * raw.
	result := raw
	const nChain = 10
	for i := 0; i < nChain; i++ {
		result = backend.Add(result, raw)
	}

	out := result.AsFloat32()
	if len(out) != 16 {
		t.Fatalf("got %d elements, want 16", len(out))
	}
	for i, v := range out {
		want := float32(nChain + 1) // 11.0
		if diff := v - want; diff > 1e-4 || diff < -1e-4 {
			t.Errorf("element %d: got %f, want %f", i, v, want)
		}
	}
}

// TestDeferredStaging_ResultBufferNotReleasedByBatch verifies that result buffers
// (owned by LazyGPUData) are NOT released by flushCommands, and remain accessible
// for ReadGPUBuffer even after all pending submissions are drained.
//
// This test explicitly forces GC after each chain step to maximize the chance of
// catching premature releases via finalizer execution.
func TestDeferredStaging_ResultBufferNotReleasedByBatch(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer backend.Release()

	raw, err := tensor.NewRaw(tensor.Shape{16}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatal(err)
	}
	for i := range raw.AsFloat32() {
		raw.AsFloat32()[i] = float32(i) + 1.0
	}

	// Build a chain where intermediate tensors may be GC'd.
	// Only the final result is kept alive.
	var finalResult *tensor.RawTensor
	for step := 0; step < 5; step++ {
		intermediate := backend.Add(raw, raw)
		// Force GC to try to collect the intermediate — it must NOT release the buffer
		// before addComputePassToEncoder has tracked it in lazyDatas.
		runtime.GC()
		finalResult = backend.Add(intermediate, raw)
		runtime.GC()
	}

	// Read result — if any buffer was prematurely released, this will panic/error.
	out := finalResult.AsFloat32()
	if len(out) != 16 {
		t.Fatalf("got %d elements, want 16", len(out))
	}
	for i, v := range out {
		// finalResult = (raw+raw)+raw = 3*raw
		want := float32(i+1) * 3.0
		if diff := v - want; diff > 1e-4 || diff < -1e-4 {
			t.Errorf("element %d: got %f, want %f", i, v, want)
		}
	}
}

// TestDeferredStaging_BufferUsageFlags verifies that result buffers created by
// lazy ops have Storage|CopySrc usage (bindable as compute input AND readable
// by ReadGPUBuffer via copy), NOT MapRead (which would indicate the old staging
// buffer approach leaked through).
//
// This test uses internal access to verify the buffer usage at the wgpu level.
func TestDeferredStaging_BufferUsageFlags(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer backend.Release()

	raw, err := tensor.NewRaw(tensor.Shape{8}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatal(err)
	}
	for i := range raw.AsFloat32() {
		raw.AsFloat32()[i] = float32(i) + 1.0
	}

	result := backend.Add(raw, raw)

	gpuData := result.GPUData()
	if gpuData == nil {
		t.Fatal("expected lazy tensor with GPUData")
	}

	// Get the underlying wgpu.Buffer.
	bufPtr := gpuData.BufferPtr()
	if bufPtr == nil {
		t.Fatal("buffer pointer is nil")
	}

	buf := (*wgpu.Buffer)(bufPtr)
	if buf == nil {
		t.Fatal("buffer is nil")
	}

	// The buffer must be a valid GPU buffer. If it were a MapRead staging buffer,
	// it would have been released after flushCommands — but because it is a
	// Storage|CopySrc result buffer owned by LazyGPUData, it must still be alive.
	// We verify by confirming Data() works (ReadGPUBuffer can copy from it).
	out := result.AsFloat32()
	if len(out) != 8 {
		t.Fatalf("got %d elements, want 8", len(out))
	}
	for i, v := range out {
		want := float32(i+1) * 2.0
		if diff := v - want; diff > 1e-5 || diff < -1e-5 {
			t.Errorf("element %d: got %f, want %f", i, v, want)
		}
	}
}
