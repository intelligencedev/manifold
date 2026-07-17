package parallel

import (
	"sync/atomic"
	"testing"
)

func TestFor(t *testing.T) {
	cfg := DefaultConfig()

	var counter int64
	n := 1000

	For(n, func(_ int) {
		atomic.AddInt64(&counter, 1)
	}, cfg)

	if counter != int64(n) {
		t.Errorf("Expected %d, got %d", n, counter)
	}
}

func TestForBatch(t *testing.T) {
	cfg := DefaultConfig()

	batch, channels := 4, 8
	results := make([][]bool, batch)
	for b := range results {
		results[b] = make([]bool, channels)
	}

	ForBatch(batch, channels, func(b, c int) {
		results[b][c] = true
	}, cfg)

	for b := 0; b < batch; b++ {
		for c := 0; c < channels; c++ {
			if !results[b][c] {
				t.Errorf("Missing result at [%d][%d]", b, c)
			}
		}
	}
}

func TestFor_Sequential(t *testing.T) {
	cfg := Config{Enabled: false}

	var counter int64
	For(100, func(_ int) {
		atomic.AddInt64(&counter, 1)
	}, cfg)

	if counter != 100 {
		t.Errorf("Expected 100, got %d", counter)
	}
}

func TestFor_SmallChunk(t *testing.T) {
	// Test that small work units fall back to sequential.
	cfg := DefaultConfig()

	var counter int64
	n := cfg.MinChunkSize - 1

	For(n, func(_ int) {
		atomic.AddInt64(&counter, 1)
	}, cfg)

	if counter != int64(n) {
		t.Errorf("Expected %d, got %d", n, counter)
	}
}

func BenchmarkFor(b *testing.B) {
	cfg := DefaultConfig()
	n := 10000

	b.Run("parallel", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			var sum int64
			For(n, func(i int) {
				atomic.AddInt64(&sum, int64(i))
			}, cfg)
		}
	})

	b.Run("sequential", func(b *testing.B) {
		cfgSeq := cfg
		cfgSeq.Enabled = false
		for i := 0; i < b.N; i++ {
			var sum int64
			For(n, func(i int) {
				atomic.AddInt64(&sum, int64(i))
			}, cfgSeq)
		}
	})
}

func BenchmarkForBatch(b *testing.B) {
	cfg := DefaultConfig()
	batch, channels := 16, 64

	b.Run("parallel", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			var sum int64
			ForBatch(batch, channels, func(bc, c int) {
				atomic.AddInt64(&sum, int64(bc*channels+c))
			}, cfg)
		}
	})

	b.Run("sequential", func(b *testing.B) {
		cfgSeq := cfg
		cfgSeq.Enabled = false
		for i := 0; i < b.N; i++ {
			var sum int64
			ForBatch(batch, channels, func(bc, c int) {
				atomic.AddInt64(&sum, int64(bc*channels+c))
			}, cfgSeq)
		}
	})
}
