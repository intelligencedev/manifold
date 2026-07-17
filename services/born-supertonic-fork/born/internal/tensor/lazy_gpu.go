//go:build !wasm

// Package tensor provides tensor data structures for the Born ML framework.
package tensor

import (
	"sync"
	"unsafe"
)

// LazyBackend is an interface for backends that support lazy GPU evaluation.
// The backend must implement ReadGPUBuffer to transfer data from GPU to CPU.
type LazyBackend interface {
	// ReadGPUBuffer reads data from a GPU buffer to CPU memory.
	ReadGPUBuffer(bufferPtr unsafe.Pointer, size uint64) ([]byte, error)

	// ReleaseGPUBuffer releases the GPU buffer when no longer needed.
	ReleaseGPUBuffer(bufferPtr unsafe.Pointer)

	// DeferReleaseGPUBuffer queues a GPU buffer for release after the next
	// flushCommands/Submit cycle. Used when a buffer may still be referenced
	// by pending command buffers in the shared encoder batch.
	DeferReleaseGPUBuffer(bufferPtr unsafe.Pointer)

	// RegisterLiveGPU registers a LazyGPUData with the backend's live tensor
	// set. UnregisterLiveGPU removes it. ReclaimMemory releases all registered
	// tensors that are not marked persistent. This enables deterministic GPU
	// memory reclamation for ALL intermediate tensors — including those created
	// outside the autodiff tape (NoGrad blocks, carry state, masks).
	RegisterLiveGPU(l *LazyGPUData)
	UnregisterLiveGPU(l *LazyGPUData)
}

// LazyGPUData holds a reference to GPU-resident data for lazy evaluation.
// When Data() is called on a RawTensor with LazyGPUData, the data is
// transferred from GPU to CPU only at that point (lazy realization).
type LazyGPUData struct {
	bufferPtr  unsafe.Pointer // Pointer to GPU buffer (*wgpu.Buffer) for unsafe casting
	bufferRef  any            // Strong reference to *wgpu.Buffer — prevents GC collection
	size       uint64         // Buffer size in bytes
	backend    LazyBackend    // Backend for reading/releasing buffer
	realized   bool           // Whether data has been transferred to CPU
	persistent bool           // If true, ReclaimMemory skips this tensor
	refCount   int32          // Number of RawTensors sharing this LazyGPUData (Clone/Detach)
	mu         sync.Mutex     // Protects realized flag and transfer
}

// NewLazyGPUData creates a new LazyGPUData referencing a GPU buffer.
// bufferPtr is the unsafe.Pointer for casting in Read/Release operations.
// bufferRef is a strong reference (typically *wgpu.Buffer) that prevents
// the Go GC from collecting the buffer object while LazyGPUData is alive.
// Without bufferRef, the GC would collect *wgpu.Buffer immediately after
// createLazyResult returns (unsafe.Pointer does not prevent GC collection),
// triggering wgpu's runtime.AddCleanup prematurely.
func NewLazyGPUData(bufferPtr unsafe.Pointer, bufferRef any, size uint64, backend LazyBackend) *LazyGPUData {
	l := &LazyGPUData{
		bufferPtr: bufferPtr,
		bufferRef: bufferRef,
		size:      size,
		backend:   backend,
		refCount:  1,
		realized:  false,
	}
	backend.RegisterLiveGPU(l)
	return l
}

// IsRealized returns whether the GPU data has been transferred to CPU.
func (l *LazyGPUData) IsRealized() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.realized
}

// MarkRealized marks the GPU data as realized (transferred to CPU).
func (l *LazyGPUData) MarkRealized() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.realized = true
}

// Realize transfers data from GPU to CPU and returns it.
// This is called lazily when Data() is accessed.
// Thread-safe: multiple goroutines can safely call this.
// After realization, the GPU buffer is released to free GPU memory.
func (l *LazyGPUData) Realize() ([]byte, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Already realized - this shouldn't happen but handle gracefully
	if l.realized {
		return nil, nil
	}

	// Read data from GPU. The mutex held above keeps this LazyGPUData
	// alive on the call stack for the duration of ReadGPUBuffer.
	data, err := l.backend.ReadGPUBuffer(l.bufferPtr, l.size)
	if err != nil {
		return nil, err
	}

	l.realized = true

	// Release GPU buffer after copying to CPU - we don't need it anymore
	if l.bufferPtr != nil && l.backend != nil {
		l.backend.UnregisterLiveGPU(l)
		l.backend.ReleaseGPUBuffer(l.bufferPtr)
		l.bufferPtr = nil
		l.bufferRef = nil
	}

	return data, nil
}

// Release decrements refcount and releases the GPU buffer when no references remain.
func (l *LazyGPUData) Release() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.refCount--
	if l.refCount > 0 {
		return
	}

	if l.bufferPtr != nil && l.backend != nil {
		l.backend.UnregisterLiveGPU(l)
		l.backend.ReleaseGPUBuffer(l.bufferPtr)
		l.bufferPtr = nil
		l.bufferRef = nil
	}
}

// ScheduleRelease queues the GPU buffer for deferred release after the next
// flushCommands/Submit cycle.
//
// Deferred release is required because Born creates bind groups referencing the
// buffer BEFORE the compute pass is encoded and submitted. wgpu Phase 2
// (SetBindGroup Clone) only protects buffers AFTER SetBindGroup is called.
// If Release fires before Submit, the command buffer references a destroyed buffer.
//
// With wgpu v0.28.9+ onZero callback: once the deferred Release fires after Submit,
// Phase 2 tracked refs keep the buffer alive if GPU is still using it.
func (l *LazyGPUData) ScheduleRelease() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.refCount--
	if l.refCount > 0 {
		return // other RawTensors still reference this GPU data
	}

	if l.bufferPtr != nil && l.backend != nil {
		l.backend.UnregisterLiveGPU(l)
		l.backend.DeferReleaseGPUBuffer(l.bufferPtr)
		l.bufferPtr = nil
		l.bufferRef = nil
	}
}

// AddRef increments the reference count. Called when RawTensor.Clone() shares this LazyGPUData.
func (l *LazyGPUData) AddRef() {
	l.mu.Lock()
	l.refCount++
	l.mu.Unlock()
}

// RefCount returns the number of RawTensors sharing this GPU data.
func (l *LazyGPUData) RefCount() int32 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.refCount
}

// SetPersistent marks this GPU data as persistent. Persistent tensors
// survive ReclaimMemory — they are NOT released when the backend drains
// the live tensor registry. Use for optimizer moments, model parameters,
// and any tensor that must persist across training steps.
func (l *LazyGPUData) SetPersistent(persistent bool) {
	l.mu.Lock()
	l.persistent = persistent
	l.mu.Unlock()
}

// IsPersistent returns whether this GPU data is marked as persistent.
func (l *LazyGPUData) IsPersistent() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.persistent
}

// BufferPtr returns the underlying GPU buffer pointer.
// This is used by backend operations that need to chain GPU operations.
func (l *LazyGPUData) BufferPtr() unsafe.Pointer {
	return l.bufferPtr
}

// Size returns the buffer size in bytes.
func (l *LazyGPUData) Size() uint64 {
	return l.size
}

// NewLazyRaw creates a new RawTensor with lazy GPU data.
// The data is not transferred from GPU until Data() is called.
func NewLazyRaw(shape Shape, dtype DataType, device Device, gpuData *LazyGPUData) (*RawTensor, error) {
	if err := shape.Validate(); err != nil {
		return nil, err
	}

	// NO CPU buffer allocation — data lives on GPU only.
	// CPU buffer is allocated lazily on first Data() call (GPU→CPU readback).
	// This prevents 18K × 100KB = 1.8GB CPU RAM waste per training step
	// for intermediate tensors that never leave the GPU.
	return &RawTensor{
		buffer:  nil,
		shape:   shape.Clone(),
		stride:  shape.ComputeStrides(),
		dtype:   dtype,
		device:  device,
		offset:  0,
		gpuData: gpuData,
	}, nil
}
