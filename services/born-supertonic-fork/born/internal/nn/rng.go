package nn

import (
	"math/rand"
	"sync"

	"github.com/born-ml/born/internal/tensor"
)

// globalRNG is the package-level random number generator for weight initialization.
// When nil, falls back to math/rand global functions.
var (
	globalRNG   *rand.Rand
	globalRNGMu sync.Mutex
)

// SetSeed sets the global random seed for ALL random operations:
// weight initialization (Xavier, Embedding) AND tensor creation (Randn, Rand).
//
// Call this before creating models to ensure reproducible initialization.
// Without calling SetSeed, initialization uses Go's auto-seeded global rand
// (non-deterministic across runs).
//
// Thread-safe: can be called concurrently with model creation.
//
// Example:
//
//	nn.SetSeed(42)
//	model1 := NewLinear(784, 128, backend)
//
//	nn.SetSeed(42)
//	model2 := NewLinear(784, 128, backend) // identical weights to model1
func SetSeed(seed int64) {
	globalRNGMu.Lock()
	defer globalRNGMu.Unlock()
	globalRNG = rand.New(rand.NewSource(seed)) //nolint:gosec // deterministic for reproducibility
	tensor.SetSeed(seed)
}

// ResetSeed clears the seeded RNG, reverting to Go's auto-seeded global rand.
func ResetSeed() {
	globalRNGMu.Lock()
	defer globalRNGMu.Unlock()
	globalRNG = nil
	tensor.ResetSeed()
}

// randFloat64 returns a random float64 from the seeded or global source.
func randFloat64() float64 {
	globalRNGMu.Lock()
	defer globalRNGMu.Unlock()
	if globalRNG != nil {
		return globalRNG.Float64()
	}
	return rand.Float64() //nolint:gosec // ML uses math/rand intentionally
}

// randNormFloat64 returns a random float64 from standard normal distribution.
func randNormFloat64() float64 {
	globalRNGMu.Lock()
	defer globalRNGMu.Unlock()
	if globalRNG != nil {
		return globalRNG.NormFloat64()
	}
	return rand.NormFloat64() //nolint:gosec // ML uses math/rand intentionally
}
