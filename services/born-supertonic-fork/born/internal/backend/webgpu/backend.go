//go:build windows || linux

// Package webgpu implements the WebGPU backend for GPU-accelerated tensor operations.
// Uses gogpu/wgpu (github.com/gogpu/wgpu) for pure Go, zero-CGO WebGPU bindings.
package webgpu

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"
	"unsafe"

	"github.com/born-ml/born/internal/tensor"
	"github.com/gogpu/gputypes"
	wgpu "github.com/gogpu/wgpu"
	_ "github.com/gogpu/wgpu/hal/allbackends"
	"github.com/gogpu/wgpu/hal/software"
)

// pipelineEntry caches a compute pipeline together with its layouts.
// The pipeline layout must remain alive because Vulkan references it
// during vkCmdBindDescriptorSets on every SetBindGroup call.
type pipelineEntry struct {
	pipeline       *wgpu.ComputePipeline
	layout         *wgpu.BindGroupLayout
	pipelineLayout *wgpu.PipelineLayout
}

// pendingSubmission holds a finished command buffer that has not yet been
// submitted to the GPU queue, plus the intermediate buffers that must
// remain alive until after queue.Submit returns (BUG-LAZY-DEFER-RELEASE).
//
// lazyDatas holds LazyGPUData references for all result tensors in this batch.
// This prevents GC from collecting LazyGPUData (and releasing their result
// buffers via finalizer) while the command buffer that writes those buffers
// is still pending. After Submit, the references are dropped so GC can
// reclaim them when the lazy tensors are no longer needed.
//
// Populated by finishActiveBatchLocked (via addComputePassToEncoder) when
// the shared encoder is sealed. All pending submissions are flushed in a
// single queue.Submit call when any tensor's Data() triggers ReadGPUBuffer.
type pendingSubmission struct {
	cmdBuffer  *wgpu.CommandBuffer
	resultBufs []*wgpu.Buffer        // params + transient inputs, released after Submit
	bindGroups []*wgpu.BindGroup     // released after Submit
	lazyDatas  []*tensor.LazyGPUData // kept alive until after Submit; NOT released here
}

// encoderBatch accumulates multiple compute passes into a single CommandEncoder.
// All passes are recorded into the same encoder; flush (Finish+Submit) happens
// at threshold or on Data() access. This eliminates per-op CreateCommandEncoder
// overhead and reduces driver synchronization points.
//
// resultBufs holds params + transient input copies — NOT compute result buffers.
// lazyDatas holds LazyGPUData refs to keep result buffers alive until Submit.
type encoderBatch struct {
	encoder    *wgpu.CommandEncoder
	resultBufs []*wgpu.Buffer        // params + transient inputs, released after Submit
	bindGroups []*wgpu.BindGroup     // released after Submit
	lazyDatas  []*tensor.LazyGPUData // kept alive until Submit; dropped after (not released)
	count      int
	allocBytes uint64 // total GPU bytes held by this batch
}

// maxBatchAllocBytes limits GPU memory held by a single encoder batch.
// Prevents OOM on iGPUs with shared memory (Iris Xe ~4GB usable).
// 16MB is conservative — allows ~500 ops of [512,512] tensors.
// Configurable via BORN_MAX_BATCH_MB environment variable.
var maxBatchAllocBytes = uint64(getEnvIntOrBackend("BORN_MAX_BATCH_MB", 16)) * 1024 * 1024

func getEnvIntOrBackend(key string, defaultVal int) int {
	if s, ok := os.LookupEnv(key); ok {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			return v
		}
	}
	return defaultVal
}

// cachedBuffer holds a GPU storage buffer that was created from a CPU RawTensor
// and is kept alive across multiple op invocations.
type cachedBuffer struct {
	buffer *wgpu.Buffer
	size   uint64
}

// Backend implements tensor operations on GPU using WebGPU.
type Backend struct {
	instance *wgpu.Instance
	adapter  *wgpu.Adapter
	device   *wgpu.Device
	queue    *wgpu.Queue

	// Shader and pipeline cache
	shaders   map[string]*wgpu.ShaderModule
	pipelines map[string]pipelineEntry
	mu        sync.RWMutex

	// Device info
	adapterInfo *wgpu.AdapterInfo

	// Buffer pool for memory management
	bufferPool *BufferPool

	// gpuPool reuses GPU result buffers across training steps (ADR-016/017).
	// Multi-tier pool following Burn/CubeCL MemoryManagement pattern:
	// log-spaced bucket sizes from 32KB to MaxStorageBufferBindingSize,
	// each tier with its own dealloc period. Budget-aware via BORN_GPU_BUDGET_MB.
	gpuPool *TieredPool

	// Lazy mode: when true, operations return lazy tensors that keep data on GPU
	// until Data() is explicitly called. This is the key optimization for
	// Phase 3 Integration - eliminates readBuffer() bottleneck.
	// Default: true for optimal performance.
	LazyMode bool

	// Pending command buffers queued for batched submission.
	// Populated by addComputePassToEncoder; drained by flushCommands.
	// Protected by pendingMu. This is the core of the batched-dispatch
	// optimization: instead of 1 Submit per op (~500 µs each), all pending
	// command buffers are submitted in a single queue.Submit call when the
	// first Data() access triggers ReadGPUBuffer.
	pending   []pendingSubmission
	pendingMu sync.Mutex

	// activeBatch accumulates compute passes into a single shared CommandEncoder.
	// Protected by pendingMu (same lock as pending to avoid ordering issues).
	// Flushed (Finish+Submit) when count reaches maxPendingBeforeFlush or when
	// flushCommands/copyGPUBuffer/execComputeAndRead need a clean GPU state.
	activeBatch encoderBatch

	// inputBufferCache caches GPU storage buffers created from CPU RawTensors.
	// Keyed by *RawTensor pointer identity — reuses the same GPU buffer when the
	// same weight tensor (e.g. Linear.weight) is passed to multiple ops in a
	// forward+backward pass, avoiding redundant host→device uploads.
	//
	// ONLY CPU tensors are cached. Lazy (GPU) tensors are transient intermediates
	// whose staging buffers are released after readback — they must NOT be cached.
	//
	// Cleared in Backend.Release() and can be cleared manually via
	// ClearInputBufferCache() between training steps if weights change.
	inputBufferCache struct {
		mu    sync.RWMutex
		cache map[*tensor.RawTensor]*cachedBuffer
	}

	// scalarCache caches GPU buffers for small constant tensors (e.g. -1.0, 1.0)
	// used repeatedly in backward ops. Keyed by (float32Value, numElements, dtype)
	// encoded as a uint64 to avoid struct allocation on the hot path.
	// Buffers are owned by the cache and released in Backend.Release().
	// Protected by scalarCacheMu.
	scalarCache   map[uint64]*GPUTensor
	scalarCacheMu sync.RWMutex

	// liveGPU tracks ALL LazyGPUData created by this backend. Every
	// createLazyResult registers here; Release/ScheduleRelease/Realize
	// unregisters. ReclaimMemory releases all remaining entries — this is
	// the mechanism for reclaiming GPU memory from tensors created outside
	// the autodiff tape (NoGrad blocks, carry state, masks, metrics).
	liveGPU struct {
		mu      sync.Mutex
		tensors map[*tensor.LazyGPUData]struct{}
	}

	// Memory tracking
	memoryStats struct {
		totalAllocatedBytes uint64
		peakMemoryBytes     uint64
		activeBuffers       int64
		mu                  sync.RWMutex
	}
}

// New creates a new WebGPU backend.
//
// Backend selection via GOGPU_GRAPHICS_API environment variable (gogpu ecosystem standard):
//
//	GOGPU_GRAPHICS_API=auto      (default) wgpu selects best available
//	GOGPU_GRAPHICS_API=vulkan    force Vulkan
//	GOGPU_GRAPHICS_API=dx12      force DirectX 12
//	GOGPU_GRAPHICS_API=metal     force Metal
//	GOGPU_GRAPHICS_API=gl        force OpenGL/ES
//	GOGPU_GRAPHICS_API=software  software compute — no GPU required (CI, testing)
func New() (*Backend, error) {
	api := os.Getenv("GOGPU_GRAPHICS_API")

	if api == "software" {
		return newSoftwareBackend()
	}

	backends := wgpu.BackendsPrimary
	switch api {
	case "vulkan", "vk":
		backends = wgpu.BackendsVulkan
	case "dx12", "d3d12":
		backends = wgpu.BackendsDX12
	case "metal":
		backends = wgpu.BackendsMetal
	case "gl", "gles":
		backends = wgpu.BackendsGL
	}

	b, err := newHardwareBackend(backends)
	if err != nil && api == "" {
		// Auto mode: hardware failed, fall back to software.
		return newSoftwareBackend()
	}
	return b, err
}

func newHardwareBackend(backends wgpu.Backends) (*Backend, error) {
	instance, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{
		Backends: backends,
	})
	if err != nil {
		return nil, fmt.Errorf("webgpu: failed to create instance: %w", err)
	}

	adapter, err := instance.RequestAdapter(&wgpu.RequestAdapterOptions{
		PowerPreference: gputypes.PowerPreferenceHighPerformance,
	})
	if err != nil {
		instance.Release()
		return nil, fmt.Errorf("webgpu: failed to request adapter: %w", err)
	}

	info := adapter.Info()

	device, err := adapter.RequestDevice(nil)
	if err != nil {
		adapter.Release()
		instance.Release()
		return nil, fmt.Errorf("webgpu: failed to request device: %w", err)
	}

	queue := device.Queue()
	if queue == nil {
		device.Release()
		adapter.Release()
		instance.Release()
		return nil, fmt.Errorf("webgpu: failed to get queue")
	}

	return newBackendFromDevice(instance, adapter, device, queue, &info)
}

// newSoftwareBackend creates a Backend using the software HAL — CPU-based
// compute with SPIR-V interpreter. No GPU hardware required.
// Used when GOGPU_GRAPHICS_API=software (CI, testing, headless).
func newSoftwareBackend() (*Backend, error) {
	api := software.API{}
	inst, err := api.CreateInstance(nil)
	if err != nil {
		return nil, fmt.Errorf("webgpu: software CreateInstance: %w", err)
	}
	adapters := inst.EnumerateAdapters(nil)
	if len(adapters) == 0 {
		inst.Destroy()
		return nil, fmt.Errorf("webgpu: software backend: no adapters")
	}
	openDev, err := adapters[0].Adapter.Open(0, gputypes.DefaultLimits())
	if err != nil {
		inst.Destroy()
		return nil, fmt.Errorf("webgpu: software Open: %w", err)
	}

	device, err := wgpu.NewDeviceFromHAL(
		openDev.Device, openDev.Queue,
		gputypes.Features(0), gputypes.DefaultLimits(),
		"born-software",
	)
	if err != nil {
		openDev.Device.Destroy()
		inst.Destroy()
		return nil, fmt.Errorf("webgpu: software NewDeviceFromHAL: %w", err)
	}

	queue := device.Queue()
	info := wgpu.AdapterInfo{Name: "Software Renderer", DeviceType: gputypes.DeviceTypeCPU}
	return newBackendFromDevice(nil, nil, device, queue, &info)
}

func newBackendFromDevice(instance *wgpu.Instance, adapter *wgpu.Adapter, device *wgpu.Device, queue *wgpu.Queue, info *wgpu.AdapterInfo) (*Backend, error) {
	b := &Backend{
		instance:    instance,
		adapter:     adapter,
		device:      device,
		queue:       queue,
		shaders:     make(map[string]*wgpu.ShaderModule),
		pipelines:   make(map[string]pipelineEntry),
		adapterInfo: info,
		bufferPool:  NewBufferPool(device),
		LazyMode:    true,
		gpuPool:     nil,
		scalarCache: make(map[uint64]*GPUTensor),
	}
	b.liveGPU.tensors = make(map[*tensor.LazyGPUData]struct{}, 1024)

	pool := NewTieredPool(device)
	pool.onOOM = func() {
		b.flushCommands()
		b.device.Poll(wgpu.PollWait)
	}
	b.gpuPool = pool

	return b, nil
}

// SetLazyMode enables or disables lazy evaluation mode.
// When enabled (default), operations return lazy tensors that keep data on GPU
// until Data() is explicitly called. This dramatically improves performance
// by eliminating unnecessary GPU→CPU transfers.
// When disabled, operations immediately transfer results to CPU (slower).
func (b *Backend) SetLazyMode(enabled bool) {
	b.LazyMode = enabled
}

// flushCommands submits all pending command buffers to the GPU in a single
// queue.Submit call, then releases the intermediate result buffers that were
// kept alive for the duration of the submission.
//
// Called by ReadGPUBuffer before Map — ensures the GPU has received all
// commands that produce data for the staging buffer being mapped.
//
// If there are no pending submissions, this is a fast no-op (single mutex
// acquire + nil check).
func (b *Backend) flushCommands() {
	b.pendingMu.Lock()
	// Finish any active encoder first — its command buffer must be in b.pending
	// before we drain the pending slice below.
	b.finishActiveBatchLocked()
	if len(b.pending) == 0 {
		b.pendingMu.Unlock()
		return
	}
	pending := b.pending
	b.pending = nil
	b.pendingMu.Unlock()

	// Collect all command buffers for a single Submit call.
	// queue.Submit is variadic: Submit(cmds ...*CommandBuffer).
	cmdBufs := make([]*wgpu.CommandBuffer, len(pending))
	for i, p := range pending {
		cmdBufs[i] = p.cmdBuffer
	}

	if _, err := b.queue.Submit(cmdBufs...); err != nil {
		panic("webgpu: flushCommands: submit failed: " + err.Error())
	}

	// Return result buffers to pool for reuse (ADR-016) now that Submit
	// has registered them with wgpu's Phase 2 tracked refs. Pool marks them
	// as free; next Acquire reuses them without device.CreateBuffer().
	// Params/uniform buffers and bind groups are NOT pooled — released directly.
	for _, p := range pending {
		for _, buf := range p.resultBufs {
			b.gpuPool.Release(buf)
		}
		for _, bg := range p.bindGroups {
			bg.Release()
		}
	}

	// Periodic GPU drain: every 8th flush, block until GPU completes all
	// pending work so DestroyQueue.Triage can free HAL buffers. Without this,
	// PollPoll (non-blocking) may never drain completions during a tight
	// compute loop, and all buffers stay HAL-alive until ReclaimMemory.
	// 8 flushes × 32 ops/flush = 256 ops between drains — balances throughput
	// and memory on constrained iGPUs (5GB Iris Xe).
	b.device.Poll(wgpu.PollPoll)
}

// Release releases all WebGPU resources.
// Must be called when the backend is no longer needed.
func (b *Backend) Release() {
	// Phase 1: Submit all pending GPU commands so resources are no longer
	// referenced by command buffers. Releases BindGroups + transient buffers.
	b.flushCommands()

	// Phase 2: Release all tracked GPU resources back to pool.
	b.liveGPU.mu.Lock()
	toRelease := make([]*tensor.LazyGPUData, 0, len(b.liveGPU.tensors))
	for l := range b.liveGPU.tensors {
		toRelease = append(toRelease, l)
	}
	b.liveGPU.tensors = nil
	b.liveGPU.mu.Unlock()
	for _, l := range toRelease {
		l.Release()
	}

	b.clearInputBufferCache()

	b.scalarCacheMu.Lock()
	for _, t := range b.scalarCache {
		t.Release()
	}
	b.scalarCache = nil
	b.scalarCacheMu.Unlock()

	b.mu.Lock()
	defer b.mu.Unlock()

	// Phase 3: Destroy pools — actually release GPU buffers to wgpu.
	if b.gpuPool != nil {
		b.gpuPool.Destroy()
		b.gpuPool = nil
	}
	if b.bufferPool != nil {
		b.bufferPool.Clear()
		b.bufferPool = nil
	}

	// Release pipelines and their associated layouts.
	for _, entry := range b.pipelines {
		entry.pipeline.Release()
		entry.layout.Release()
		if entry.pipelineLayout != nil {
			entry.pipelineLayout.Release()
		}
	}
	b.pipelines = nil

	// Release shaders
	for _, s := range b.shaders {
		s.Release()
	}
	b.shaders = nil

	// Phase 4: Block until GPU processes all deferred destructions.
	// Without this, the Vulkan driver may not reclaim memory before device
	// teardown, causing vkMapMemory failures in subsequent tests.
	if b.device != nil {
		b.device.Poll(wgpu.PollWait)
	}

	// Release WebGPU objects.
	// Note: Queue is owned by Device in gogpu/wgpu and released via device.Release().
	b.queue = nil
	if b.device != nil {
		b.device.Release()
		b.device = nil
	}
	if b.adapter != nil {
		b.adapter.Release()
		b.adapter = nil
	}
	if b.instance != nil {
		b.instance.Release()
		b.instance = nil
	}
}

// Name returns the backend name.
func (b *Backend) Name() string {
	if b.adapterInfo != nil {
		return fmt.Sprintf("WebGPU (%s)", b.adapterInfo.Name)
	}
	return "WebGPU"
}

// Device returns the compute device.
func (b *Backend) Device() tensor.Device {
	return tensor.WebGPU
}

// AdapterInfo returns information about the GPU adapter.
func (b *Backend) AdapterInfo() *wgpu.AdapterInfo {
	return b.adapterInfo
}

// IsAvailable checks if WebGPU with compute shader support is available.
// Returns false on software renderers that don't support compute pipelines,
// and also returns false if the underlying driver panics (e.g., missing GPU
// on CI runners). The panic recovery prevents process crashes on headless
// systems such as GitHub Actions Windows runners that have no Vulkan driver.
func IsAvailable() bool {
	ch := make(chan bool, 1)
	go func() {
		ch <- isAvailableProbe()
	}()
	select {
	case result := <-ch:
		return result
	case <-time.After(5 * time.Second):
		return false
	}
}

func isAvailableProbe() (available bool) {
	defer func() {
		if r := recover(); r != nil {
			available = false
		}
	}()

	backend, err := New()
	if err != nil {
		return false
	}
	defer backend.Release()

	shader, err := backend.device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "availability-check",
		WGSL:  "@compute @workgroup_size(1) fn main() {}",
	})
	if err != nil {
		return false
	}
	defer shader.Release()

	bgl, err := backend.device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{})
	if err != nil {
		return false
	}
	defer bgl.Release()

	pl, err := backend.device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		BindGroupLayouts: []*wgpu.BindGroupLayout{bgl},
	})
	if err != nil {
		return false
	}
	defer pl.Release()

	pipeline, err := backend.device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label: "availability-check", Layout: pl, Module: shader, EntryPoint: "main",
	})
	if err != nil {
		return false
	}
	pipeline.Release()

	return true
}

// ListAdapters returns information about all available GPU adapters.
func ListAdapters() ([]*wgpu.AdapterInfo, error) {
	instance, err := wgpu.CreateInstance(nil)
	if err != nil {
		return nil, fmt.Errorf("webgpu: failed to create instance: %w", err)
	}
	defer instance.Release()

	// WebGPU spec doesn't expose adapter enumeration; return the default adapter.
	adapter, err := instance.RequestAdapter(nil)
	if err != nil {
		return nil, fmt.Errorf("webgpu: no adapters available: %w", err)
	}
	defer adapter.Release()

	// In gogpu/wgpu, Info() returns AdapterInfo by value (no error).
	info := adapter.Info()
	return []*wgpu.AdapterInfo{&info}, nil
}

// MemoryStats represents GPU memory usage statistics.
type MemoryStats struct {
	// Total bytes allocated since backend creation
	TotalAllocatedBytes uint64
	// Peak memory usage in bytes
	PeakMemoryBytes uint64
	// Number of currently active buffers
	ActiveBuffers int64
	// Buffer pool statistics
	PoolAllocated uint64
	PoolReleased  uint64
	PoolHits      uint64
	PoolMisses    uint64
	PooledBuffers int
}

// MemoryStats returns current GPU memory usage statistics.
func (b *Backend) MemoryStats() MemoryStats {
	b.memoryStats.mu.RLock()
	totalAllocated := b.memoryStats.totalAllocatedBytes
	peakMemory := b.memoryStats.peakMemoryBytes
	activeBuffers := b.memoryStats.activeBuffers
	b.memoryStats.mu.RUnlock()

	// Get buffer pool stats
	allocated, released, hits, misses, pooledCount := b.bufferPool.Stats()

	return MemoryStats{
		TotalAllocatedBytes: totalAllocated,
		PeakMemoryBytes:     peakMemory,
		ActiveBuffers:       activeBuffers,
		PoolAllocated:       allocated,
		PoolReleased:        released,
		PoolHits:            hits,
		PoolMisses:          misses,
		PooledBuffers:       pooledCount,
	}
}

// trackBufferAllocation records a buffer allocation in memory statistics.
func (b *Backend) trackBufferAllocation(size uint64) {
	b.memoryStats.mu.Lock()
	defer b.memoryStats.mu.Unlock()

	b.memoryStats.totalAllocatedBytes += size
	b.memoryStats.activeBuffers++

	// Update peak memory if needed
	currentMemory := b.memoryStats.totalAllocatedBytes
	if currentMemory > b.memoryStats.peakMemoryBytes {
		b.memoryStats.peakMemoryBytes = currentMemory
	}
}

// trackBufferRelease records a buffer release in memory statistics.
func (b *Backend) trackBufferRelease(size uint64) {
	b.memoryStats.mu.Lock()
	defer b.memoryStats.mu.Unlock()

	if b.memoryStats.totalAllocatedBytes >= size {
		b.memoryStats.totalAllocatedBytes -= size
	}
	b.memoryStats.activeBuffers--
}

// Gather selects elements along dim using index tensor on GPU.
func (b *Backend) Gather(input *tensor.RawTensor, dim int, indices *tensor.RawTensor) *tensor.RawTensor {
	var result *tensor.RawTensor
	var err error
	if b.LazyMode {
		result, err = b.runGatherLazy(input, dim, indices)
	} else {
		result, err = b.runGather(input, dim, indices)
	}
	if err != nil {
		panic("webgpu: Gather: " + err.Error())
	}
	return result
}

// Where performs conditional element selection on GPU.
// result[i] = condition[i] != 0 ? x[i] : y[i].
func (b *Backend) Where(condition, x, y *tensor.RawTensor) *tensor.RawTensor {
	var result *tensor.RawTensor
	var err error
	if b.LazyMode {
		result, err = b.runWhereLazy(condition, x, y)
	} else {
		result, err = b.runWhere(condition, x, y)
	}
	if err != nil {
		panic("webgpu: Where: " + err.Error())
	}
	return result
}

// Embedding performs embedding lookup on GPU.
// weight: [num_embeddings, embedding_dim], indices: int32 tensor.
// Returns: [...indices_shape, embedding_dim].
func (b *Backend) Embedding(weight, indices *tensor.RawTensor) *tensor.RawTensor {
	var result *tensor.RawTensor
	var err error
	if b.LazyMode {
		result, err = b.runEmbeddingLazy(weight, indices)
	} else {
		result, err = b.runEmbedding(weight, indices)
	}
	if err != nil {
		panic("webgpu: Embedding: " + err.Error())
	}
	return result
}

// ReadGPUBuffer implements tensor.LazyBackend interface.
// Reads data from a GPU Storage buffer (Storage | CopySrc) to CPU memory.
// bufferPtr must point to a *wgpu.Buffer created with BufferUsageStorage|CopySrc.
//
// Deferred staging: a temporary MapRead buffer is created here on demand,
// the result buffer is copied into it, and the staging buffer is released
// immediately after readback. This means NO staging buffers exist during
// op execution — only one result buffer per lazy tensor.
//
// Sequence:
//  1. flushCommands() — submit all pending compute command buffers in one batch.
//  2. Poll(PollWait) — block until the GPU completes all submitted commands.
//  3. Create a transient MapRead staging buffer.
//  4. CopyBufferToBuffer(resultBuf → staging), Submit, Poll.
//  5. Map staging, copy bytes to CPU slice, Unmap, release staging.
func (b *Backend) ReadGPUBuffer(bufferPtr unsafe.Pointer, size uint64) ([]byte, error) {
	b.flushCommands()

	if debugReadGPU {
		fmt.Fprintf(os.Stderr, "[ReadGPUBuffer] Poll(PollWait) start, size=%d\n", size)
	}
	b.device.Poll(wgpu.PollWait)
	if debugReadGPU {
		fmt.Fprintln(os.Stderr, "[ReadGPUBuffer] Poll(PollWait) done")
	}

	resultBuf := (*wgpu.Buffer)(bufferPtr)

	// Create a transient MapRead staging buffer — only at readback time.
	stagingBuf, err := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: gputypes.BufferUsageMapRead | gputypes.BufferUsageCopyDst,
		Size:  size,
	})
	if err != nil {
		return nil, fmt.Errorf("webgpu: ReadGPUBuffer: create staging: %w", err)
	}
	defer stagingBuf.Release()

	// Copy result buffer → staging in a dedicated one-shot encoder.
	enc, err := b.device.CreateCommandEncoder(nil)
	if err != nil {
		return nil, fmt.Errorf("webgpu: ReadGPUBuffer: create encoder: %w", err)
	}
	enc.CopyBufferToBuffer(resultBuf, 0, stagingBuf, 0, size)
	cmdBuf, err := enc.Finish()
	if err != nil {
		return nil, fmt.Errorf("webgpu: ReadGPUBuffer: finish encoder: %w", err)
	}
	if _, err := b.queue.Submit(cmdBuf); err != nil {
		return nil, fmt.Errorf("webgpu: ReadGPUBuffer: submit copy: %w", err)
	}
	b.device.Poll(wgpu.PollWait)

	// Map staging buffer and copy bytes to CPU.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if debugReadGPU {
		fmt.Fprintln(os.Stderr, "[ReadGPUBuffer] Map start")
	}
	if err := stagingBuf.Map(ctx, wgpu.MapModeRead, 0, size); err != nil {
		return nil, fmt.Errorf("webgpu: ReadGPUBuffer: map failed (size=%d): %w", size, err)
	}
	if debugReadGPU {
		fmt.Fprintln(os.Stderr, "[ReadGPUBuffer] Map done")
	}
	defer func() { _ = stagingBuf.Unmap() }()

	mappedRange, err := stagingBuf.MappedRange(0, size)
	if err != nil {
		return nil, fmt.Errorf("webgpu: ReadGPUBuffer: mapped range (size=%d): %w", size, err)
	}
	defer mappedRange.Release()

	data := mappedRange.Bytes()
	if data == nil {
		return nil, fmt.Errorf("webgpu: ReadGPUBuffer: Bytes() nil (buffer released?)")
	}
	if uint64(len(data)) < size {
		return nil, fmt.Errorf("webgpu: ReadGPUBuffer: got %d bytes, need %d", len(data), size)
	}
	result := make([]byte, size)
	copy(result, data)
	return result, nil
}

// debugReadGPU enables stderr logging for ReadGPUBuffer calls.
// Set via BORN_DEBUG_GPU=1 environment variable.
var debugReadGPU = os.Getenv("BORN_DEBUG_GPU") == "1"

// ReleaseGPUBuffer implements tensor.LazyBackend interface.
// Returns the buffer to the GPU pool for reuse (ADR-016 ExclusivePool).
func (b *Backend) ReleaseGPUBuffer(bufferPtr unsafe.Pointer) {
	buffer := (*wgpu.Buffer)(bufferPtr)
	if buffer != nil {
		b.gpuPool.Release(buffer)
	}
}

// DeferReleaseGPUBuffer implements tensor.LazyBackend interface.
// Returns the buffer to pool for reuse. If no encoder is active (no pending
// command buffers reference the buffer), returns immediately. Otherwise queues
// for return after the next flushCommands/Submit.
func (b *Backend) DeferReleaseGPUBuffer(bufferPtr unsafe.Pointer) {
	buffer := (*wgpu.Buffer)(bufferPtr)
	if buffer == nil {
		return
	}
	b.pendingMu.Lock()
	if b.activeBatch.encoder == nil && len(b.pending) == 0 {
		// No active encoder, no pending submissions — buffer is not referenced
		// by any command buffer. Safe to return to pool immediately.
		b.pendingMu.Unlock()
		b.gpuPool.Release(buffer)
		return
	}
	b.activeBatch.resultBufs = append(b.activeBatch.resultBufs, buffer)
	b.pendingMu.Unlock()
}

// TrainingScope tracks intermediate GPU tensors produced during a training step
// for bulk release after the step completes. This is the primary mechanism for
// explicit GPU memory lifecycle management under ADR-015.
//
// Usage:
//
//	scope := webgpu.NewTrainingScope()
//	defer scope.Release()
//	// ... forward + backward pass, call scope.Track(t) on intermediates ...
type TrainingScope struct {
	tracked []*tensor.RawTensor
}

// NewTrainingScope creates a TrainingScope for tracking intermediate tensors.
func NewTrainingScope() *TrainingScope {
	return &TrainingScope{
		tracked: make([]*tensor.RawTensor, 0, 256),
	}
}

// Track registers a tensor for release when Release is called.
// Safe to call with nil (no-op). Does not take ownership — the caller is
// responsible for not using the tensor after calling scope.Release.
func (s *TrainingScope) Track(t *tensor.RawTensor) {
	if t == nil {
		return
	}
	s.tracked = append(s.tracked, t)
}

// Release calls ReleaseGPU on all tracked tensors and resets the scope for reuse.
// Idempotent — safe to call multiple times. The GPU buffers are enqueued for
// deferred destruction via wgpu's DestroyQueue and released after the next
// queue.Submit completes.
func (s *TrainingScope) Release() {
	for _, t := range s.tracked {
		t.ReleaseGPU()
	}
	s.tracked = s.tracked[:0]
}

// RegisterLiveGPU registers a LazyGPUData in the backend's live tensor set.
func (b *Backend) RegisterLiveGPU(l *tensor.LazyGPUData) {
	b.liveGPU.mu.Lock()
	b.liveGPU.tensors[l] = struct{}{}
	b.liveGPU.mu.Unlock()
}

// UnregisterLiveGPU removes a LazyGPUData from the live tensor set.
func (b *Backend) UnregisterLiveGPU(l *tensor.LazyGPUData) {
	b.liveGPU.mu.Lock()
	delete(b.liveGPU.tensors, l)
	b.liveGPU.mu.Unlock()
}

// FlushGPU submits all pending GPU commands and returns deferred release
// buffers to pool. Unlike ReclaimMemory, does NOT release live tensors —
// safe to call between training steps when carry state is alive.
func (b *Backend) FlushGPU() {
	b.flushCommands()
	if b.device != nil {
		b.device.Poll(wgpu.PollWait)
	}
}

// ReclaimMemory implements tensor.MemoryReclaimer.
// Releases all non-persistent live GPU tensors tracked by the backend,
// then flushes pending commands and blocks until the GPU completes them.
// Persistent tensors (optimizer moments, model weights) survive — only
// transient intermediates (forward pass, NoGrad blocks, masks) are released.
func (b *Backend) ReclaimMemory() {
	b.liveGPU.mu.Lock()
	toRelease := make([]*tensor.LazyGPUData, 0, len(b.liveGPU.tensors))
	surviving := make(map[*tensor.LazyGPUData]struct{})
	for l := range b.liveGPU.tensors {
		if l.IsPersistent() || l.RefCount() > 1 {
			surviving[l] = struct{}{}
		} else {
			toRelease = append(toRelease, l)
		}
	}
	b.liveGPU.tensors = surviving
	b.liveGPU.mu.Unlock()

	for _, l := range toRelease {
		l.Release()
	}

	b.flushCommands()
	if b.device != nil {
		b.device.Poll(wgpu.PollWait)
	}

	// Cleanup pool: deallocate pages unused for 5+ consecutive cycles.
	if b.gpuPool != nil {
		b.gpuPool.Cleanup(false)
	}
}

// Conv2DInputBackward computes gradient with respect to input for Conv2D.
// Not yet implemented for WebGPU backend.
//
//nolint:revive // Parameters unused in stub implementation.
func (b *Backend) Conv2DInputBackward(input, kernel, grad *tensor.RawTensor, stride, padding int) *tensor.RawTensor {
	panic("webgpu: Conv2DInputBackward not implemented")
}

// Conv2DKernelBackward computes gradient with respect to kernel for Conv2D.
// Not yet implemented for WebGPU backend.
//
//nolint:revive // Parameters unused in stub implementation.
func (b *Backend) Conv2DKernelBackward(input, kernel, grad *tensor.RawTensor, stride, padding int) *tensor.RawTensor {
	panic("webgpu: Conv2DKernelBackward not implemented")
}

// MaxPool2DBackward computes gradient with respect to input for MaxPool2D.
// Not yet implemented for WebGPU backend.
//
//nolint:revive // Parameters unused in stub implementation.
func (b *Backend) MaxPool2DBackward(input, grad *tensor.RawTensor, maxIndices []int, kernelSize, stride int) *tensor.RawTensor {
	panic("webgpu: MaxPool2DBackward not implemented")
}
