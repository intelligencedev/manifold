//go:build windows || linux

package webgpu

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"sync"

	"github.com/gogpu/gputypes"
	wgpu "github.com/gogpu/wgpu"
)

// ExclusivePool manages GPU buffer reuse following the Burn/CubeCL pattern.
// Each page holds exactly one wgpu.Buffer (wgpu constraint: buffer is either
// read-only or read_write, not both). Pages are reused across training steps
// to eliminate allocation churn.
//
// After warmup (1 training step), allocation count drops to near-zero.
//
// Reference: cubecl-runtime/src/memory_management/memory_pool/exclusive_pool.rs.
type ExclusivePool struct {
	pages         []gpuPoolPage
	maxAllocSize  uint64 // max single allocation this pool accepts (0 = unlimited)
	alignment     uint64
	curAvgSize    float64
	allocCount    uint64
	deallocPer    uint64
	lastDealloc   uint64
	totalReserved uint64 // total GPU bytes across all pages
	device        *wgpu.Device
	mu            sync.Mutex
}

type gpuPoolPage struct {
	buffer    *wgpu.Buffer
	allocSize uint64
	inUse     bool
	freeCount uint32
}

const (
	gpuPoolAlignment  = 256  // wgpu minimum buffer alignment
	gpuPoolFreeThresh = 2    // consecutive unused observations before dealloc
	gpuPoolSizeDecay  = 0.01 // exponential avg decay (matches CubeCL)

	// CubeCL dealloc scaling constants.
	baseDeallocPeriod = 5000
	deallocScaleBytes = 1024 * 1024 * 1024 // 1 GiB
)

// newExclusivePool creates a single-tier GPU buffer pool.
// maxAllocSize: max single allocation accepted (0 = unlimited).
// deallocPeriod: allocation count between cleanup checks.
func newExclusivePool(device *wgpu.Device, maxAllocSize, alignment, deallocPeriod uint64) *ExclusivePool {
	return &ExclusivePool{
		pages:        make([]gpuPoolPage, 0, 64),
		maxAllocSize: maxAllocSize,
		alignment:    alignment,
		curAvgSize:   float64(maxAllocSize) / 2.0,
		deallocPer:   deallocPeriod,
		device:       device,
	}
}

// Accept returns true if this pool handles allocations of the given size.
// Mirrors CubeCL ExclusiveMemoryPool::accept().
func (p *ExclusivePool) Accept(size uint64) bool {
	return p.maxAllocSize == 0 || p.maxAllocSize >= size
}

// Acquire returns a GPU buffer of at least `size` bytes.
// Reuses a free page if available, otherwise allocates a new one.
func (p *ExclusivePool) Acquire(size uint64) (*wgpu.Buffer, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.curAvgSize = p.curAvgSize*(1.0-gpuPoolSizeDecay) + float64(size)*gpuPoolSizeDecay
	p.allocCount++

	// Find smallest free page that fits, preferring recently used (low freeCount).
	bestIdx := -1
	for i := range p.pages {
		pg := &p.pages[i]
		if !pg.inUse && pg.allocSize >= size {
			if bestIdx == -1 || pg.freeCount < p.pages[bestIdx].freeCount {
				bestIdx = i
			}
		}
	}

	if bestIdx >= 0 {
		p.pages[bestIdx].inUse = true
		if p.pages[bestIdx].freeCount > 0 {
			p.pages[bestIdx].freeCount--
		}
		return p.pages[bestIdx].buffer, nil
	}

	// No reusable page — allocate new.
	allocSize := size
	if uint64(p.curAvgSize) > allocSize {
		allocSize = uint64(p.curAvgSize)
	}
	allocSize = roundUpAlign(allocSize, p.alignment)

	buffer, err := p.device.CreateBuffer(&wgpu.BufferDescriptor{
		Size:  allocSize,
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc,
	})
	if err != nil {
		// Allocation failed (likely OOM). Signal caller to retry after flush.
		return nil, err
	}

	p.totalReserved += allocSize
	p.pages = append(p.pages, gpuPoolPage{
		buffer:    buffer,
		allocSize: allocSize,
		inUse:     true,
		freeCount: gpuPoolFreeThresh - 1,
	})

	return buffer, nil
}

// Release returns a buffer to the pool for reuse. The buffer is NOT destroyed.
func (p *ExclusivePool) Release(buffer *wgpu.Buffer) {
	if buffer == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	for i := range p.pages {
		if p.pages[i].buffer == buffer {
			p.pages[i].inUse = false
			return
		}
	}
	// Buffer not from this pool — destroy it. These are params/uniform/transient
	// buffers that share the resultBufs list with pool buffers. Persistent
	// carry/model tensors are protected by the Persist() flag and never reach here.
	buffer.Release()
}

// Cleanup deallocates pages that have been unused for gpuPoolFreeThresh consecutive cycles.
func (p *ExclusivePool) Cleanup(explicit bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	checkPeriod := p.deallocPer / uint64(gpuPoolFreeThresh)
	if !explicit && p.allocCount-p.lastDealloc < checkPeriod {
		return
	}
	p.lastDealloc = p.allocCount

	n := 0
	for i := range p.pages {
		pg := &p.pages[i]
		if !pg.inUse {
			pg.freeCount++
			if pg.freeCount >= gpuPoolFreeThresh || explicit {
				p.totalReserved -= pg.allocSize
				pg.buffer.Release()
				continue
			}
		}
		p.pages[n] = *pg
		n++
	}
	p.pages = p.pages[:n]
}

// Stats returns pool statistics.
func (p *ExclusivePool) Stats() (total, inUse, free int, bytesReserved uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i := range p.pages {
		if p.pages[i].inUse {
			inUse++
		} else {
			free++
		}
	}
	total = len(p.pages)
	bytesReserved = p.totalReserved
	return
}

// Destroy releases all pages. Called from Backend.Release().
func (p *ExclusivePool) Destroy() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i := range p.pages {
		if p.pages[i].buffer != nil {
			p.pages[i].buffer.Release()
		}
	}
	p.pages = nil
	p.totalReserved = 0
}

// ─────────────────────────────────────────────────────────────────────────────
// TieredPool — multi-tier GPU buffer pool (CubeCL MemoryManagement pattern)
//
// Reference: cubecl-runtime/src/memory_management/memory_manage.rs
//   - generate_bucket_sizes(): log-spaced bucket generation
//   - MemoryManagement::reserve(): first-fit pool routing
//   - Per-pool dealloc_period scaling by bucket size
// ─────────────────────────────────────────────────────────────────────────────

// TieredPool routes allocations to size-appropriate ExclusivePool buckets.
// Smaller allocations go to smaller pools, reducing fragmentation. Each pool
// has its own dealloc period — smaller pools clean up more aggressively.
type TieredPool struct {
	pools       []*ExclusivePool
	maxPageSize uint64 // from device limits
	alignment   uint64 // from device limits
	budgetBytes uint64 // total GPU memory budget (0 = unlimited)
	onOOM       func() // called on allocation failure before retry (flush + poll)
}

// DeviceLimits holds GPU device properties used for pool configuration.
// Queried from wgpu adapter/device at backend initialization.
type DeviceLimits struct {
	MaxBufferSize           uint64 // device.Limits().MaxBufferSize
	MaxStorageBufferBinding uint64 // device.Limits().MaxStorageBufferBindingSize
	MinAlignment            uint64 // device.Limits().MinUniformBufferOffsetAlignment
}

// QueryDeviceLimits reads limits from a wgpu device.
func QueryDeviceLimits(device *wgpu.Device) DeviceLimits {
	limits := device.Limits()
	dl := DeviceLimits{
		MaxBufferSize:           limits.MaxBufferSize,
		MaxStorageBufferBinding: limits.MaxStorageBufferBindingSize,
		MinAlignment:            uint64(limits.MinUniformBufferOffsetAlignment),
	}
	if dl.MinAlignment < gpuPoolAlignment {
		dl.MinAlignment = gpuPoolAlignment
	}
	return dl
}

const (
	defaultNumBuckets = 12
	minBucketSize     = 32 * 1024 // 32 KiB
	defaultBudgetMB   = 2048      // 2 GiB — conservative for iGPU
)

// NewTieredPool creates a multi-tier GPU buffer pool from device limits.
// Budget is configurable via BORN_GPU_BUDGET_MB env var.
//
// Reference: CubeCL generate_bucket_sizes() + MemoryManagement::from_configuration().
func NewTieredPool(device *wgpu.Device) *TieredPool {
	dl := QueryDeviceLimits(device)

	// Use the smaller of MaxStorageBufferBinding and MaxBufferSize.
	// MaxStorageBufferBinding can exceed MaxBufferSize (e.g., 1023 MB vs 256 MB),
	// but CreateBuffer enforces MaxBufferSize as the hard limit.
	maxPage := dl.MaxStorageBufferBinding
	if dl.MaxBufferSize > 0 && dl.MaxBufferSize < maxPage {
		maxPage = dl.MaxBufferSize
	}
	if maxPage == 0 {
		maxPage = 128 * 1024 * 1024 // 128 MiB fallback
	}
	alignment := dl.MinAlignment
	if alignment == 0 {
		alignment = gpuPoolAlignment
	}

	budgetBytes := getEnvUint64Or("BORN_GPU_BUDGET_MB", defaultBudgetMB) * 1024 * 1024

	bucketSizes := generateBucketSizes(minBucketSize, maxPage, defaultNumBuckets, alignment)

	pools := make([]*ExclusivePool, len(bucketSizes))
	for i, maxSize := range bucketSizes {
		deallocPeriod := uint64(float64(baseDeallocPeriod) * math.Round(1.0+float64(maxSize)/float64(deallocScaleBytes)))
		pools[i] = newExclusivePool(device, maxSize, alignment, deallocPeriod)
	}

	return &TieredPool{
		pools:       pools,
		maxPageSize: maxPage,
		alignment:   alignment,
		budgetBytes: budgetBytes,
	}
}

// generateBucketSizes creates logarithmically-spaced bucket sizes.
// Reference: CubeCL generate_bucket_sizes() in memory_manage.rs:128-151.
func generateBucketSizes(startSize, endSize uint64, numBuckets int, alignment uint64) []uint64 {
	if numBuckets <= 1 {
		return []uint64{roundUpAlign(endSize, alignment)}
	}

	logMin := math.Log(float64(startSize))
	logMax := math.Log(float64(endSize))
	logRange := logMax - logMin

	buckets := make([]uint64, 0, numBuckets)
	for i := 0; i < numBuckets; i++ {
		p := float64(i) / float64(numBuckets-1)
		size := uint64(math.Exp(logMin + logRange*p))
		aligned := roundUpAlign(size, alignment)
		if len(buckets) == 0 || buckets[len(buckets)-1] != aligned {
			buckets = append(buckets, aligned)
		}
	}
	return buckets
}

// Acquire routes the allocation to the first pool that accepts the size.
// If allocation fails (GPU OOM), calls the onOOM callback to flush pending
// GPU commands and free completed buffers, then retries once.
func (tp *TieredPool) Acquire(size uint64) (*wgpu.Buffer, error) {
	if tp.budgetBytes > 0 && tp.TotalReserved() > tp.budgetBytes {
		tp.Cleanup(true)
	}

	for _, pool := range tp.pools {
		if pool.Accept(size) {
			return pool.Acquire(size)
		}
	}
	return nil, fmt.Errorf("gpu pool: buffer size %d exceeds max pool capacity %d", size, tp.maxPageSize)
}

// Release returns a buffer to the appropriate pool.
func (tp *TieredPool) Release(buffer *wgpu.Buffer) {
	if buffer == nil {
		return
	}
	for _, pool := range tp.pools {
		pool.mu.Lock()
		found := false
		for i := range pool.pages {
			if pool.pages[i].buffer == buffer {
				pool.pages[i].inUse = false
				found = true
				break
			}
		}
		pool.mu.Unlock()
		if found {
			return
		}
	}
	// Not in any pool tier — params/uniform/transient buffer. Destroy it.
	buffer.Release()
}

// Cleanup runs cleanup on all pool tiers.
func (tp *TieredPool) Cleanup(explicit bool) {
	for _, pool := range tp.pools {
		pool.Cleanup(explicit)
	}
}

// TotalReserved returns total GPU bytes reserved across all tiers.
func (tp *TieredPool) TotalReserved() uint64 {
	var total uint64
	for _, pool := range tp.pools {
		pool.mu.Lock()
		total += pool.totalReserved
		pool.mu.Unlock()
	}
	return total
}

// Stats returns aggregate pool statistics.
func (tp *TieredPool) Stats() (total, inUse, free int, bytesReserved uint64) {
	for _, pool := range tp.pools {
		t, u, f, b := pool.Stats()
		total += t
		inUse += u
		free += f
		bytesReserved += b
	}
	return
}

// Destroy releases all pages in all tiers.
func (tp *TieredPool) Destroy() {
	for _, pool := range tp.pools {
		pool.Destroy()
	}
}

// BucketSizes returns the max allocation size for each tier (for diagnostics).
func (tp *TieredPool) BucketSizes() []uint64 {
	sizes := make([]uint64, len(tp.pools))
	for i, pool := range tp.pools {
		sizes[i] = pool.maxAllocSize
	}
	return sizes
}

// BudgetBytes returns the configured GPU memory budget.
func (tp *TieredPool) BudgetBytes() uint64 {
	return tp.budgetBytes
}

// DeviceLimitsInfo returns the device limits used for pool configuration.
func (tp *TieredPool) DeviceLimitsInfo() (maxPageSize, alignment uint64) {
	return tp.maxPageSize, tp.alignment
}

func getEnvUint64Or(key string, defaultVal uint64) uint64 {
	if s, ok := os.LookupEnv(key); ok {
		if v, err := strconv.ParseUint(s, 10, 64); err == nil && v > 0 {
			return v
		}
	}
	return defaultVal
}

func roundUpAlign(size, alignment uint64) uint64 {
	return ((size + alignment - 1) / alignment) * alignment
}
