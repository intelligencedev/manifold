//go:build windows || linux

package webgpu

import (
	"testing"
)

func TestGenerateBucketSizes_LogSpacing(t *testing.T) {
	sizes := generateBucketSizes(32*1024, 128*1024*1024, 12, 256)

	if len(sizes) == 0 {
		t.Fatal("generateBucketSizes returned empty slice")
	}

	// First bucket should be near startSize, last near endSize.
	if sizes[0] < 32*1024 {
		t.Errorf("first bucket %d < startSize 32768", sizes[0])
	}
	if sizes[len(sizes)-1] != 128*1024*1024 {
		t.Errorf("last bucket %d != endSize %d", sizes[len(sizes)-1], 128*1024*1024)
	}

	// All sizes must be aligned.
	for i, s := range sizes {
		if s%256 != 0 {
			t.Errorf("bucket[%d] = %d not aligned to 256", i, s)
		}
	}

	// Sizes must be strictly increasing (dedup'd).
	for i := 1; i < len(sizes); i++ {
		if sizes[i] <= sizes[i-1] {
			t.Errorf("bucket[%d]=%d <= bucket[%d]=%d — not strictly increasing", i, sizes[i], i-1, sizes[i-1])
		}
	}
}

func TestGenerateBucketSizes_SingleBucket(t *testing.T) {
	sizes := generateBucketSizes(1024, 1024*1024, 1, 256)
	if len(sizes) != 1 {
		t.Errorf("single bucket: got %d sizes, want 1", len(sizes))
	}
}

func TestGenerateBucketSizes_Alignment(t *testing.T) {
	sizes := generateBucketSizes(100, 10000, 8, 64)
	for i, s := range sizes {
		if s%64 != 0 {
			t.Errorf("bucket[%d] = %d not aligned to 64", i, s)
		}
	}
}

func TestExclusivePool_Accept(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}
	backend, err := New()
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer backend.Release()

	pool := newExclusivePool(backend.device, 1024, 256, 5000)
	defer pool.Destroy()

	if !pool.Accept(512) {
		t.Error("pool should accept 512 (< maxAllocSize 1024)")
	}
	if !pool.Accept(1024) {
		t.Error("pool should accept 1024 (== maxAllocSize)")
	}
	if pool.Accept(2048) {
		t.Error("pool should reject 2048 (> maxAllocSize 1024)")
	}
}

func TestExclusivePool_AcquireRelease(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}
	backend, err := New()
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer backend.Release()

	pool := newExclusivePool(backend.device, 4096, 256, 5000)
	defer pool.Destroy()

	buf1, err := pool.Acquire(512)
	if err != nil {
		t.Fatalf("Acquire(512) failed: %v", err)
	}

	total, inUse, free, reserved := pool.Stats()
	if total != 1 || inUse != 1 || free != 0 {
		t.Errorf("after Acquire: total=%d inUse=%d free=%d, want 1/1/0", total, inUse, free)
	}
	if reserved == 0 {
		t.Error("reserved bytes should be > 0")
	}

	pool.Release(buf1)

	total, inUse, free, _ = pool.Stats()
	if total != 1 || inUse != 0 || free != 1 {
		t.Errorf("after Release: total=%d inUse=%d free=%d, want 1/0/1", total, inUse, free)
	}
}

func TestExclusivePool_Reuse(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}
	backend, err := New()
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer backend.Release()

	pool := newExclusivePool(backend.device, 4096, 256, 5000)
	defer pool.Destroy()

	buf1, _ := pool.Acquire(512)
	pool.Release(buf1)

	buf2, _ := pool.Acquire(512)
	if buf2 != buf1 {
		t.Error("second Acquire should reuse the released buffer")
	}

	total, _, _, _ := pool.Stats() //nolint:dogsled // only need total count
	if total != 1 {
		t.Errorf("should have only 1 page after reuse, got %d", total)
	}
	pool.Release(buf2)
}

func TestExclusivePool_TotalReservedTracking(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}
	backend, err := New()
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer backend.Release()

	pool := newExclusivePool(backend.device, 1024*1024, 256, 5000)
	defer pool.Destroy()

	buf1, _ := pool.Acquire(1024)
	buf2, _ := pool.Acquire(2048)

	_, _, _, reserved := pool.Stats() //nolint:dogsled // only need reserved bytes
	if reserved == 0 {
		t.Error("reserved should be > 0 after allocations")
	}

	pool.Release(buf1)
	pool.Release(buf2)
	pool.Cleanup(true)

	_, _, _, reservedAfter := pool.Stats() //nolint:dogsled // only need reserved bytes
	if reservedAfter != 0 {
		t.Errorf("reserved should be 0 after explicit cleanup, got %d", reservedAfter)
	}
}

func TestTieredPool_AcquireRouting(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}
	backend, err := New()
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer backend.Release()

	pool := backend.gpuPool

	buckets := pool.BucketSizes()
	if len(buckets) < 2 {
		t.Fatalf("expected at least 2 bucket tiers, got %d", len(buckets))
	}
	t.Logf("TieredPool: %d tiers, buckets: %v", len(buckets), buckets)

	// Small allocation should succeed.
	buf, err := pool.Acquire(256)
	if err != nil {
		t.Fatalf("Acquire(256) failed: %v", err)
	}
	pool.Release(buf)

	// Medium allocation should succeed (use last bucket size, not raw maxPage).
	bucketMax := buckets[len(buckets)-1]
	dl := QueryDeviceLimits(backend.device)
	allocSize := bucketMax
	if dl.MaxBufferSize > 0 && allocSize > dl.MaxBufferSize {
		allocSize = dl.MaxBufferSize
	}
	buf2, err := pool.Acquire(allocSize)
	if err != nil {
		t.Fatalf("Acquire(allocSize=%d) failed: %v", allocSize, err)
	}
	pool.Release(buf2)
}

func TestTieredPool_OversizedRejection(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}
	backend, err := New()
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer backend.Release()

	pool := backend.gpuPool

	// Allocation exceeding all pools should fail with error, not panic.
	maxPage, _ := pool.DeviceLimitsInfo()
	_, err = pool.Acquire(maxPage * 2)
	if err == nil {
		t.Error("Acquire(2x maxPage) should return error, got nil")
	}
}

func TestTieredPool_BudgetFromEnv(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}
	backend, err := New()
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer backend.Release()

	budget := backend.gpuPool.BudgetBytes()
	if budget == 0 {
		t.Error("budget should be > 0 (default 2GB)")
	}
	t.Logf("GPU budget: %d MB", budget/1024/1024)
}

func TestTieredPool_Stats(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}
	backend, err := New()
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer backend.Release()

	pool := backend.gpuPool

	buf1, _ := pool.Acquire(1024)
	buf2, _ := pool.Acquire(4096)

	total, inUse, _, reserved := pool.Stats()
	if total < 2 || inUse < 2 {
		t.Errorf("after 2 Acquires: total=%d inUse=%d, want >=2", total, inUse)
	}
	if reserved == 0 {
		t.Error("reserved should be > 0")
	}
	t.Logf("Stats: total=%d inUse=%d reserved=%d bytes", total, inUse, reserved)

	pool.Release(buf1)
	pool.Release(buf2)
}

func TestTieredPool_DeviceLimits(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}
	backend, err := New()
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer backend.Release()

	dl := QueryDeviceLimits(backend.device)
	if dl.MaxBufferSize == 0 {
		t.Error("MaxBufferSize should be > 0")
	}
	if dl.MaxStorageBufferBinding == 0 {
		t.Error("MaxStorageBufferBinding should be > 0")
	}
	if dl.MinAlignment == 0 {
		t.Error("MinAlignment should be > 0")
	}
	t.Logf("Device limits: MaxBuffer=%d MB, MaxStorage=%d MB, Alignment=%d",
		dl.MaxBufferSize/1024/1024, dl.MaxStorageBufferBinding/1024/1024, dl.MinAlignment)
}
