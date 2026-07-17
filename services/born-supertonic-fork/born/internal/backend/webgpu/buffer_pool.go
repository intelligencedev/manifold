//go:build windows || linux

package webgpu

import (
	"sync"

	"github.com/gogpu/gputypes"
	wgpu "github.com/gogpu/wgpu"
)

// BufferSize represents different buffer size categories for pooling.
type BufferSize int

const (
	// SmallBuffer for tensors < 4KB.
	SmallBuffer BufferSize = iota
	// MediumBuffer for tensors 4KB-1MB.
	MediumBuffer
	// LargeBuffer for tensors > 1MB.
	LargeBuffer
)

const (
	// Size thresholds for buffer categories.
	smallThreshold  = 4 * 1024    // 4KB
	mediumThreshold = 1024 * 1024 // 1MB
	maxPoolSize     = 100         // Max buffers per category
)

// pooledBuffer wraps a GPU buffer with metadata.
type pooledBuffer struct {
	buffer *wgpu.Buffer
	size   uint64
	usage  gputypes.BufferUsage
}

// BufferPool manages GPU buffer reuse to reduce allocation overhead.
// Buffers are categorized by size and usage flags.
type BufferPool struct {
	device *wgpu.Device

	// Pools organized by size category
	small  []*pooledBuffer
	medium []*pooledBuffer
	large  []*pooledBuffer

	mu sync.Mutex

	// Statistics
	totalAllocated uint64
	totalReleased  uint64
	poolHits       uint64
	poolMisses     uint64
}

// NewBufferPool creates a new buffer pool for the given device.
func NewBufferPool(device *wgpu.Device) *BufferPool {
	return &BufferPool{
		device: device,
		small:  make([]*pooledBuffer, 0, maxPoolSize),
		medium: make([]*pooledBuffer, 0, maxPoolSize),
		large:  make([]*pooledBuffer, 0, maxPoolSize),
	}
}

// Acquire gets a buffer from the pool or creates a new one.
// Returns a buffer that matches or exceeds the requested size and usage.
func (p *BufferPool) Acquire(size uint64, usage gputypes.BufferUsage) *wgpu.Buffer {
	p.mu.Lock()
	defer p.mu.Unlock()

	category := p.categorize(size)
	pool := p.getPool(category)

	// Try to find a suitable buffer in the pool
	for i, pb := range pool {
		if pb.size >= size && pb.usage&usage == usage {
			// Found a match - remove from pool and return
			buffer := pb.buffer
			p.removeFromPool(category, i)
			p.poolHits++
			return buffer
		}
	}

	// No suitable buffer found - create new one.
	p.poolMisses++
	p.totalAllocated++

	buffer, err := p.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: usage,
		Size:  size,
	})
	if err != nil {
		panic("webgpu: BufferPool.Acquire: failed to create buffer: " + err.Error())
	}

	return buffer
}

// Release returns a buffer to the pool for reuse.
// If the pool is full, the buffer is immediately released.
func (p *BufferPool) Release(buffer *wgpu.Buffer, size uint64, usage gputypes.BufferUsage) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.totalReleased++

	category := p.categorize(size)
	pool := p.getPool(category)

	// Check if pool has space
	if len(pool) >= maxPoolSize {
		// Pool is full - release buffer immediately
		buffer.Release()
		return
	}

	// Add to pool
	pb := &pooledBuffer{
		buffer: buffer,
		size:   size,
		usage:  usage,
	}
	p.addToPool(category, pb)
}

// Clear releases all pooled buffers.
// Should be called when the backend is released.
func (p *BufferPool) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Release all small buffers
	for _, pb := range p.small {
		pb.buffer.Release()
	}
	p.small = p.small[:0]

	// Release all medium buffers
	for _, pb := range p.medium {
		pb.buffer.Release()
	}
	p.medium = p.medium[:0]

	// Release all large buffers
	for _, pb := range p.large {
		pb.buffer.Release()
	}
	p.large = p.large[:0]
}

// Stats returns statistics about buffer pool usage.
func (p *BufferPool) Stats() (allocated, released, hits, misses uint64, pooledCount int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.totalAllocated, p.totalReleased, p.poolHits, p.poolMisses,
		len(p.small) + len(p.medium) + len(p.large)
}

// categorize determines the size category for a buffer.
func (p *BufferPool) categorize(size uint64) BufferSize {
	if size < smallThreshold {
		return SmallBuffer
	}
	if size < mediumThreshold {
		return MediumBuffer
	}
	return LargeBuffer
}

// getPool returns the pool slice for a given category.
func (p *BufferPool) getPool(category BufferSize) []*pooledBuffer {
	switch category {
	case SmallBuffer:
		return p.small
	case MediumBuffer:
		return p.medium
	case LargeBuffer:
		return p.large
	default:
		return nil
	}
}

// addToPool adds a buffer to the appropriate pool category.
func (p *BufferPool) addToPool(category BufferSize, pb *pooledBuffer) {
	switch category {
	case SmallBuffer:
		p.small = append(p.small, pb)
	case MediumBuffer:
		p.medium = append(p.medium, pb)
	case LargeBuffer:
		p.large = append(p.large, pb)
	}
}

// removeFromPool removes a buffer at index i from the appropriate pool.
func (p *BufferPool) removeFromPool(category BufferSize, i int) {
	switch category {
	case SmallBuffer:
		p.small = append(p.small[:i], p.small[i+1:]...)
	case MediumBuffer:
		p.medium = append(p.medium[:i], p.medium[i+1:]...)
	case LargeBuffer:
		p.large = append(p.large[:i], p.large[i+1:]...)
	}
}
