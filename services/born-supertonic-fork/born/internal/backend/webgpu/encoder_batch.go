//go:build windows || linux

// Package webgpu implements the WebGPU backend for GPU-accelerated tensor operations.
package webgpu

import (
	"fmt"
	"runtime"

	"github.com/born-ml/born/internal/tensor"
	"github.com/gogpu/gputypes"
	wgpu "github.com/gogpu/wgpu"
)

// ─────────────────────────────────────────────────────────────────────────────
// Shared encoder accumulator (Part 2)
// ─────────────────────────────────────────────────────────────────────────────

// getOrCreateEncoderLocked returns the active CommandEncoder, creating one if
// necessary. MUST be called with pendingMu held.
func (b *Backend) getOrCreateEncoderLocked() *wgpu.CommandEncoder {
	if b.activeBatch.encoder == nil {
		var err error
		b.activeBatch.encoder, err = b.device.CreateCommandEncoder(nil)
		if err != nil {
			panic("webgpu: CreateCommandEncoder: " + err.Error())
		}
	}
	return b.activeBatch.encoder
}

// finishActiveBatchLocked finalizes the active encoder — finishing it into a
// CommandBuffer and appending it to b.pending. Resets activeBatch to zero value.
//
// No CopyBufferToBuffer entries are emitted here: with deferred staging, result
// buffers are owned by LazyGPUData and the copy to a staging buffer happens
// on-demand inside ReadGPUBuffer when Data() is accessed.
//
// MUST be called with pendingMu held. Safe to call when activeBatch.encoder is
// nil (no-op).
func (b *Backend) finishActiveBatchLocked() {
	if b.activeBatch.encoder == nil {
		return
	}

	cmdBuffer, err := b.activeBatch.encoder.Finish()
	if err != nil {
		// On Finish error, release everything to avoid leaks and panic loudly.
		for _, buf := range b.activeBatch.resultBufs {
			buf.Release()
		}
		for _, bg := range b.activeBatch.bindGroups {
			bg.Release()
		}
		b.activeBatch = encoderBatch{}
		panic("webgpu: finishActiveBatch: encoder Finish: " + err.Error())
	}

	b.pending = append(b.pending, pendingSubmission{
		cmdBuffer:  cmdBuffer,
		resultBufs: b.activeBatch.resultBufs,
		bindGroups: b.activeBatch.bindGroups,
		lazyDatas:  b.activeBatch.lazyDatas,
	})

	b.activeBatch = encoderBatch{}
}

// addComputePassToEncoder encodes a single compute dispatch into the shared
// encoder accumulator.
//
// Deferred staging: resultBuf (Storage|CopySrc) is passed directly to
// createLazyResult and owned by the returned LazyGPUData. No staging buffer
// is created here — the MapRead staging buffer is allocated on demand inside
// ReadGPUBuffer when Data() is called on the lazy tensor.
//
// To prevent the GC from releasing resultBuf (via LazyGPUData finalizer) while
// the command buffer referencing it is still in the pending queue, the
// LazyGPUData pointer is added to activeBatch.lazyDatas. After Submit the
// reference is dropped, allowing GC to reclaim it normally.
//
// res.buffers (params + transient input copies) are tracked for post-Submit
// release. res.bindGroups must NOT be defer-released in caller.
func (b *Backend) addComputePassToEncoder(
	pipeline *wgpu.ComputePipeline,
	bg *wgpu.BindGroup,
	workgroupsX, workgroupsY, workgroupsZ uint32,
	resultBuf *wgpu.Buffer,
	resultSize uint64,
	shape tensor.Shape,
	dtype tensor.DataType,
	res lazyResources,
) (*tensor.RawTensor, error) {
	// Create lazy tensor first (outside the lock) so we have the LazyGPUData
	// pointer to add to the batch for GC-safety.
	lazyTensor, err := b.createLazyResult(resultBuf, resultSize, shape, dtype)
	if err != nil {
		// createLazyResult already released resultBuf on failure.
		for _, buf := range res.buffers {
			buf.Release()
		}
		bg.Release()
		return nil, err
	}
	gpuData := lazyTensor.GPUData() // non-nil: just created

	b.pendingMu.Lock()

	enc := b.getOrCreateEncoderLocked()

	computePass, cpErr := enc.BeginComputePass(nil)
	if cpErr != nil {
		b.pendingMu.Unlock()
		// Release caller-owned resources on failure.
		// resultBuf is now owned by lazyTensor (via LazyGPUData) — do not release here.
		for _, buf := range res.buffers {
			buf.Release()
		}
		bg.Release()
		// lazyTensor will be GC'd and its finalizer will release resultBuf.
		return nil, fmt.Errorf("addComputePassToEncoder: BeginComputePass: %w", cpErr)
	}
	computePass.SetPipeline(pipeline)
	computePass.SetBindGroup(0, bg, nil)
	computePass.Dispatch(workgroupsX, workgroupsY, workgroupsZ)
	if endErr := computePass.End(); endErr != nil {
		b.pendingMu.Unlock()
		for _, buf := range res.buffers {
			buf.Release()
		}
		bg.Release()
		panic(fmt.Sprintf("webgpu: addComputePassToEncoder: compute pass End: %v", endErr))
	}

	// Track params + transient input copies for post-Submit release.
	// Track gpuData (result) + res.lazyDatas (inputs) to prevent GC from
	// running their finalizers and releasing the referenced buffers before Submit.
	b.activeBatch.resultBufs = append(b.activeBatch.resultBufs, res.buffers...)
	b.activeBatch.bindGroups = append(b.activeBatch.bindGroups, bg)
	b.activeBatch.bindGroups = append(b.activeBatch.bindGroups, res.bindGroups...)
	b.activeBatch.lazyDatas = append(b.activeBatch.lazyDatas, gpuData)
	b.activeBatch.lazyDatas = append(b.activeBatch.lazyDatas, res.lazyDatas...)
	b.activeBatch.count++
	// Track memory: resultBuf only (staging created on demand in ReadGPUBuffer).
	b.activeBatch.allocBytes += resultSize
	for range res.buffers {
		b.activeBatch.allocBytes += 64 // params are typically 16-64 bytes
	}

	// Auto-flush on EITHER count threshold (TDR safety) OR memory threshold (OOM safety).
	shouldFlush := b.activeBatch.count >= maxPendingBeforeFlush || b.activeBatch.allocBytes >= maxBatchAllocBytes

	if shouldFlush {
		b.finishActiveBatchLocked()
	}

	b.pendingMu.Unlock()

	if shouldFlush {
		b.flushCommands()
	}

	return lazyTensor, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Input buffer cache (Part 1)
// ─────────────────────────────────────────────────────────────────────────────

// getOrCreateInputBuffer returns a GPU storage buffer for the given tensor.
//
// For CPU tensors (t.GPUData() == nil): creates and caches a Storage|CopySrc
// buffer on first access. Subsequent calls return the cached buffer without
// re-uploading. Cached buffers are never released by finishActiveBatchLocked
// — they are released in clearInputBufferCache() from Backend.Release().
//
// For lazy GPU tensors (t.GPUData() != nil && !IsRealized()): returns the
// existing result buffer directly (cached:true), without any GPU→GPU copy.
// The result buffer is Storage|CopySrc and can be bound directly as a
// compute shader input. Ownership remains with LazyGPUData.
//
// Thread-safe: uses a separate RWMutex (inputBufferCache.mu) so cache reads
// do not contend with pendingMu.
// inputBufferResult holds the result of getOrCreateInputBuffer.
type inputBufferResult struct {
	buffer  *wgpu.Buffer
	cached  bool                // true = owned by cache or LazyGPUData; false = caller must release
	gpuData *tensor.LazyGPUData // non-nil for lazy GPU tensors; must be tracked for GC-safety
}

func (b *Backend) getOrCreateInputBuffer(t *tensor.RawTensor) inputBufferResult {
	if gpuData := t.GPUData(); gpuData != nil && !gpuData.IsRealized() {
		if bp := gpuData.BufferPtr(); bp != nil {
			existingBuffer := (*wgpu.Buffer)(bp)
			runtime.KeepAlive(gpuData)
			return inputBufferResult{buffer: existingBuffer, cached: true, gpuData: gpuData}
		}
	}

	// CPU tensor: check cache.
	b.inputBufferCache.mu.RLock()
	if cb, ok := b.inputBufferCache.cache[t]; ok {
		b.inputBufferCache.mu.RUnlock()
		return inputBufferResult{buffer: cb.buffer, cached: true}
	}
	b.inputBufferCache.mu.RUnlock()

	// Cache miss — upload.
	buf := b.createBuffer(t.Data(), gputypes.BufferUsageStorage|gputypes.BufferUsageCopySrc)
	size := uint64(t.ByteSize()) //nolint:gosec // G115: ByteSize is non-negative

	b.inputBufferCache.mu.Lock()
	if b.inputBufferCache.cache == nil {
		b.inputBufferCache.cache = make(map[*tensor.RawTensor]*cachedBuffer)
	}
	if existing, ok := b.inputBufferCache.cache[t]; ok {
		b.inputBufferCache.mu.Unlock()
		buf.Release()
		return inputBufferResult{buffer: existing.buffer, cached: true}
	}
	b.inputBufferCache.cache[t] = &cachedBuffer{buffer: buf, size: size}
	b.inputBufferCache.mu.Unlock()

	return inputBufferResult{buffer: buf, cached: true}
}

// clearInputBufferCache releases all cached GPU buffers and clears the cache.
// Called automatically from Backend.Release(). May also be called explicitly
// between training steps when weight tensors change (e.g. after optimizer.Step
// replaces tensors with new objects).
func (b *Backend) clearInputBufferCache() {
	b.inputBufferCache.mu.Lock()
	defer b.inputBufferCache.mu.Unlock()

	for _, cb := range b.inputBufferCache.cache {
		cb.buffer.Release()
	}
	b.inputBufferCache.cache = nil
}

// ClearInputBufferCache is the exported version of clearInputBufferCache.
// Call this between training steps if weight tensors are replaced by new
// *RawTensor objects (e.g. after in-place optimizer updates that produce new
// tensors). Forgetting to call this after weight replacement causes stale
// GPU data to be used in forward passes.
func (b *Backend) ClearInputBufferCache() {
	b.clearInputBufferCache()
}

// inputBufferCacheSize returns the current number of entries in the input
// buffer cache. Intended for testing and observability only.
func (b *Backend) inputBufferCacheSize() int {
	b.inputBufferCache.mu.RLock()
	defer b.inputBufferCache.mu.RUnlock()
	return len(b.inputBufferCache.cache)
}

// activeBatchCount returns the number of compute passes currently accumulated
// in the active (not-yet-flushed) encoder batch. Intended for testing only.
func (b *Backend) activeBatchCount() int {
	b.pendingMu.Lock()
	defer b.pendingMu.Unlock()
	return b.activeBatch.count
}
