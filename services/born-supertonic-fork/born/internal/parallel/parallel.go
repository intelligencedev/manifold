// Package parallel provides parallel execution utilities for the Born ML framework.
package parallel

import (
	"runtime"
	"sync"
)

// Config controls parallel execution behavior.
type Config struct {
	Enabled      bool // Whether parallel execution is enabled.
	NumWorkers   int  // Number of worker goroutines to use.
	MinChunkSize int  // Minimum items per goroutine to avoid overhead.
}

// DefaultConfig returns sensible defaults based on CPU count.
func DefaultConfig() Config {
	n := runtime.NumCPU()
	return Config{
		Enabled:      n > 1,
		NumWorkers:   n,
		MinChunkSize: 64, // Typical cache line aware chunk.
	}
}

// For executes f(i) for i in [0, n) with optional parallelism.
// Falls back to sequential execution if parallelism is disabled or n is too small.
func For(n int, f func(i int), cfg Config) {
	if !cfg.Enabled || n < cfg.MinChunkSize {
		// Sequential fallback.
		for i := 0; i < n; i++ {
			f(i)
		}
		return
	}

	var wg sync.WaitGroup
	chunkSize := max((n+cfg.NumWorkers-1)/cfg.NumWorkers, cfg.MinChunkSize)

	for start := 0; start < n; start += chunkSize {
		end := min(start+chunkSize, n)
		wg.Add(1)
		go func(s, e int) {
			defer wg.Done()
			for i := s; i < e; i++ {
				f(i)
			}
		}(start, end)
	}
	wg.Wait()
}

// ForBatch optimized for batch*channels iteration pattern.
// Common in CNN operations like Conv2D.
func ForBatch(batch, channels int, f func(b, c int), cfg Config) {
	n := batch * channels
	For(n, func(k int) {
		f(k/channels, k%channels)
	}, cfg)
}
