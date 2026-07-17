//go:build windows || linux

package webgpu

import (
	"testing"

	"github.com/gogpu/gputypes"
	wgpu "github.com/gogpu/wgpu"
)

// poolStats is a helper struct for cleaner stats access in tests.
type poolStats struct {
	allocated   uint64
	released    uint64
	hits        uint64
	misses      uint64
	pooledCount int
}

// getPoolStats returns pool statistics in a structured format.
func getPoolStats(pool *BufferPool) poolStats {
	allocated, released, hits, misses, pooledCount := pool.Stats()
	return poolStats{
		allocated:   allocated,
		released:    released,
		hits:        hits,
		misses:      misses,
		pooledCount: pooledCount,
	}
}

func TestBufferPoolAcquireRelease(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer backend.Release()

	pool := backend.bufferPool

	// Acquire a small buffer
	size := uint64(1024) // 1KB
	usage := gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc
	buffer1 := pool.Acquire(size, usage)

	// Check stats
	stats := getPoolStats(pool)
	if stats.allocated != 1 {
		t.Errorf("Expected 1 allocation, got %d", stats.allocated)
	}
	if stats.misses != 1 {
		t.Errorf("Expected 1 miss, got %d", stats.misses)
	}
	if stats.hits != 0 {
		t.Errorf("Expected 0 hits, got %d", stats.hits)
	}
	if stats.released != 0 {
		t.Errorf("Expected 0 releases initially, got %d", stats.released)
	}

	// Release buffer back to pool
	pool.Release(buffer1, size, usage)

	stats = getPoolStats(pool)
	if stats.released != 1 {
		t.Errorf("Expected 1 release, got %d", stats.released)
	}
	if stats.pooledCount != 1 {
		t.Errorf("Expected 1 buffer in pool, got %d", stats.pooledCount)
	}

	// Acquire again - should hit the pool
	buffer2 := pool.Acquire(size, usage)

	stats = getPoolStats(pool)
	if stats.hits != 1 {
		t.Errorf("Expected 1 hit, got %d", stats.hits)
	}
	if stats.pooledCount != 0 {
		t.Errorf("Expected 0 buffers in pool, got %d", stats.pooledCount)
	}

	// Clean up
	buffer2.Release()
}

func TestBufferPoolSizeCategories(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer backend.Release()

	pool := backend.bufferPool

	// Test small buffer (< 4KB)
	smallSize := uint64(2048) // 2KB
	if pool.categorize(smallSize) != SmallBuffer {
		t.Errorf("Expected SmallBuffer for size %d", smallSize)
	}

	// Test medium buffer (4KB - 1MB)
	mediumSize := uint64(512 * 1024) // 512KB
	if pool.categorize(mediumSize) != MediumBuffer {
		t.Errorf("Expected MediumBuffer for size %d", mediumSize)
	}

	// Test large buffer (> 1MB)
	largeSize := uint64(2 * 1024 * 1024) // 2MB
	if pool.categorize(largeSize) != LargeBuffer {
		t.Errorf("Expected LargeBuffer for size %d", largeSize)
	}

	// Acquire buffers from different categories
	usage := gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc

	smallBuf := pool.Acquire(smallSize, usage)
	mediumBuf := pool.Acquire(mediumSize, usage)
	largeBuf := pool.Acquire(largeSize, usage)

	// Release them
	pool.Release(smallBuf, smallSize, usage)
	pool.Release(mediumBuf, mediumSize, usage)
	pool.Release(largeBuf, largeSize, usage)

	// Should have 3 buffers in pool (one per category)
	stats := getPoolStats(pool)
	if stats.pooledCount != 3 {
		t.Errorf("Expected 3 buffers in pool, got %d", stats.pooledCount)
	}

	// Acquire again - all should hit
	buf1 := pool.Acquire(smallSize, usage)
	buf2 := pool.Acquire(mediumSize, usage)
	buf3 := pool.Acquire(largeSize, usage)

	stats = getPoolStats(pool)
	if stats.hits != 3 {
		t.Errorf("Expected 3 hits, got %d", stats.hits)
	}

	// Clean up acquired buffers
	buf1.Release()
	buf2.Release()
	buf3.Release()
}

func TestBufferPoolClear(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer backend.Release()

	pool := backend.bufferPool

	// Acquire and release several buffers
	usage := gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc
	sizes := []uint64{1024, 8192, 2 * 1024 * 1024} // Small, medium, large

	buffers := make([]*wgpu.Buffer, len(sizes))
	for i, size := range sizes {
		buffers[i] = pool.Acquire(size, usage)
	}

	for i, size := range sizes {
		pool.Release(buffers[i], size, usage)
	}

	// Check pool has buffers
	stats := getPoolStats(pool)
	if stats.pooledCount == 0 {
		t.Error("Expected buffers in pool before clear")
	}

	// Clear pool
	pool.Clear()

	// Check pool is empty
	stats = getPoolStats(pool)
	if stats.pooledCount != 0 {
		t.Errorf("Expected 0 buffers after clear, got %d", stats.pooledCount)
	}
}

func TestBufferPoolMaxSize(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer backend.Release()

	pool := backend.bufferPool

	// Try to exceed pool capacity (maxPoolSize = 100)
	size := uint64(1024)
	usage := gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc

	buffers := make([]*wgpu.Buffer, 105) // More than maxPoolSize

	// Acquire buffers
	for i := range buffers {
		buffers[i] = pool.Acquire(size, usage)
	}

	// Release all buffers
	for _, buf := range buffers {
		pool.Release(buf, size, usage)
	}

	// Pool should have at most maxPoolSize buffers
	stats := getPoolStats(pool)
	if stats.pooledCount > maxPoolSize {
		t.Errorf("Pool exceeded max size: %d > %d", stats.pooledCount, maxPoolSize)
	}

	// The excess buffers should have been released immediately
	// So pooledCount should be exactly maxPoolSize
	if stats.pooledCount != maxPoolSize {
		t.Errorf("Expected exactly %d buffers in pool, got %d", maxPoolSize, stats.pooledCount)
	}
}

func TestBufferPoolUsageMismatch(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer backend.Release()

	pool := backend.bufferPool

	size := uint64(1024)
	usage1 := gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc
	usage2 := gputypes.BufferUsageStorage | gputypes.BufferUsageCopyDst

	// Acquire and release with usage1
	buf1 := pool.Acquire(size, usage1)
	pool.Release(buf1, size, usage1)

	// Acquire with different usage - should miss
	buf2 := pool.Acquire(size, usage2)

	stats := getPoolStats(pool)
	if stats.hits != 0 {
		t.Errorf("Expected 0 hits for different usage, got %d", stats.hits)
	}
	if stats.misses != 2 {
		t.Errorf("Expected 2 misses (initial + mismatch), got %d", stats.misses)
	}

	// Clean up
	buf2.Release()
}

func TestBackendMemoryStats(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer backend.Release()

	// Initial stats should be zero
	stats := backend.MemoryStats()
	if stats.TotalAllocatedBytes != 0 {
		t.Errorf("Expected 0 total allocated, got %d", stats.TotalAllocatedBytes)
	}
	if stats.ActiveBuffers != 0 {
		t.Errorf("Expected 0 active buffers, got %d", stats.ActiveBuffers)
	}

	// Track some allocations
	backend.trackBufferAllocation(1024)
	backend.trackBufferAllocation(2048)

	stats = backend.MemoryStats()
	if stats.TotalAllocatedBytes != 3072 {
		t.Errorf("Expected 3072 total allocated, got %d", stats.TotalAllocatedBytes)
	}
	if stats.ActiveBuffers != 2 {
		t.Errorf("Expected 2 active buffers, got %d", stats.ActiveBuffers)
	}
	if stats.PeakMemoryBytes != 3072 {
		t.Errorf("Expected 3072 peak memory, got %d", stats.PeakMemoryBytes)
	}

	// Release one buffer
	backend.trackBufferRelease(1024)

	stats = backend.MemoryStats()
	if stats.TotalAllocatedBytes != 2048 {
		t.Errorf("Expected 2048 total allocated after release, got %d", stats.TotalAllocatedBytes)
	}
	if stats.ActiveBuffers != 1 {
		t.Errorf("Expected 1 active buffer after release, got %d", stats.ActiveBuffers)
	}
	// Peak should remain at 3072
	if stats.PeakMemoryBytes != 3072 {
		t.Errorf("Expected peak memory to remain 3072, got %d", stats.PeakMemoryBytes)
	}
}

func TestBufferPoolIntegration(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer backend.Release()

	pool := backend.bufferPool

	// Simulate realistic usage pattern
	usage := gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc

	// Phase 1: Acquire several buffers
	buffers := make([]*wgpu.Buffer, 10)
	sizes := make([]uint64, 10)
	for i := range buffers {
		sizes[i] = uint64(1024 * (i + 1)) // Varying sizes
		buffers[i] = pool.Acquire(sizes[i], usage)
	}

	// Phase 2: Release half of them
	for i := 0; i < 5; i++ {
		pool.Release(buffers[i], sizes[i], usage)
	}

	stats := backend.MemoryStats()
	if stats.PooledBuffers != 5 {
		t.Errorf("Expected 5 pooled buffers, got %d", stats.PooledBuffers)
	}

	// Phase 3: Acquire some more (should hit pool)
	initialHits := stats.PoolHits
	acquiredBufs := make([]*wgpu.Buffer, 3)
	for i := 0; i < 3; i++ {
		acquiredBufs[i] = pool.Acquire(sizes[i], usage)
	}

	stats = backend.MemoryStats()
	hitsGained := stats.PoolHits - initialHits
	if hitsGained != 3 {
		t.Errorf("Expected 3 pool hits, got %d", hitsGained)
	}

	// Clean up
	for _, buf := range acquiredBufs {
		buf.Release()
	}
	for i := 5; i < 10; i++ {
		buffers[i].Release()
	}
}
