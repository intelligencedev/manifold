//go:build wasm

package tensor

import "unsafe"

// LazyBackend is an interface for backends that support lazy GPU evaluation.
// On WASM, GPU operations are handled by the browser's WebGPU API through
// a separate path — this stub satisfies the compiler.
type LazyBackend interface {
	ReadGPUBuffer(bufferPtr unsafe.Pointer, size uint64) ([]byte, error)
	ReleaseGPUBuffer(bufferPtr unsafe.Pointer)
	DeferReleaseGPUBuffer(bufferPtr unsafe.Pointer)
	RegisterLiveGPU(l *LazyGPUData)
	UnregisterLiveGPU(l *LazyGPUData)
}

// LazyGPUData holds a reference to GPU-resident data for lazy evaluation.
// On WASM, this is a placeholder — browser WebGPU uses a different data path.
type LazyGPUData struct {
	persistent bool
	refCount   int32
}

// NewLazyGPUData creates a new LazyGPUData (stub for WASM).
func NewLazyGPUData(_ unsafe.Pointer, _ any, _ uint64, _ LazyBackend) *LazyGPUData {
	return &LazyGPUData{refCount: 1}
}

// IsRealized returns whether the GPU data has been transferred to CPU.
func (l *LazyGPUData) IsRealized() bool { return true }

// MarkRealized marks the GPU data as realized.
func (l *LazyGPUData) MarkRealized() {}

// Realize transfers data from GPU to CPU (no-op on WASM).
func (l *LazyGPUData) Realize() ([]byte, error) { return nil, nil }

// Release decrements refcount (no-op on WASM).
func (l *LazyGPUData) Release() {}

// ScheduleRelease queues deferred release (no-op on WASM).
func (l *LazyGPUData) ScheduleRelease() {}

// AddRef increments the reference count.
func (l *LazyGPUData) AddRef() {}

// RefCount returns the number of references.
func (l *LazyGPUData) RefCount() int32 { return l.refCount }

// SetPersistent marks this GPU data as persistent.
func (l *LazyGPUData) SetPersistent(persistent bool) { l.persistent = persistent }

// IsPersistent returns whether this GPU data is marked as persistent.
func (l *LazyGPUData) IsPersistent() bool { return l.persistent }

// BufferPtr returns the underlying GPU buffer pointer (nil on WASM).
func (l *LazyGPUData) BufferPtr() unsafe.Pointer { return nil }

// Size returns the buffer size in bytes (0 on WASM).
func (l *LazyGPUData) Size() uint64 { return 0 }

// NewLazyRaw creates a new RawTensor with lazy GPU data (stub for WASM).
func NewLazyRaw(shape Shape, dtype DataType, device Device, _ *LazyGPUData) (*RawTensor, error) {
	return NewRaw(shape, dtype, device)
}
